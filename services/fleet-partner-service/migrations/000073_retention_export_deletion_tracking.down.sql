DROP INDEX IF EXISTS idx_fleet_export_pending_media_deletion;
ALTER TABLE fleet_partner_export_jobs
    DROP COLUMN IF EXISTS media_deletion_request_id,
    DROP COLUMN IF EXISTS media_deletion_status;
