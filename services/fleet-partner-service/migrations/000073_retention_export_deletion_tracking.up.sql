-- Section 28 gap-2C: the expired-exports retention executor
-- (internal/platform/retentionexec/executor_expired_export.go) requests
-- deletion of an export's underlying file through Media Service (never
-- deletes bytes directly -- media-file-service is the sole authority over
-- its own objects, same discipline internal/features/documents' own
-- deletion workflow already established) and must record the request/
-- result reference so a later pass can tell "already confirmed deleted,
-- do not ask again" from "still pending, ask again" without re-deriving
-- it from Media on every single pass. fleet_partner_export_jobs
-- (migration 000055) had no columns for this at all -- exports never
-- needed post-expiry file lifecycle tracking before this executor existed.
--
-- media_deletion_status intentionally has no CHECK constraint (unlike most
-- status columns in this schema): its vocabulary is owned by
-- platformclients.Outcome (COMPLETED/BLOCKED/FAILED/UNKNOWN-mapped
-- states), which is Go code, not a value this migration should have to
-- stay in lockstep with by hand.
ALTER TABLE fleet_partner_export_jobs
    ADD COLUMN IF NOT EXISTS media_deletion_status text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS media_deletion_request_id text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_fleet_export_pending_media_deletion
    ON fleet_partner_export_jobs (organization_id, status)
    WHERE status = 'EXPIRED' AND media_deletion_status <> 'CONFIRMED';
