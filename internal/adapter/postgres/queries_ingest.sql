-- name: EnqueueMissingJobs :exec
-- Cria um job para cada par (espécie, fonte) que ainda não existe: CROSS JOIN
-- unnest(lista) combina cada espécie com cada fonte da lista. ON CONFLICT DO
-- NOTHING pula os pares que já têm job, o que torna a operação idempotente.
INSERT INTO ingestion_job (species_id, source)
SELECT s.id, src FROM species s CROSS JOIN unnest(sqlc.arg('sources')::text[]) AS src
ON CONFLICT (species_id, source) DO NOTHING;

-- name: RescheduleStaleJobs :exec
-- Dados mudam (novas fotos, novos avistamentos): refaz jobs antigos.
UPDATE ingestion_job SET status = 'pending', attempts = 0, next_run_at = now(), updated_at = now()
WHERE status = 'done' AND updated_at < now() - make_interval(secs => sqlc.arg('refresh_after_secs')::double precision);

-- name: RequeueStuckJobs :exec
-- Se o worker morrer no meio de um job, ele fica "running" para sempre.
-- Depois de 15 min sem atualização, devolvemos o job para a fila.
-- Conta como tentativa (senão um job que SEMPRE estoura o tempo repetiria para
-- sempre) e desiste na 5ª (mesmo limite MaxAttempts do Runner).
UPDATE ingestion_job SET
    attempts = attempts + 1,
    status = CASE WHEN attempts + 1 >= 5 THEN 'failed' ELSE 'pending' END,
    last_error = 'worker parou no meio do job',
    updated_at = now()
WHERE status = 'running' AND updated_at < now() - interval '15 minutes';

-- name: ReopenFailedJobs :exec
-- Uma queda temporária da fonte (ou chave trocada) não pode condenar o job
-- para sempre: depois de 24 h, jobs 'failed' ganham nova chance.
UPDATE ingestion_job SET status = 'pending', attempts = 0, next_run_at = now(), updated_at = now()
WHERE status = 'failed' AND updated_at < now() - interval '24 hours';

-- name: ReleaseJob :exec
-- Devolve à fila um job pego que não chegou a rodar, sem contar tentativa.
UPDATE ingestion_job SET status = 'pending', updated_at = now() WHERE id = $1 AND status = 'running';

-- name: CountPendingJobs :one
-- Quantos jobs estão esperando a vez.
SELECT count(*)::int FROM ingestion_job WHERE status = 'pending';

-- name: ClaimDueJobs :many
-- FOR UPDATE trava as linhas escolhidas até o fim da transação; SKIP LOCKED
-- faz esta consulta pular as que outro worker já travou, em vez de esperar.
-- É o padrão clássico de "fila no PostgreSQL": dois workers nunca pegam o
-- mesmo job. Os mais atrasados (menor next_run_at) saem primeiro, e o JOIN
-- com species traz o nome científico que a fonte usa na busca.
UPDATE ingestion_job j SET status = 'running', updated_at = now()
FROM species s
WHERE j.species_id = s.id AND j.id IN (
    SELECT id FROM ingestion_job
    WHERE status = 'pending' AND next_run_at <= now()
    ORDER BY next_run_at
    LIMIT sqlc.arg('lim')
    FOR UPDATE SKIP LOCKED)
RETURNING j.id, j.species_id, s.scientific_name, j.source, j.attempts;

-- name: CompleteJob :exec
-- Marca o job como concluído e limpa o último erro.
UPDATE ingestion_job SET status = 'done', last_error = '', updated_at = now() WHERE id = $1;

-- name: FailJob :exec
-- Registra a falha: com give_up, o job vira 'failed'; senão volta para
-- 'pending' com a próxima tentativa daqui a retry_in_secs segundos.
UPDATE ingestion_job SET
    status = CASE WHEN sqlc.arg('give_up')::bool THEN 'failed' ELSE 'pending' END,
    attempts = attempts + 1,
    last_error = sqlc.arg('last_error'),
    next_run_at = now() + make_interval(secs => sqlc.arg('retry_in_secs')::double precision),
    updated_at = now()
WHERE id = sqlc.arg('id');

-- name: DeleteMediaByKind :exec
-- Apaga as fotos (ou o canto) da espécie, antes de gravar as novas.
DELETE FROM media WHERE species_id = $1 AND kind = $2;

-- name: InsertMedia :exec
-- Grava uma foto ou um canto, com o crédito. Os 4 últimos campos são da
-- curadoria (só fotos; NULL no canto).
INSERT INTO media (species_id, kind, position, thumb_key, medium_key, large_key, audio_key,
                   width, height, duration_ms, author, license, source, source_url,
                   source_photo_id, category, score, model_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18);

-- name: DeleteClusters :exec
-- Apaga os grupos de avistamentos da espécie.
DELETE FROM occurrence_cluster WHERE species_id = $1;

-- name: InsertCluster :exec
-- Grava um grupo de avistamentos.
INSERT INTO occurrence_cluster (species_id, lat, lng, count, precision) VALUES ($1, $2, $3, $4, $5);
