-- 0002_media_unique_position: dá a "media" a chave natural que faltava
-- (ADR-0013 do passarim-docs). (species_id, kind, position) já é o que
-- distingue cada mídia de uma espécie (ex.: foto 0 é a capa, foto 1 é a
-- segunda da galeria) — mas nada impedia o worker gravar a mesma posição
-- duas vezes. O índice comum vira um índice ÚNICO (continua servindo pra
-- acelerar "ORDER BY kind, position", só que agora também barra duplicata).
DROP INDEX media_species_idx;
CREATE UNIQUE INDEX media_species_kind_position_key ON media (species_id, kind, position);
