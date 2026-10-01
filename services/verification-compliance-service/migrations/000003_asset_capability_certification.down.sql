-- Reverse of 000003. Order matters: certifications reference decisions and
-- cases, cases reference inspections.
DROP INDEX IF EXISTS verification_outbox_asset_delivery_idx;

ALTER TABLE verification_outbox DROP CONSTRAINT IF EXISTS verification_outbox_delivery_status_check;
-- Restore the pre-000003 vocabulary. Any NOT_REQUIRED row would violate it, so
-- those rows are retired to DELIVERED first: they were never deliverable, and
-- that is what DELIVERED means for them.
UPDATE verification_outbox SET delivery_status = 'DELIVERED' WHERE delivery_status = 'NOT_REQUIRED';
ALTER TABLE verification_outbox
  ADD CONSTRAINT verification_outbox_delivery_status_check
  CHECK (delivery_status IN ('PENDING','FAILED','DELIVERED','DEAD_LETTER'));
ALTER TABLE verification_outbox DROP COLUMN IF EXISTS delivery_target;

DROP TABLE IF EXISTS service_tier_certifications;
DROP TRIGGER IF EXISTS asset_capability_decisions_append_only ON asset_capability_decisions;
DROP TABLE IF EXISTS asset_capability_decisions;
DROP FUNCTION IF EXISTS reject_asset_capability_decision_mutation();
DROP TABLE IF EXISTS asset_capability_cases;
DROP TABLE IF EXISTS asset_inspections;
DROP TABLE IF EXISTS vehicle_documents;
DROP TABLE IF EXISTS provider_verification_records;
