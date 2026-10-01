-- 0001_init: cria o esquema inicial do catálogo.
-- Migrations são "versões" do banco: cada arquivo leva o banco de um estado
-- para o próximo. O golang-migrate registra quais já rodaram.

-- pg_trgm: índices de "trigramas" deixam buscas com LIKE '%texto%' rápidas.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TYPE biome AS ENUM ('amazonia', 'mata_atlantica', 'cerrado', 'caatinga', 'pantanal', 'pampa');

CREATE TABLE species (
    id                     text PRIMARY KEY,           -- ex.: turdus-rufiventris
    scientific_name        text NOT NULL UNIQUE,
    common_name_pt         text NOT NULL,
    family                 text NOT NULL,
    size_cm                integer CHECK (size_cm > 0), -- NULL = desconhecido
    diet                   text,                        -- NULL = desconhecida
    conservation_status    text NOT NULL DEFAULT ''
        CHECK (conservation_status IN ('', 'LC', 'NT', 'VU', 'EN', 'CR', 'EW', 'EX', 'DD')),
    description            text NOT NULL DEFAULT '',
    description_author     text NOT NULL DEFAULT '',
    description_license    text NOT NULL DEFAULT '',
    description_source     text NOT NULL DEFAULT '',
    description_source_url text NOT NULL DEFAULT '',
    is_curated             boolean NOT NULL DEFAULT false,
    -- Colunas calculadas pela aplicação (domain.NormalizeForSearch):
    sort_name              text NOT NULL,  -- nome popular sem acento: ordena a lista
    search_text            text NOT NULL,  -- nomes popular + científico sem acento: busca
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);

-- Índice que a paginação por cursor usa: ORDER BY sort_name, id.
CREATE INDEX species_sort_idx ON species (sort_name, id);
-- Índice de trigramas para a busca por trecho do nome.
CREATE INDEX species_search_idx ON species USING gin (search_text gin_trgm_ops);

CREATE TABLE species_fact (
    id         bigserial PRIMARY KEY,
    species_id text NOT NULL REFERENCES species (id) ON DELETE CASCADE,
    position   integer NOT NULL,   -- ordem de exibição
    text       text NOT NULL,
    source     text NOT NULL
);

CREATE TABLE species_biome (
    species_id text NOT NULL REFERENCES species (id) ON DELETE CASCADE,
    biome      biome NOT NULL,
    PRIMARY KEY (species_id, biome)
);
CREATE INDEX species_biome_biome_idx ON species_biome (biome);

CREATE TABLE species_state (
    species_id text NOT NULL REFERENCES species (id) ON DELETE CASCADE,
    uf         char(2) NOT NULL CHECK (uf ~ '^[A-Z]{2}$'),
    PRIMARY KEY (species_id, uf)
);
CREATE INDEX species_state_uf_idx ON species_state (uf);

-- Fotos e cantos: preenchidos pelo worker (Etapa 2b).
CREATE TABLE media (
    id          bigserial PRIMARY KEY,
    species_id  text NOT NULL REFERENCES species (id) ON DELETE CASCADE,
    kind        text NOT NULL CHECK (kind IN ('photo', 'audio')),
    position    integer NOT NULL,
    thumb_key   text NOT NULL DEFAULT '',
    medium_key  text NOT NULL DEFAULT '',
    large_key   text NOT NULL DEFAULT '',
    audio_key   text NOT NULL DEFAULT '',
    width       integer NOT NULL DEFAULT 0,
    height      integer NOT NULL DEFAULT 0,
    duration_ms integer NOT NULL DEFAULT 0,
    author      text NOT NULL,
    license     text NOT NULL,
    source      text NOT NULL,
    source_url  text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX media_species_idx ON media (species_id, kind, position);

-- Avistamentos agrupados para o mapa: preenchidos pelo worker (Etapa 2b).
CREATE TABLE occurrence_cluster (
    species_id text NOT NULL REFERENCES species (id) ON DELETE CASCADE,
    lat        double precision NOT NULL,
    lng        double precision NOT NULL,
    count      integer NOT NULL CHECK (count > 0),
    precision  double precision NOT NULL,
    PRIMARY KEY (species_id, lat, lng)
);
