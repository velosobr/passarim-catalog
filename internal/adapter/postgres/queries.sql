-- name: ListSpecies :many
-- Lista resumida com busca, filtros e paginação por cursor.
-- Cada filtro é opcional: "(parâmetro IS NULL OR condição)".
-- A miniatura é a thumb da foto de menor position ('' se não houver foto).
-- ESCAPE '\' declara a barra como escape do LIKE (o Go já escapou % e _).
-- kids_only (frente B) deixa só as espécies com conteúdo Kids, e
-- primary_biome filtra pelo "mundo" do Kids, diferente de biome, que filtra
-- por qualquer bioma em que a ave vive.
-- "(s.sort_name, s.id) > (after_sort, after_id)" compara os pares em ordem
-- (sort_name e, no empate, id): começa logo depois do último item da página
-- anterior e usa o índice species_sort_idx.
SELECT s.id, s.scientific_name, s.common_name_pt, s.conservation_status, s.sort_name,
       COALESCE((SELECT m.thumb_key FROM media m
                 WHERE m.species_id = s.id AND m.kind = 'photo'
                 ORDER BY m.position LIMIT 1), '')::text AS thumbnail_key,
       COALESCE(s.primary_biome::text, '')::text AS primary_biome
FROM species s
WHERE (sqlc.narg('query')::text IS NULL OR s.search_text LIKE '%' || sqlc.narg('query')::text || '%' ESCAPE '\')
  AND (sqlc.narg('biome')::biome IS NULL OR EXISTS (
        SELECT 1 FROM species_biome b WHERE b.species_id = s.id AND b.biome = sqlc.narg('biome')::biome))
  AND (sqlc.narg('state')::text IS NULL OR EXISTS (
        SELECT 1 FROM species_state st WHERE st.species_id = s.id AND st.uf = sqlc.narg('state')::text))
  AND (NOT sqlc.arg('kids_only')::bool OR s.kids IS NOT NULL)
  AND (sqlc.narg('primary_biome')::biome IS NULL OR s.primary_biome = sqlc.narg('primary_biome')::biome)
  AND (sqlc.narg('after_sort')::text IS NULL
       OR (s.sort_name, s.id) > (sqlc.narg('after_sort')::text, sqlc.narg('after_id')::text))
ORDER BY s.sort_name, s.id
LIMIT sqlc.arg('lim');

-- name: GetSpecies :one
-- A linha da espécie pelo id. As listas (fatos, biomas, UFs, mídia e
-- grupos) vêm das consultas abaixo, uma por tabela.
SELECT * FROM species WHERE id = $1;

-- name: ListFacts :many
-- As curiosidades da espécie, na ordem de exibição.
SELECT text, source FROM species_fact WHERE species_id = $1 ORDER BY position;

-- name: ListBiomes :many
-- Os biomas da espécie.
SELECT biome FROM species_biome WHERE species_id = $1 ORDER BY biome;

-- name: ListStates :many
-- As UFs da espécie. O "::text" converte o char(2) da coluna em texto.
SELECT uf::text FROM species_state WHERE species_id = $1 ORDER BY uf;

-- name: ListMedia :many
-- Fotos e canto da espécie, em ordem de tipo e de posição (a foto 0 é a
-- principal).
SELECT * FROM media WHERE species_id = $1 ORDER BY kind, position;

-- name: ListClusters :many
-- Os grupos de avistamentos do mapa, do maior para o menor.
SELECT lat, lng, count, precision FROM occurrence_cluster WHERE species_id = $1 ORDER BY count DESC;

-- name: CountByBiome :many
-- Quantas espécies há em cada bioma (os filtros do Explorar). Com
-- kids_only, só as que têm conteúdo Kids.
SELECT b.biome, count(*)::int AS total
FROM species_biome b JOIN species s ON s.id = b.species_id
WHERE NOT sqlc.arg('kids_only')::bool OR s.kids IS NOT NULL
GROUP BY b.biome;

-- name: CountByState :many
-- Quantas espécies há em cada UF, com o mesmo kids_only.
SELECT st.uf::text AS uf, count(*)::int AS total
FROM species_state st JOIN species s ON s.id = st.species_id
WHERE NOT sqlc.arg('kids_only')::bool OR s.kids IS NOT NULL
GROUP BY st.uf;

-- name: CountByPrimaryBiome :many
-- Quantas espécies há em cada bioma principal ("mundo" do Kids): é a conta
-- do álbum. Espécies sem bioma principal não entram.
SELECT COALESCE(primary_biome::text, '')::text AS biome, count(*)::int AS total
FROM species
WHERE primary_biome IS NOT NULL AND (NOT sqlc.arg('kids_only')::bool OR kids IS NOT NULL)
GROUP BY primary_biome;

-- name: UpsertSpecies :exec
-- "Upsert" = insere, ou atualiza se o id já existir. Só mexe nos campos
-- curados; created_at é preservado. primary_biome e kids (frente B) também
-- são curados: um YAML sem as chaves grava NULL, e a espécie sai do Kids.
INSERT INTO species (id, scientific_name, common_name_pt, family, size_cm, diet, conservation_status,
                     description, description_author, description_license, description_source,
                     description_source_url, is_curated, sort_name, search_text, primary_biome, kids)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, true, $13, $14, $15, $16)
ON CONFLICT (id) DO UPDATE SET
    scientific_name = EXCLUDED.scientific_name, common_name_pt = EXCLUDED.common_name_pt,
    family = EXCLUDED.family, size_cm = EXCLUDED.size_cm, diet = EXCLUDED.diet,
    conservation_status = EXCLUDED.conservation_status, description = EXCLUDED.description,
    description_author = EXCLUDED.description_author, description_license = EXCLUDED.description_license,
    description_source = EXCLUDED.description_source, description_source_url = EXCLUDED.description_source_url,
    is_curated = true, sort_name = EXCLUDED.sort_name, search_text = EXCLUDED.search_text,
    primary_biome = EXCLUDED.primary_biome, kids = EXCLUDED.kids,
    updated_at = now();

-- name: DeleteFacts :exec
-- Apaga as curiosidades da espécie, antes de gravar as novas. O seed
-- regrava fatos, biomas e UFs inteiros a cada rodada, numa transação.
DELETE FROM species_fact WHERE species_id = $1;

-- name: InsertFact :exec
-- Grava uma curiosidade na posição dada.
INSERT INTO species_fact (species_id, position, text, source) VALUES ($1, $2, $3, $4);

-- name: DeleteBiomes :exec
-- Apaga os biomas da espécie.
DELETE FROM species_biome WHERE species_id = $1;

-- name: InsertBiome :exec
-- Grava um bioma da espécie.
INSERT INTO species_biome (species_id, biome) VALUES ($1, $2);

-- name: DeleteStates :exec
-- Apaga as UFs da espécie.
DELETE FROM species_state WHERE species_id = $1;

-- name: InsertState :exec
-- Grava uma UF da espécie.
INSERT INTO species_state (species_id, uf) VALUES ($1, $2);

-- name: UpsertPhotoOverride :exec
-- Grava a escolha manual de fotos do YAML (chave photos:). EXCLUDED.x é o
-- valor que a linha nova traria: no conflito, a linha inteira é trocada.
INSERT INTO photo_override (species_id, primary_observation_id, primary_photo_id, excluded_ids)
VALUES ($1, $2, $3, $4)
ON CONFLICT (species_id) DO UPDATE SET
    primary_observation_id = EXCLUDED.primary_observation_id,
    primary_photo_id = EXCLUDED.primary_photo_id,
    excluded_ids = EXCLUDED.excluded_ids;

-- name: DeletePhotoOverride :exec
-- O YAML deixou de ter a chave photos: a escolha manual some.
DELETE FROM photo_override WHERE species_id = $1;

-- name: DeleteSpeciesNotIn :many
-- O seed apaga as espécies cujo YAML saiu da pasta de conteúdo. "<> ALL(lista)"
-- é "diferente de todos os da lista". As tabelas filhas (mídia, fatos, jobs,
-- relatório) têm ON DELETE CASCADE e somem junto.
DELETE FROM species WHERE id <> ALL(sqlc.arg('keep_ids')::text[]) RETURNING id;

-- name: ListFeaturedEligible :many
-- As espécies que podem ser a "Ave do dia", em ordem de id:
--   - com conteúdo Kids;
--   - com canto (o áudio da posição 0, com chave), porque o card tem botão de canto;
--   - com foto principal (posição 0) de ave sozinha, ou escolhida pelo
--     override do YAML (a palavra final da frente A, seja qual for a
--     categoria que o curador deu). Fotos de antes da 0004 têm category
--     NULL e não entram.
SELECT s.id FROM species s
WHERE s.kids IS NOT NULL
  AND EXISTS (SELECT 1 FROM media a
              WHERE a.species_id = s.id AND a.kind = 'audio' AND a.position = 0 AND a.audio_key <> '')
  AND EXISTS (SELECT 1 FROM media p LEFT JOIN photo_override o ON o.species_id = p.species_id
              WHERE p.species_id = s.id AND p.kind = 'photo' AND p.position = 0
                AND (p.category = 'single_bird' OR (o.primary_photo_id IS NOT NULL AND o.primary_photo_id = p.source_photo_id)))
ORDER BY s.id;

-- name: GetFeaturedDay :one
-- A ave já escolhida para o dia (nenhuma linha = o dia ainda não foi escolhido).
SELECT species_id FROM featured_day WHERE day = $1;

-- name: ListFeaturedCycle :many
-- As linhas do ciclo da linha mais recente antes do dia pedido (inclusive
-- as de dias depois dele, se amanhã foi pedido antes de hoje). Sem nenhuma
-- linha antes do dia, a subconsulta dá NULL e nada volta: é o ciclo 1.
SELECT day, species_id, cycle FROM featured_day
WHERE cycle = (SELECT f.cycle FROM featured_day f WHERE f.day < $1 ORDER BY f.day DESC LIMIT 1)
ORDER BY day;

-- name: InsertFeaturedDay :exec
-- Grava a escolha do dia. ON CONFLICT (day) DO NOTHING: se outra réplica
-- gravou primeiro, a dela fica, e quem chamou relê a linha.
INSERT INTO featured_day (day, species_id, cycle) VALUES ($1, $2, $3) ON CONFLICT (day) DO NOTHING;
