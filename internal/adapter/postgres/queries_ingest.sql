-- name: EnqueueMissingJobs :exec
-- Cria (espécie × fonte) que ainda não existe. ON CONFLICT DO NOTHING
-- torna a operação idempotente.
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
UPDATE ingestion_job SET status = 'pending', updated_at = now()
WHERE status = 'running' AND updated_at < now() - interval '15 minutes';

-- name: CountPendingJobs :one
SELECT count(*)::int FROM ingestion_job WHERE status = 'pending';

-- name: ClaimDueJobs :many
-- FOR UPDATE SKIP LOCKED: se outro worker já travou uma linha nesta
-- transação, pulamos ela em vez de esperar. É o padrão clássico de
-- "fila no PostgreSQL" sem dois workers pegarem o mesmo job.
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
UPDATE ingestion_job SET status = 'done', last_error = '', updated_at = now() WHERE id = $1;

-- name: FailJob :exec
UPDATE ingestion_job SET
    status = CASE WHEN sqlc.arg('give_up')::bool THEN 'failed' ELSE 'pending' END,
    attempts = attempts + 1,
    last_error = sqlc.arg('last_error'),
    next_run_at = now() + make_interval(secs => sqlc.arg('retry_in_secs')::double precision),
    updated_at = now()
WHERE id = sqlc.arg('id');

-- name: DeleteMediaByKind :exec
DELETE FROM media WHERE species_id = $1 AND kind = $2;

-- name: InsertMedia :exec
INSERT INTO media (species_id, kind, position, thumb_key, medium_key, large_key, audio_key,
                   width, height, duration_ms, author, license, source, source_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: DeleteClusters :exec
DELETE FROM occurrence_cluster WHERE species_id = $1;

-- name: InsertCluster :exec
INSERT INTO occurrence_cluster (species_id, lat, lng, count, precision) VALUES ($1, $2, $3, $4, $5);
