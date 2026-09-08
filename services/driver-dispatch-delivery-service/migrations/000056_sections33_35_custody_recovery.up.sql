-- Sections 33-35: authority-local immutable custody evidence and recovery ledger.
ALTER TABLE dispatch_vendor_delivery_missions DROP CONSTRAINT dispatch_vendor_delivery_missions_status_check;
ALTER TABLE dispatch_vendor_delivery_missions ADD CONSTRAINT dispatch_vendor_delivery_missions_status_check
CHECK (status IN ('ASSIGNED','PICKUP_REQUIRED','PICKED_UP','DELIVERY_REQUIRED','DELIVERED','COMPLETED','RETURN_REQUIRED','RETURNED','REPLACEMENT_REQUIRED','TRANSFER_REQUIRED','CANCELLED','FAILED'));
ALTER TABLE dispatch_vendor_handover_challenges DROP CONSTRAINT dispatch_vendor_handover_challenges_stage_check;
ALTER TABLE dispatch_vendor_handover_challenges ADD CONSTRAINT dispatch_vendor_handover_challenges_stage_check
CHECK (stage IN ('PICKUP','DELIVERY','RETURN','PROVIDER_TRANSFER'));

CREATE TABLE custody_manifests (
  mission_ref TEXT PRIMARY KEY,
  authority TEXT NOT NULL,
  tenant_ref TEXT NOT NULL,
  business_ref TEXT NOT NULL,
  branch_ref TEXT NOT NULL,
  manifest_ref TEXT NOT NULL,
  manifest_version BIGINT NOT NULL CHECK (manifest_version >= 1),
  pickup_stop_ref TEXT NOT NULL,
  parcel_refs TEXT[] NOT NULL CHECK (cardinality(parcel_refs) > 0),
  parcels JSONB NOT NULL CHECK (jsonb_typeof(parcels) = 'array'),
  package_count BIGINT NOT NULL CHECK (package_count > 0),
  created_at TIMESTAMPTZ NOT NULL,
  frozen_at TIMESTAMPTZ NOT NULL,
  status TEXT NOT NULL CHECK (status = 'FROZEN'),
  operation_ref TEXT NOT NULL UNIQUE,
  request_fingerprint TEXT NOT NULL,
  UNIQUE (tenant_ref, manifest_ref, manifest_version)
);

CREATE TABLE custody_mission_state (
  mission_ref TEXT PRIMARY KEY REFERENCES custody_manifests(mission_ref),
  tenant_ref TEXT NOT NULL,
  manifest_ref TEXT NOT NULL,
  manifest_version BIGINT NOT NULL,
  current_provider_ref TEXT NOT NULL DEFAULT '',
  replacement_provider_ref TEXT NOT NULL DEFAULT '',
  receiver_challenge_ref TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  version BIGINT NOT NULL CHECK (version >= 1),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE custody_parcel_confirmations (
  operation_ref TEXT PRIMARY KEY,
  mission_ref TEXT NOT NULL REFERENCES custody_manifests(mission_ref),
  pickup_stop_ref TEXT NOT NULL,
  provider_ref TEXT NOT NULL,
  parcel_refs TEXT[] NOT NULL,
  package_count BIGINT NOT NULL CHECK (package_count > 0),
  request_fingerprint TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (mission_ref, pickup_stop_ref)
);

CREATE TABLE custody_operations (
  operation_ref TEXT PRIMARY KEY,
  mission_ref TEXT NOT NULL REFERENCES custody_manifests(mission_ref),
  operation_type TEXT NOT NULL CHECK (operation_type IN (
    'MANIFEST_FROZEN','PARCEL_CONFIRMED','EVIDENCE_RECORDED','EXCEPTION_REPORTED',
    'RETURN_STARTED','RETURN_HANDOVER_REQUIRED','RETURN_COMPLETED',
    'REPLACEMENT_REQUIRED','REPLACEMENT_ASSIGNED','PROVIDER_TRANSFER_REQUIRED',
    'PROVIDER_TRANSFER_COMPLETED','WAIT_REQUESTED','RESCHEDULED','RECEIVER_CHANGED',
    'MISSION_DELIVERED','MISSION_COMPLETED','EARNING_ALLOCATION_PENDING_READBACK','EARNING_ALLOCATION_FINAL'
  )),
  request_fingerprint TEXT NOT NULL,
  state TEXT NOT NULL,
  payload JSONB NOT NULL,
  version BIGINT NOT NULL CHECK (version >= 1),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX custody_operations_mission_idx ON custody_operations(mission_ref, created_at);

CREATE TABLE custody_earning_allocations (
  operation_ref TEXT PRIMARY KEY,
  mission_ref TEXT NOT NULL REFERENCES custody_manifests(mission_ref),
  policy_ref TEXT NOT NULL,
  approved_total_minor BIGINT NOT NULL CHECK (approved_total_minor >= 0),
  currency TEXT NOT NULL,
  wallet_operation_ref TEXT NOT NULL UNIQUE,
  wallet_status TEXT NOT NULL CHECK (wallet_status IN ('APPLIED','UNKNOWN')),
  allocations JSONB NOT NULL CHECK (jsonb_typeof(allocations) = 'array'),
  total_allocated_minor BIGINT NOT NULL CHECK (total_allocated_minor >= 0 AND total_allocated_minor <= approved_total_minor),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE custody_evidence (
  id TEXT PRIMARY KEY,
  operation_ref TEXT NOT NULL UNIQUE,
  mission_ref TEXT NOT NULL REFERENCES custody_manifests(mission_ref),
  tenant_ref TEXT NOT NULL,
  manifest_ref TEXT NOT NULL,
  manifest_version BIGINT NOT NULL,
  stop_ref TEXT NOT NULL DEFAULT '',
  parcel_refs TEXT[] NOT NULL DEFAULT '{}',
  evidence_type TEXT NOT NULL CHECK (evidence_type IN (
    'PICKUP','DELIVERY','DAMAGE','DISCREPANCY','FAILED_ATTEMPT','RETURN','PROVIDER_TRANSFER'
  )),
  media_ref TEXT NOT NULL,
  provider_ref TEXT NOT NULL DEFAULT '',
  actor_ref TEXT NOT NULL,
  captured_at TIMESTAMPTZ NOT NULL,
  latitude DOUBLE PRECISION,
  longitude DOUBLE PRECISION,
  access_policy TEXT NOT NULL,
  authority TEXT NOT NULL,
  request_fingerprint TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (mission_ref, media_ref, evidence_type)
);
CREATE INDEX custody_evidence_mission_idx ON custody_evidence(mission_ref, evidence_type);

CREATE TABLE custody_operation_outbox (
  id TEXT PRIMARY KEY,
  aggregate_ref TEXT NOT NULL,
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  attempts INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX custody_operation_outbox_pending_idx ON custody_operation_outbox(created_at) WHERE published_at IS NULL;

CREATE TABLE custody_request_nonces (
  caller TEXT NOT NULL,
  nonce TEXT NOT NULL,
  request_path TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (caller, nonce)
);

CREATE FUNCTION reject_custody_immutable_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'immutable custody evidence cannot be changed';
END $$;

CREATE TRIGGER custody_manifests_immutable
BEFORE UPDATE OR DELETE ON custody_manifests
FOR EACH ROW EXECUTE FUNCTION reject_custody_immutable_mutation();

CREATE TRIGGER custody_evidence_immutable
BEFORE UPDATE OR DELETE ON custody_evidence
FOR EACH ROW EXECUTE FUNCTION reject_custody_immutable_mutation();

CREATE FUNCTION guard_dispatch_custody_completion() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER dispatch_custody_completion_guard BEFORE UPDATE OF status ON trips
FOR EACH ROW EXECUTE FUNCTION guard_dispatch_custody_completion();

CREATE FUNCTION guard_vendor_owned_custody_completion() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER vendor_owned_custody_completion_guard BEFORE UPDATE OF status ON dispatch_vendor_delivery_missions
FOR EACH ROW EXECUTE FUNCTION guard_vendor_owned_custody_completion();
