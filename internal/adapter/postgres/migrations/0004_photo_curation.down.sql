-- Desfaz a 0004, na ordem inversa.
ALTER TABLE ingestion_job DROP COLUMN deferrals;
DROP TABLE curation_report;
DROP TABLE photo_override;
ALTER TABLE media
    DROP COLUMN model_version,
    DROP COLUMN score,
    DROP COLUMN category,
    DROP COLUMN source_photo_id;
