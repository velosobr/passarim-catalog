-- name: ListSpecies :many
-- Lista resumida com busca, filtros e paginação por cursor.
-- Cada filtro é opcional: "(parâmetro IS NULL OR condição)".
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
SELECT * FROM species WHERE id = $1;

-- name: ListFacts :many
SELECT text, source FROM species_fact WHERE species_id = $1 ORDER BY position;

-- name: ListBiomes :many
SELECT biome FROM species_biome WHERE species_id = $1 ORDER BY biome;

-- name: ListStates :many
SELECT uf::text FROM species_state WHERE species_id = $1 ORDER BY uf;

-- name: ListMedia :many
SELECT * FROM media WHERE species_id = $1 ORDER BY kind, position;

-- name: ListClusters :many
SELECT lat, lng, count, precision FROM occurrence_cluster WHERE species_id = $1 ORDER BY count DESC;

-- name: CountByBiome :many
SELECT biome, count(*)::int AS total FROM species_biome GROUP BY biome;

-- name: CountByState :many
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
DELETE FROM species_fact WHERE species_id = $1;

-- name: InsertFact :exec
INSERT INTO species_fact (species_id, position, text, source) VALUES ($1, $2, $3, $4);

-- name: DeleteBiomes :exec
DELETE FROM species_biome WHERE species_id = $1;

-- name: InsertBiome :exec
INSERT INTO species_biome (species_id, biome) VALUES ($1, $2);

-- name: DeleteStates :exec
DELETE FROM species_state WHERE species_id = $1;

-- name: InsertState :exec
INSERT INTO species_state (species_id, uf) VALUES ($1, $2);
