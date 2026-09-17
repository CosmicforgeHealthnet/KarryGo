-- Shared custodyops multi-manifest custody (Sections 6/7): one mission ->
-- many stops -> one or more manifests per stop -> packages, with
-- manifest-level custody and atomic manifest-granular transfer.

-- BEGIN shared/go/custodyops/migrations/multi_manifest.up.sql (keep verbatim)
-- Shared custodyops multi-manifest evolution (canonical copy).
-- Each execution service (Taxi, Dispatch, Hauling) ships this body in its
-- own numbered migration, followed by its own completion-guard update.
-- Additive: no legacy table or column is dropped; only foreign keys are
-- re-pointed from custody_manifests to the new custody_missions anchor.

CREATE TABLE custody_missions (
  mission_ref TEXT PRIMARY KEY,
  authority TEXT NOT NULL,
  tenant_ref TEXT NOT NULL,
  custody_mode TEXT NOT NULL CHECK (custody_mode IN ('LEGACY_SINGLE','MULTI_MANIFEST')),
  source_version BIGINT NOT NULL DEFAULT 0 CHECK (source_version >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Every existing single-manifest mission becomes a LEGACY_SINGLE anchor.
INSERT INTO custody_missions (mission_ref, authority, tenant_ref, custody_mode, created_at)
SELECT mission_ref, authority, tenant_ref, 'LEGACY_SINGLE', created_at FROM custody_manifests
ON CONFLICT (mission_ref) DO NOTHING;

ALTER TABLE custody_mission_state DROP CONSTRAINT IF EXISTS custody_mission_state_mission_ref_fkey;
ALTER TABLE custody_mission_state ADD CONSTRAINT custody_mission_state_mission_ref_fkey FOREIGN KEY (mission_ref) REFERENCES custody_missions(mission_ref);
ALTER TABLE custody_parcel_confirmations DROP CONSTRAINT IF EXISTS custody_parcel_confirmations_mission_ref_fkey;
ALTER TABLE custody_parcel_confirmations ADD CONSTRAINT custody_parcel_confirmations_mission_ref_fkey FOREIGN KEY (mission_ref) REFERENCES custody_missions(mission_ref);
ALTER TABLE custody_operations DROP CONSTRAINT IF EXISTS custody_operations_mission_ref_fkey;
ALTER TABLE custody_operations ADD CONSTRAINT custody_operations_mission_ref_fkey FOREIGN KEY (mission_ref) REFERENCES custody_missions(mission_ref);
ALTER TABLE custody_earning_allocations DROP CONSTRAINT IF EXISTS custody_earning_allocations_mission_ref_fkey;
ALTER TABLE custody_earning_allocations ADD CONSTRAINT custody_earning_allocations_mission_ref_fkey FOREIGN KEY (mission_ref) REFERENCES custody_missions(mission_ref);
ALTER TABLE custody_evidence DROP CONSTRAINT IF EXISTS custody_evidence_mission_ref_fkey;
ALTER TABLE custody_evidence ADD CONSTRAINT custody_evidence_mission_ref_fkey FOREIGN KEY (mission_ref) REFERENCES custody_missions(mission_ref);
ALTER TABLE custody_manifests ADD CONSTRAINT custody_manifests_mission_anchor_fkey FOREIGN KEY (mission_ref) REFERENCES custody_missions(mission_ref);

-- Legacy single-manifest state has exactly one manifest; multi-manifest
-- missions keep manifest identity on custody_stop_manifests instead.
ALTER TABLE custody_mission_state ALTER COLUMN manifest_ref SET DEFAULT '';
ALTER TABLE custody_mission_state ALTER COLUMN manifest_version SET DEFAULT 0;

CREATE TABLE custody_mission_stops (
  mission_ref TEXT NOT NULL REFERENCES custody_missions(mission_ref),
  stop_ref TEXT NOT NULL,
  stop_type TEXT NOT NULL CHECK (stop_type IN ('PICKUP','DROPOFF')),
  sequence INT NOT NULL CHECK (sequence >= 1),
  status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PARTIALLY_COLLECTED','COLLECTED','DELIVERED','FAILED','CANCELLED_BY_REPLAN')),
  version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (mission_ref, stop_ref),
  UNIQUE (mission_ref, sequence)
);

-- One mission -> many stops -> one or more manifests per stop. A NULL
-- pickup_stop_ref is reserved for legacy mission-level association and is
-- never guessed during backfill.
CREATE TABLE custody_stop_manifests (
  mission_ref TEXT NOT NULL REFERENCES custody_missions(mission_ref),
  manifest_ref TEXT NOT NULL,
  manifest_version BIGINT NOT NULL CHECK (manifest_version >= 1),
  pickup_stop_ref TEXT,
  source_authority TEXT NOT NULL,
  source_manifest_checksum TEXT NOT NULL CHECK (length(source_manifest_checksum) BETWEEN 1 AND 128),
  vendor_ref TEXT,
  vendor_branch_ref TEXT,
  package_count BIGINT NOT NULL CHECK (package_count > 0),
  aggregate_weight_grams BIGINT NOT NULL DEFAULT 0 CHECK (aggregate_weight_grams >= 0),
  aggregate_volume_ml BIGINT NOT NULL DEFAULT 0 CHECK (aggregate_volume_ml >= 0),
  handling TEXT[] NOT NULL DEFAULT '{}',
  required BOOLEAN NOT NULL DEFAULT TRUE,
  status TEXT NOT NULL DEFAULT 'REGISTERED' CHECK (status IN ('REGISTERED','COLLECTED','DELIVERED','FAILED','MISSING','DAMAGED','RETURNED','CANCELLED_BY_REPLAN')),
  current_custodian_ref TEXT NOT NULL DEFAULT '',
  operation_ref TEXT NOT NULL,
  collected_at TIMESTAMPTZ,
  delivered_at TIMESTAMPTZ,
  version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (mission_ref, manifest_ref, manifest_version),
  FOREIGN KEY (mission_ref, pickup_stop_ref) REFERENCES custody_mission_stops(mission_ref, stop_ref),
  CHECK ((status = 'COLLECTED') = (current_custodian_ref <> ''))
);
CREATE INDEX custody_stop_manifests_stop_idx ON custody_stop_manifests(mission_ref, pickup_stop_ref, status);

CREATE TABLE custody_manifest_packages (
  mission_ref TEXT NOT NULL,
  manifest_ref TEXT NOT NULL,
  manifest_version BIGINT NOT NULL,
  package_ref TEXT NOT NULL,
  package_count BIGINT NOT NULL DEFAULT 1 CHECK (package_count > 0),
  weight_grams BIGINT NOT NULL DEFAULT 0 CHECK (weight_grams >= 0),
  volume_ml BIGINT NOT NULL DEFAULT 0 CHECK (volume_ml >= 0),
  status TEXT NOT NULL DEFAULT 'EXPECTED' CHECK (status IN ('EXPECTED','COLLECTED','DELIVERED','MISSING','DAMAGED','RETURNED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (mission_ref, package_ref),
  FOREIGN KEY (mission_ref, manifest_ref, manifest_version) REFERENCES custody_stop_manifests(mission_ref, manifest_ref, manifest_version)
);

-- Custody chain. The partial unique index makes a second active custodian
-- for the same manifest impossible regardless of application races.
CREATE TABLE custody_manifest_holdings (
  id TEXT PRIMARY KEY,
  mission_ref TEXT NOT NULL,
  manifest_ref TEXT NOT NULL,
  manifest_version BIGINT NOT NULL,
  custodian_ref TEXT NOT NULL CHECK (custodian_ref <> ''),
  acquired_operation_ref TEXT NOT NULL,
  acquired_via TEXT NOT NULL CHECK (acquired_via IN ('PICKUP_HANDOVER','PROVIDER_TRANSFER')),
  acquired_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  released_operation_ref TEXT,
  release_reason TEXT CHECK (release_reason IN ('TRANSFERRED','DELIVERED','RETURNED','FAILED')),
  released_at TIMESTAMPTZ,
  FOREIGN KEY (mission_ref, manifest_ref, manifest_version) REFERENCES custody_stop_manifests(mission_ref, manifest_ref, manifest_version),
  CHECK ((released_at IS NULL) = (release_reason IS NULL) AND (released_at IS NULL) = (released_operation_ref IS NULL))
);
CREATE UNIQUE INDEX custody_manifest_one_active_custodian ON custody_manifest_holdings(mission_ref, manifest_ref, manifest_version) WHERE released_at IS NULL;
CREATE INDEX custody_manifest_holdings_custodian_idx ON custody_manifest_holdings(mission_ref, custodian_ref) WHERE released_at IS NULL;

-- Exact inventory of what changed hands in each provider transfer.
CREATE TABLE custody_transfer_items (
  transfer_operation_ref TEXT NOT NULL,
  mission_ref TEXT NOT NULL,
  manifest_ref TEXT NOT NULL,
  manifest_version BIGINT NOT NULL,
  from_custodian_ref TEXT NOT NULL,
  to_custodian_ref TEXT NOT NULL CHECK (to_custodian_ref <> from_custodian_ref),
  package_count BIGINT NOT NULL CHECK (package_count > 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (transfer_operation_ref, manifest_ref, manifest_version),
  FOREIGN KEY (mission_ref, manifest_ref, manifest_version) REFERENCES custody_stop_manifests(mission_ref, manifest_ref, manifest_version)
);

ALTER TABLE custody_operations DROP CONSTRAINT IF EXISTS custody_operations_operation_type_check;
ALTER TABLE custody_operations ADD CONSTRAINT custody_operations_operation_type_check CHECK (operation_type IN (
  'MANIFEST_FROZEN','PARCEL_CONFIRMED','EVIDENCE_RECORDED','EXCEPTION_REPORTED',
  'RETURN_STARTED','RETURN_HANDOVER_REQUIRED','RETURN_COMPLETED',
  'REPLACEMENT_REQUIRED','REPLACEMENT_ASSIGNED','PROVIDER_TRANSFER_REQUIRED',
  'PROVIDER_TRANSFER_COMPLETED','WAIT_REQUESTED','RESCHEDULED','RECEIVER_CHANGED',
  'MISSION_DELIVERED','MISSION_COMPLETED','EARNING_ALLOCATION_PENDING_READBACK','EARNING_ALLOCATION_FINAL',
  'MANIFESTS_REGISTERED','MANIFESTS_COLLECTED','MANIFESTS_TRANSFERRED','MANIFESTS_DELIVERED','MANIFEST_EXCEPTION'
));

-- Stale-proof immutability: registered manifest identity and package
-- snapshots never change; only lifecycle columns move.
CREATE FUNCTION reject_custody_manifest_identity_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'custody manifests cannot be deleted';
  END IF;
  IF NEW.pickup_stop_ref IS DISTINCT FROM OLD.pickup_stop_ref OR NEW.source_manifest_checksum <> OLD.source_manifest_checksum
     OR NEW.package_count <> OLD.package_count OR NEW.source_authority <> OLD.source_authority THEN
    RAISE EXCEPTION 'custody manifest identity is immutable';
  END IF;
  IF OLD.status = 'DELIVERED' AND NEW.status <> 'DELIVERED' THEN
    RAISE EXCEPTION 'a delivered manifest cannot return to an earlier custody state';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER custody_stop_manifests_identity BEFORE UPDATE OR DELETE ON custody_stop_manifests
FOR EACH ROW EXECUTE FUNCTION reject_custody_manifest_identity_change();

-- Number of required manifests not yet accounted for. Completion guards in
-- each service call this for MULTI_MANIFEST missions.
CREATE FUNCTION custody_unaccounted_required_manifests(target_mission TEXT) RETURNS BIGINT LANGUAGE sql STABLE AS $$
  SELECT count(*) FROM custody_stop_manifests
   WHERE mission_ref = target_mission AND required AND status NOT IN ('DELIVERED','RETURNED','CANCELLED_BY_REPLAN')
$$;
-- END shared/go/custodyops/migrations/multi_manifest.up.sql

-- Service completion guard: multi-manifest missions cannot complete while a
-- required manifest is unaccounted for.
CREATE OR REPLACE FUNCTION guard_dispatch_custody_completion() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE custody_status TEXT; custody_provider TEXT; custody_mission TEXT; mode TEXT;
BEGIN
  IF NEW.status IN ('completed','proof_submitted') AND NEW.status IS DISTINCT FROM OLD.status THEN
    SELECT s.status,s.current_provider_ref,s.mission_ref,a.custody_mode INTO custody_status,custody_provider,custody_mission,mode
      FROM commerce_missions m JOIN custody_mission_state s ON s.mission_ref=m.mission_ref
      JOIN custody_missions a ON a.mission_ref=s.mission_ref
      WHERE m.booking_id=NEW.booking_id FOR UPDATE OF s;
    IF FOUND AND (custody_status IN ('RECOVERY_REQUIRED','RETURN_REQUIRED','REPLACEMENT_REQUIRED','TRANSFER_REQUIRED')
      OR (custody_provider <> '' AND custody_provider <> COALESCE(NEW.provider_id::text,''))) THEN
      RAISE EXCEPTION 'dispatch completion frozen by custody recovery';
    END IF;
    IF FOUND AND mode = 'MULTI_MANIFEST' AND NEW.status = 'completed' AND custody_unaccounted_required_manifests(custody_mission) > 0 THEN
      RAISE EXCEPTION 'dispatch completion requires every required manifest to be accounted for';
    END IF;
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION guard_vendor_owned_custody_completion() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE custody_status TEXT; custody_provider TEXT; custody_mission TEXT; mode TEXT;
BEGIN
  IF NEW.status IN ('DELIVERED','COMPLETED') AND NEW.status IS DISTINCT FROM OLD.status THEN
    SELECT s.status,s.current_provider_ref,s.mission_ref,a.custody_mode INTO custody_status,custody_provider,custody_mission,mode
      FROM custody_mission_state s JOIN custody_missions a ON a.mission_ref=s.mission_ref
      WHERE s.mission_ref=NEW.mission_ref FOR UPDATE OF s;
    IF FOUND AND (custody_status IN ('RECOVERY_REQUIRED','RETURN_REQUIRED','REPLACEMENT_REQUIRED','TRANSFER_REQUIRED')
      OR (custody_provider <> '' AND custody_provider <> NEW.provider_ref)) THEN
      RAISE EXCEPTION 'vendor-owned completion frozen by custody recovery';
    END IF;
    IF FOUND AND mode = 'MULTI_MANIFEST' AND NEW.status = 'COMPLETED' AND custody_unaccounted_required_manifests(custody_mission) > 0 THEN
      RAISE EXCEPTION 'vendor-owned completion requires every required manifest to be accounted for';
    END IF;
  END IF;
  RETURN NEW;
END $$;
