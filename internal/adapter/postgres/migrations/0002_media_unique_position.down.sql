DROP INDEX media_species_kind_position_key;
CREATE INDEX media_species_idx ON media (species_id, kind, position);
