ALTER TABLE media_objects
    DROP CONSTRAINT IF EXISTS media_capture_metadata_complete_check,
    DROP COLUMN IF EXISTS integrity_hash,
    DROP COLUMN IF EXISTS original_size_bytes,
    DROP COLUMN IF EXISTS original_mime_type,
    DROP COLUMN IF EXISTS actor_reference,
    DROP COLUMN IF EXISTS device_reference,
    DROP COLUMN IF EXISTS capture_method,
    DROP COLUMN IF EXISTS source_timestamp,
    DROP COLUMN IF EXISTS captured_at;
