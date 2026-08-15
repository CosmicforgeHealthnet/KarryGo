DROP INDEX IF EXISTS idx_media_objects_data_export_retention;
ALTER TABLE media_upload_authorizations DROP CONSTRAINT IF EXISTS chk_upload_authorization_data_export_private;
ALTER TABLE media_objects DROP CONSTRAINT IF EXISTS chk_media_object_data_export_private;
ALTER TABLE media_upload_authorizations DROP COLUMN IF EXISTS file_name;
ALTER TABLE media_upload_authorizations DROP COLUMN IF EXISTS visibility;
ALTER TABLE media_objects DROP COLUMN IF EXISTS file_name;
ALTER TABLE media_objects DROP COLUMN IF EXISTS visibility;
