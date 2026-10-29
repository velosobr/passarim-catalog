-- name: ListSpecies :many
-- Lista resumida com busca, filtros e paginação por cursor.
-- Cada filtro é opcional: "(parâmetro IS NULL OR condição)".
-- A miniatura é a thumb da foto de menor position ('' se não houver foto).
-- ESCAPE '\' declara a barra como escape do LIKE (o Go já escapou % e _).
-- "(s.sort_name, s.id) > (after_sort, after_id)" compara os pares em ordem
-- (sort_name e, no empate, id): começa logo depois do último item da página
-- anterior e usa o índice species_sort_idx.
SELECT s.id, s.scientific_name, s.common_name_pt, s.conservation_status, s.sort_name,
       COALESCE((SELECT m.thumb_key FROM media m
                 WHERE m.species_id = s.id AND m.kind = 'photo'
                 ORDER BY m.position LIMIT 1), '')::text AS thumbnail_key
FROM species s
WHERE (sqlc.narg('query')::text IS NULL OR s.search_text LIKE '%' || sqlc.narg('query')::text || '%' ESCAPE '\')
  AND (sqlc.narg('biome')::biome IS NULL OR EXISTS (
        SELECT 1 FROM species_biome b WHERE b.species_id = s.id AND b.biome = sqlc.narg('biome')::biome))
  AND (sqlc.narg('state')::text IS NULL OR EXISTS (
        SELECT 1 FROM species_state st WHERE st.species_id = s.id AND st.uf = sqlc.narg('state')::text))
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
-- Quantas espécies há em cada bioma (os filtros do Explorar).
SELECT biome, count(*)::int AS total FROM species_biome GROUP BY biome;

-- name: CountByState :many
-- Quantas espécies há em cada UF.
SELECT uf::text AS uf, count(*)::int AS total FROM species_state GROUP BY uf;

-- name: UpsertSpecies :exec
-- "Upsert" = insere, ou atualiza se o id já existir. Só mexe nos campos
-- curados; created_at é preservado.
INSERT INTO species (id, scientific_name, common_name_pt, family, size_cm, diet, conservation_status,
                     description, description_author, description_license, description_source,
                     description_source_url, is_curated, sort_name, search_text)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, true, $13, $14)
ON CONFLICT (id) DO UPDATE SET
    scientific_name = EXCLUDED.scientific_name, common_name_pt = EXCLUDED.common_name_pt,
    family = EXCLUDED.family, size_cm = EXCLUDED.size_cm, diet = EXCLUDED.diet,
    conservation_status = EXCLUDED.conservation_status, description = EXCLUDED.description,
    description_author = EXCLUDED.description_author, description_license = EXCLUDED.description_license,
    description_source = EXCLUDED.description_source, description_source_url = EXCLUDED.description_source_url,
    is_curated = true, sort_name = EXCLUDED.sort_name, search_text = EXCLUDED.search_text,
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
