-- 0003: fila de ingestão. Cada linha = "buscar <fonte> para <espécie>".
CREATE TABLE ingestion_job (
    id          bigserial PRIMARY KEY,
    species_id  text NOT NULL REFERENCES species (id) ON DELETE CASCADE,
    source      text NOT NULL CHECK (source IN ('inaturalist', 'xenocanto', 'gbif')),
    status      text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'done', 'failed')),
    attempts    integer NOT NULL DEFAULT 0,
    next_run_at timestamptz NOT NULL DEFAULT now(),
    last_error  text NOT NULL DEFAULT '',
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (species_id, source)   -- no máximo um job por espécie e fonte
);
-- Índice para a pergunta mais frequente: "quais jobs estão vencidos?".
CREATE INDEX ingestion_job_due_idx ON ingestion_job (status, next_run_at);
