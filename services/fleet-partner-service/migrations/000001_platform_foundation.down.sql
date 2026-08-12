DROP TABLE IF EXISTS fleet_partner_file_references;
DROP TRIGGER IF EXISTS trg_fleet_partner_audit_append_only ON fleet_partner_audit_records;
DROP TABLE IF EXISTS fleet_partner_audit_records;
DROP TABLE IF EXISTS fleet_partner_idempotency_keys;
DROP TABLE IF EXISTS fleet_partner_jobs;
DROP TABLE IF EXISTS fleet_partner_inbox;
DROP TABLE IF EXISTS fleet_partner_outbox;
DROP FUNCTION IF EXISTS fleet_partner_forbid_mutation();
