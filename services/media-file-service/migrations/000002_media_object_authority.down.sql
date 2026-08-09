-- Reverses 000002_media_object_authority.
--
-- Dropped in dependency order. media_assets and the original public upload
-- path are untouched, because this migration never modified them.

DROP TRIGGER IF EXISTS trg_media_audit_append_only ON media_audit_records;
DROP TRIGGER IF EXISTS trg_media_object_history_append_only ON media_object_history;

DROP TABLE IF EXISTS media_deletion_requests;
DROP TABLE IF EXISTS media_worker_leases;
DROP TABLE IF EXISTS media_service_request_nonces;
DROP TABLE IF EXISTS media_outbox;
DROP TABLE IF EXISTS media_audit_records;
DROP TABLE IF EXISTS media_object_history;
DROP TABLE IF EXISTS media_upload_authorizations;
DROP TABLE IF EXISTS media_objects;

DROP FUNCTION IF EXISTS media_forbid_mutation();
