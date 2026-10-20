-- Source-owned reconciliation evidence survives operation retention. The
-- canonical operation reference is checked in the same transaction as insertion;
-- no cascading relation may erase this evidence.
CREATE TABLE IF NOT EXISTS dispatch_fleet_relay_readback_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), operation_id uuid NOT NULL,
 outcome text NOT NULL CHECK(outcome IN ('CONFIRMED','UNRESOLVED','UNSUPPORTED')),
 receipt_ref text, intent_digest text,
 recorded_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((outcome='CONFIRMED' AND receipt_ref IS NOT NULL AND intent_digest IS NOT NULL AND intent_digest ~ '^[0-9a-f]{64}$')
     OR (outcome<>'CONFIRMED' AND receipt_ref IS NULL AND intent_digest IS NULL))
);
CREATE INDEX IF NOT EXISTS dispatch_fleet_relay_readback_operation ON dispatch_fleet_relay_readback_audit(operation_id,recorded_at);
CREATE OR REPLACE FUNCTION reject_dispatch_relay_readback_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Relay read-back audit is append-only'; END $$;
DROP TRIGGER IF EXISTS dispatch_relay_readback_immutable ON dispatch_fleet_relay_readback_audit;
CREATE TRIGGER dispatch_relay_readback_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON dispatch_fleet_relay_readback_audit FOR EACH STATEMENT EXECUTE FUNCTION reject_dispatch_relay_readback_mutation();
