-- Desfaz a 0005, na ordem inversa.
DROP TABLE featured_day;
ALTER TABLE media DROP COLUMN peaks;
ALTER TABLE species DROP COLUMN kids, DROP COLUMN primary_biome;
