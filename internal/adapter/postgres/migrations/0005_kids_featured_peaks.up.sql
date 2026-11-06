-- Frente B: conteúdo Kids, bioma principal, picos do canto e "Ave do dia".

-- primary_biome usa o tipo ENUM biome que já existe (o mesmo de
-- species_biome.biome). NULL = a espécie ainda não tem bioma principal.
-- Não há chave estrangeira para species_biome porque o seed grava a
-- espécie antes dos biomas; quem garante que o principal está entre os
-- biomas da ave é a validação do domínio, no loader.
-- kids guarda o conteúdo para crianças como jsonb: JSON binário, que o
-- PostgreSQL valida na gravação. NULL = sem conteúdo Kids, e a espécie
-- não aparece no Kids nem na "Ave do dia".
ALTER TABLE species
    ADD COLUMN primary_biome biome,
    ADD COLUMN kids          jsonb;

-- Os 64 picos do canto, de 0 a 1. real[] é uma lista de números reais.
-- A lista vazia (padrão) vale para fotos e para cantos antigos; o CHECK
-- impede gravar um tamanho errado.
ALTER TABLE media
    ADD COLUMN peaks real[] NOT NULL DEFAULT '{}' CHECK (cardinality(peaks) IN (0, 64));

-- A "Ave do dia": uma linha por dia. A primeira chamada do dia escolhe e
-- grava; as outras leem a mesma linha, então a ave não muda no meio do
-- dia. cycle é a volta do rodízio: dentro de um ciclo nenhuma ave se
-- repete. Apagar a espécie apaga as linhas dela (CASCADE).
CREATE TABLE featured_day (
    day        date PRIMARY KEY,
    species_id text NOT NULL REFERENCES species (id) ON DELETE CASCADE,
    cycle      integer NOT NULL CHECK (cycle > 0)
);
CREATE INDEX featured_day_cycle_idx ON featured_day (cycle);
