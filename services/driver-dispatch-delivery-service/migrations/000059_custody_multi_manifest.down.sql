-- Restore the original completion guard(s), then reverse the shared schema.
CREATE OR REPLACE FUNCTION guard_dispatch_custody_completion() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE custody_status TEXT; custody_provider TEXT;
BEGIN
  IF NEW.status IN ('completed','proof_submitted') AND NEW.status IS DISTINCT FROM OLD.status THEN
    SELECT s.status,s.current_provider_ref INTO custody_status,custody_provider
      FROM commerce_missions m JOIN custody_mission_state s ON s.mission_ref=m.mission_ref
      WHERE m.booking_id=NEW.booking_id FOR UPDATE OF s;
    IF FOUND AND (custody_status IN ('RECOVERY_REQUIRED','RETURN_REQUIRED','REPLACEMENT_REQUIRED','TRANSFER_REQUIRED')
      OR (custody_provider <> '' AND custody_provider <> COALESCE(NEW.provider_id::text,''))) THEN
      RAISE EXCEPTION 'dispatch completion frozen by custody recovery';
    END IF;
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION guard_vendor_owned_custody_completion() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE custody_status TEXT; custody_provider TEXT;
BEGIN
  IF NEW.status IN ('DELIVERED','COMPLETED') AND NEW.status IS DISTINCT FROM OLD.status THEN
    SELECT status,current_provider_ref INTO custody_status,custody_provider
      FROM custody_mission_state WHERE mission_ref=NEW.mission_ref FOR UPDATE;
    IF FOUND AND (custody_status IN ('RECOVERY_REQUIRED','RETURN_REQUIRED','REPLACEMENT_REQUIRED','TRANSFER_REQUIRED')
      OR (custody_provider <> '' AND custody_provider <> NEW.provider_ref)) THEN
      RAISE EXCEPTION 'vendor-owned completion frozen by custody recovery';
    END IF;
  END IF;
  RETURN NEW;
END $$;

-- BEGIN shared/go/custodyops/migrations/multi_manifest.down.sql (keep verbatim)
-- Reverses the shared multi-manifest evolution. Legacy single-manifest rows
-- are untouched; multi-manifest custody data is removed.
DROP FUNCTION IF EXISTS custody_unaccounted_required_manifests(TEXT);
DROP TRIGGER IF EXISTS custody_stop_manifests_identity ON custody_stop_manifests;
DROP FUNCTION IF EXISTS reject_custody_manifest_identity_change();
DROP TABLE IF EXISTS custody_transfer_items;
DROP TABLE IF EXISTS custody_manifest_holdings;
DROP TABLE IF EXISTS custody_manifest_packages;
DROP TABLE IF EXISTS custody_stop_manifests;
DROP TABLE IF EXISTS custody_mission_stops;

DELETE FROM custody_operations WHERE mission_ref IN (SELECT mission_ref FROM custody_missions WHERE custody_mode = 'MULTI_MANIFEST');
DELETE FROM custody_earning_allocations WHERE mission_ref IN (SELECT mission_ref FROM custody_missions WHERE custody_mode = 'MULTI_MANIFEST');
ALTER TABLE custody_evidence DISABLE TRIGGER custody_evidence_immutable;
DELETE FROM custody_evidence WHERE mission_ref IN (SELECT mission_ref FROM custody_missions WHERE custody_mode = 'MULTI_MANIFEST');
ALTER TABLE custody_evidence ENABLE TRIGGER custody_evidence_immutable;
DELETE FROM custody_mission_state WHERE mission_ref IN (SELECT mission_ref FROM custody_missions WHERE custody_mode = 'MULTI_MANIFEST');

ALTER TABLE custody_operations DROP CONSTRAINT IF EXISTS custody_operations_operation_type_check;
ALTER TABLE custody_operations ADD CONSTRAINT custody_operations_operation_type_check CHECK (operation_type IN (
  'MANIFEST_FROZEN','PARCEL_CONFIRMED','EVIDENCE_RECORDED','EXCEPTION_REPORTED',
  'RETURN_STARTED','RETURN_HANDOVER_REQUIRED','RETURN_COMPLETED',
  'REPLACEMENT_REQUIRED','REPLACEMENT_ASSIGNED','PROVIDER_TRANSFER_REQUIRED',
  'PROVIDER_TRANSFER_COMPLETED','WAIT_REQUESTED','RESCHEDULED','RECEIVER_CHANGED',
  'MISSION_DELIVERED','MISSION_COMPLETED','EARNING_ALLOCATION_PENDING_READBACK','EARNING_ALLOCATION_FINAL'
));

ALTER TABLE custody_manifests DROP CONSTRAINT IF EXISTS custody_manifests_mission_anchor_fkey;
ALTER TABLE custody_mission_state DROP CONSTRAINT IF EXISTS custody_mission_state_mission_ref_fkey;
ALTER TABLE custody_mission_state ADD CONSTRAINT custody_mission_state_mission_ref_fkey FOREIGN KEY (mission_ref) REFERENCES custody_manifests(mission_ref);
ALTER TABLE custody_parcel_confirmations DROP CONSTRAINT IF EXISTS custody_parcel_confirmations_mission_ref_fkey;
ALTER TABLE custody_parcel_confirmations ADD CONSTRAINT custody_parcel_confirmations_mission_ref_fkey FOREIGN KEY (mission_ref) REFERENCES custody_manifests(mission_ref);
ALTER TABLE custody_operations DROP CONSTRAINT IF EXISTS custody_operations_mission_ref_fkey;
ALTER TABLE custody_operations ADD CONSTRAINT custody_operations_mission_ref_fkey FOREIGN KEY (mission_ref) REFERENCES custody_manifests(mission_ref);
ALTER TABLE custody_earning_allocations DROP CONSTRAINT IF EXISTS custody_earning_allocations_mission_ref_fkey;
ALTER TABLE custody_earning_allocations ADD CONSTRAINT custody_earning_allocations_mission_ref_fkey FOREIGN KEY (mission_ref) REFERENCES custody_manifests(mission_ref);
ALTER TABLE custody_evidence DROP CONSTRAINT IF EXISTS custody_evidence_mission_ref_fkey;
ALTER TABLE custody_evidence ADD CONSTRAINT custody_evidence_mission_ref_fkey FOREIGN KEY (mission_ref) REFERENCES custody_manifests(mission_ref);
ALTER TABLE custody_mission_state ALTER COLUMN manifest_ref DROP DEFAULT;
ALTER TABLE custody_mission_state ALTER COLUMN manifest_version DROP DEFAULT;
DROP TABLE IF EXISTS custody_missions;
-- END shared/go/custodyops/migrations/multi_manifest.down.sql
