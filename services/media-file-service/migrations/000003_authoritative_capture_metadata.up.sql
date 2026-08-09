ALTER TABLE media_objects
    ADD COLUMN IF NOT EXISTS captured_at timestamptz,
    ADD COLUMN IF NOT EXISTS source_timestamp timestamptz,
    ADD COLUMN IF NOT EXISTS capture_method text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS device_reference text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS actor_reference text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS original_mime_type text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS original_size_bytes bigint NOT NULL DEFAULT 0 CHECK (original_size_bytes >= 0),
    ADD COLUMN IF NOT EXISTS integrity_hash text NOT NULL DEFAULT '';

-- Metadata is populated only after Media has validated and re-read the stored
-- bytes. Existing completed objects remain readable with empty/null metadata.
ALTER TABLE media_objects
    ADD CONSTRAINT media_capture_metadata_complete_check CHECK (
        captured_at IS NULL OR
        (source_timestamp IS NOT NULL AND capture_method <> '' AND
         original_mime_type <> '' AND original_size_bytes > 0 AND integrity_hash <> '')
    );
