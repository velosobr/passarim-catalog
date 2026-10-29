-- 0004_photo_curation: a curadoria de fotos (frente A).
--
-- 1) media ganha o que o curador disse de cada foto. As colunas aceitam NULL:
--    as fotos gravadas antes desta migration não têm esses dados até a
--    reingestão, e quem depende da categoria trata NULL como "não elegível".
ALTER TABLE media
    ADD COLUMN source_photo_id text,   -- id da foto no iNaturalist
    ADD COLUMN category        text CHECK (category IN ('unspecified', 'single_bird', 'flock', 'nest',
                                                      'eggs', 'feather', 'dead_bird', 'no_bird', 'other')),
    ADD COLUMN score           real,   -- pontuação da escolha, de 0 a 1
    ADD COLUMN model_version   text;   -- ex.: rtdetr-r50vd+openclip-b32@2

-- 2) photo_override: a escolha manual da chave photos: do YAML. O seed grava,
--    o worker lê. Uma linha por espécie, no máximo. O CHECK exige que a
--    observação e a foto da principal venham juntas (ou nenhuma das duas).
CREATE TABLE photo_override (
    species_id             text PRIMARY KEY REFERENCES species (id) ON DELETE CASCADE,
    primary_observation_id text,
    primary_photo_id       text,
    excluded_ids           text[] NOT NULL DEFAULT '{}',
    CHECK ((primary_observation_id IS NULL) = (primary_photo_id IS NULL))
);

-- 3) curation_report: uma linha por espécie e execução da curadoria. Existe
--    porque o worker roda com o sistema de arquivos só de leitura: o
--    relatório fica no banco e o comando "worker curation-report" o imprime.
--    jsonb guarda JSON num formato binário que o PostgreSQL consegue consultar.
CREATE TABLE curation_report (
    id               bigserial PRIMARY KEY,
    species_id       text NOT NULL REFERENCES species (id) ON DELETE CASCADE,
    ran_at           timestamptz NOT NULL DEFAULT now(),
    model_version    text NOT NULL,
    primary_photo_id text,                       -- NULL = a espécie ficou sem fotos
    candidates       jsonb NOT NULL,             -- id, observação, categoria, pontuação e confiança de cada candidata
    flags            text[] NOT NULL DEFAULT '{}'
);
-- "A última execução de cada espécie" é a pergunta do relatório.
CREATE INDEX curation_report_species_idx ON curation_report (species_id, ran_at DESC);

-- 4) ingestion_job.deferrals: quantas vezes o job foi ADIADO porque o curador
--    estava fora do ar. Adiar não gasta tentativa (attempts), mas a espera
--    cresce com o backoff a cada adiamento; o sucesso zera a contagem.
ALTER TABLE ingestion_job ADD COLUMN deferrals integer NOT NULL DEFAULT 0;
