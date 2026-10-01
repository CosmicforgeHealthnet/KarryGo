-- Section 12: formal asset capability and service-tier certification.
--
-- Verification decides; it never owns the asset. authority/provider_id/asset_id
-- are OPAQUE references into driver-taxi-service, driver-dispatch-delivery-service
-- and driver-hauling-service, deliberately not foreign keys, exactly as the
-- vendor case tables reference a business they do not own.
--
-- The operational services already run their own per-asset product-eligibility
-- projections (taxi_vehicle_product_eligibility, dispatch_vehicle_product_eligibility,
-- hauling_truck_product_eligibility). Nothing here replaces them: this schema is
-- the authority those projections have been waiting for a producer of.

-- --- Provider verification ---------------------------------------------------
--
-- The minimum provider-verification state certification actually depends on: an
-- asset cannot be admitted to a service product while the provider who operates
-- it is not verified. It is a per-authority record because the same human can be
-- verified as a taxi driver and not as a haulier.
CREATE TABLE IF NOT EXISTS provider_verification_records (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  authority      TEXT NOT NULL CHECK (authority IN ('TAXI','DISPATCH','HAULING')),
  provider_id    TEXT NOT NULL CHECK (btrim(provider_id) <> ''),
  status         TEXT NOT NULL CHECK (status IN ('PENDING','VERIFIED','REJECTED','SUSPENDED','EXPIRED')),
  reason_code    TEXT NOT NULL DEFAULT '',
  source_version BIGINT NOT NULL DEFAULT 1 CHECK (source_version > 0),
  effective_at   TIMESTAMPTZ,
  expires_at     TIMESTAMPTZ,
  correlation_id TEXT NOT NULL DEFAULT '',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (authority, provider_id),
  CHECK (expires_at IS NULL OR effective_at IS NULL OR expires_at > effective_at)
);
CREATE INDEX IF NOT EXISTS provider_verification_records_expiry_idx
  ON provider_verification_records (expires_at)
  WHERE status = 'VERIFIED' AND expires_at IS NOT NULL;

-- --- Vehicle documents -------------------------------------------------------
--
-- A document is EVIDENCE, never a capability. A valid insurance certificate
-- proves the vehicle is insured; it proves nothing about refrigeration range,
-- wheelchair access or a tail lift. There is deliberately no path in this schema
-- or in any code that reads it from a VALID document to an ELIGIBLE
-- certification: only an explicit reviewed decision creates one.
CREATE TABLE IF NOT EXISTS vehicle_documents (
  id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  authority            TEXT NOT NULL CHECK (authority IN ('TAXI','DISPATCH','HAULING')),
  provider_id          TEXT NOT NULL CHECK (btrim(provider_id) <> ''),
  asset_id             TEXT NOT NULL CHECK (btrim(asset_id) <> ''),
  document_type        TEXT NOT NULL CHECK (document_type IN
                         ('REGISTRATION','INSURANCE','ROADWORTHINESS','PERMIT','INSPECTION_REPORT','OTHER')),
  -- Opaque media-file-service asset id. Verification stores the reference and
  -- validates the binding; the bytes stay in Media.
  media_asset_id       TEXT NOT NULL CHECK (btrim(media_asset_id) <> ''),
  document_number      TEXT NOT NULL DEFAULT '',
  status               TEXT NOT NULL DEFAULT 'PENDING'
                         CHECK (status IN ('PENDING','VALID','REJECTED','EXPIRED','SUPERSEDED')),
  issued_at            TIMESTAMPTZ,
  expires_at           TIMESTAMPTZ,
  reason_code          TEXT NOT NULL DEFAULT '',
  source_version       BIGINT NOT NULL DEFAULT 1 CHECK (source_version > 0),
  idempotency_key      TEXT NOT NULL UNIQUE,
  request_fingerprint  TEXT NOT NULL,
  correlation_id       TEXT NOT NULL DEFAULT '',
  version              BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (expires_at IS NULL OR issued_at IS NULL OR expires_at > issued_at)
);
-- One live document of each type per asset. Without this an asset could hold two
-- VALID registrations and "is it registered" would depend on row order.
CREATE UNIQUE INDEX IF NOT EXISTS vehicle_documents_live_idx
  ON vehicle_documents (authority, asset_id, document_type)
  WHERE status IN ('PENDING','VALID');
CREATE INDEX IF NOT EXISTS vehicle_documents_asset_idx
  ON vehicle_documents (authority, asset_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS vehicle_documents_expiry_idx
  ON vehicle_documents (expires_at)
  WHERE status = 'VALID' AND expires_at IS NOT NULL;

-- --- Asset inspections -------------------------------------------------------
CREATE TABLE IF NOT EXISTS asset_inspections (
  id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  authority            TEXT NOT NULL CHECK (authority IN ('TAXI','DISPATCH','HAULING')),
  provider_id          TEXT NOT NULL DEFAULT '',
  asset_id             TEXT NOT NULL CHECK (btrim(asset_id) <> ''),
  inspection_type      TEXT NOT NULL CHECK (inspection_type IN
                         ('PHYSICAL','DOCUMENT','REMOTE','PERIODIC','INCIDENT')),
  outcome              TEXT NOT NULL DEFAULT 'PENDING'
                         CHECK (outcome IN ('PENDING','PASSED','FAILED','EXPIRED')),
  evidence_refs        JSONB NOT NULL DEFAULT '[]'::jsonb,
  inspected_at         TIMESTAMPTZ,
  expires_at           TIMESTAMPTZ,
  inspector_ref        TEXT NOT NULL DEFAULT '',
  reason_code          TEXT NOT NULL DEFAULT '',
  source_version       BIGINT NOT NULL DEFAULT 1 CHECK (source_version > 0),
  idempotency_key      TEXT NOT NULL UNIQUE,
  request_fingerprint  TEXT NOT NULL,
  correlation_id       TEXT NOT NULL DEFAULT '',
  version              BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (jsonb_typeof(evidence_refs) = 'array'),
  CHECK (expires_at IS NULL OR inspected_at IS NULL OR expires_at > inspected_at)
);
CREATE INDEX IF NOT EXISTS asset_inspections_asset_idx
  ON asset_inspections (authority, asset_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS asset_inspections_expiry_idx
  ON asset_inspections (expires_at)
  WHERE outcome = 'PASSED' AND expires_at IS NOT NULL;

-- --- Capability cases --------------------------------------------------------
--
-- product_code is REQUIRED. A capability certified against no service product
-- has no consumer in any operational service and no effect on any matcher, so
-- accepting one would produce a decision that looks authoritative and changes
-- nothing: the silent-eligibility failure this section exists to prevent.
CREATE TABLE IF NOT EXISTS asset_capability_cases (
  id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  authority            TEXT NOT NULL CHECK (authority IN ('TAXI','DISPATCH','HAULING')),
  provider_id          TEXT NOT NULL CHECK (btrim(provider_id) <> ''),
  asset_id             TEXT NOT NULL CHECK (btrim(asset_id) <> ''),
  capability_code      TEXT NOT NULL CHECK (capability_code ~ '^[A-Z][A-Z0-9_]{1,63}$'),
  product_code         TEXT NOT NULL CHECK (product_code ~ '^[A-Z][A-Z0-9_]{1,63}$'),
  requested_value_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  evidence_refs        JSONB NOT NULL DEFAULT '[]'::jsonb,
  document_refs        JSONB NOT NULL DEFAULT '[]'::jsonb,
  inspection_id        UUID REFERENCES asset_inspections(id),
  status               TEXT NOT NULL DEFAULT 'PENDING_REVIEW' CHECK (status IN
                         ('NOT_STARTED','IN_PROGRESS','PENDING_REVIEW','VERIFIED','REJECTED','EXPIRED','SUSPENDED')),
  policy_version       TEXT NOT NULL DEFAULT '',
  source_version       BIGINT NOT NULL DEFAULT 1 CHECK (source_version > 0),
  reason_code          TEXT NOT NULL DEFAULT '',
  expires_at           TIMESTAMPTZ,
  correlation_id       TEXT NOT NULL DEFAULT '',
  idempotency_key      TEXT NOT NULL UNIQUE,
  request_fingerprint  TEXT NOT NULL,
  observed_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (jsonb_typeof(evidence_refs) = 'array'),
  CHECK (jsonb_typeof(document_refs) = 'array')
);
CREATE INDEX IF NOT EXISTS asset_capability_cases_asset_idx
  ON asset_capability_cases (authority, asset_id, product_code, updated_at DESC);
CREATE INDEX IF NOT EXISTS asset_capability_cases_queue_idx
  ON asset_capability_cases (status, created_at)
  WHERE status IN ('NOT_STARTED','IN_PROGRESS','PENDING_REVIEW');

-- --- Decisions (append-only) -------------------------------------------------
CREATE TABLE IF NOT EXISTS asset_capability_decisions (
  id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id              UUID NOT NULL REFERENCES asset_capability_cases(id),
  decision_version     BIGINT NOT NULL CHECK (decision_version > 0),
  decision             TEXT NOT NULL CHECK (decision IN ('VERIFIED','REJECTED','SUSPENDED','EXPIRED')),
  verified_value_json  JSONB NOT NULL DEFAULT '{}'::jsonb,
  reason_code          TEXT NOT NULL DEFAULT '',
  reviewer             TEXT NOT NULL CHECK (btrim(reviewer) <> ''),
  evidence_refs        JSONB NOT NULL DEFAULT '[]'::jsonb,
  document_refs        JSONB NOT NULL DEFAULT '[]'::jsonb,
  policy_version       TEXT NOT NULL DEFAULT '',
  effective_at         TIMESTAMPTZ,
  expires_at           TIMESTAMPTZ,
  correlation_id       TEXT NOT NULL DEFAULT '',
  decided_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (case_id, decision_version)
);

-- Append-only is enforced by the database, not by convention. A decision is the
-- record of what a reviewer concluded at a point in time; editing one would
-- rewrite history that a suspension or an audit may later depend on.
CREATE OR REPLACE FUNCTION reject_asset_capability_decision_mutation() RETURNS trigger
LANGUAGE plpgsql AS $reject$
BEGIN
  RAISE EXCEPTION 'asset capability decisions are append-only';
END $reject$;
DROP TRIGGER IF EXISTS asset_capability_decisions_append_only ON asset_capability_decisions;
CREATE TRIGGER asset_capability_decisions_append_only
  BEFORE UPDATE OR DELETE ON asset_capability_decisions
  FOR EACH ROW EXECUTE FUNCTION reject_asset_capability_decision_mutation();

-- --- Service-tier certifications ---------------------------------------------
--
-- The current authoritative answer per asset per product. Its status vocabulary
-- is the operational projections' vocabulary, not Verification's decision
-- vocabulary, so the mapping happens once here (VERIFIED -> ELIGIBLE) instead of
-- three times in three driver services.
CREATE TABLE IF NOT EXISTS service_tier_certifications (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  authority         TEXT NOT NULL CHECK (authority IN ('TAXI','DISPATCH','HAULING')),
  provider_id       TEXT NOT NULL DEFAULT '',
  asset_id          TEXT NOT NULL CHECK (btrim(asset_id) <> ''),
  product_code      TEXT NOT NULL CHECK (product_code ~ '^[A-Z][A-Z0-9_]{1,63}$'),
  capability_code   TEXT NOT NULL DEFAULT '',
  status            TEXT NOT NULL CHECK (status IN ('PENDING','ELIGIBLE','SUSPENDED','EXPIRED','REJECTED')),
  policy_version    TEXT NOT NULL DEFAULT '',
  certification_ref TEXT NOT NULL DEFAULT '',
  effective_at      TIMESTAMPTZ,
  expires_at        TIMESTAMPTZ,
  source_version    BIGINT NOT NULL DEFAULT 1 CHECK (source_version > 0),
  reason_code       TEXT NOT NULL DEFAULT '',
  last_decision_id  UUID REFERENCES asset_capability_decisions(id),
  case_id           UUID REFERENCES asset_capability_cases(id),
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (authority, asset_id, product_code),
  -- An asset is never eligible without a certification reference to point at.
  CHECK (status <> 'ELIGIBLE' OR btrim(certification_ref) <> ''),
  CHECK (expires_at IS NULL OR effective_at IS NULL OR expires_at > effective_at)
);
CREATE INDEX IF NOT EXISTS service_tier_certifications_asset_idx
  ON service_tier_certifications (authority, asset_id);
CREATE INDEX IF NOT EXISTS service_tier_certifications_expiry_idx
  ON service_tier_certifications (expires_at)
  WHERE status = 'ELIGIBLE' AND expires_at IS NOT NULL;

-- --- Outbox delivery routing -------------------------------------------------
--
-- Two additive changes. delivery_target names the authority a row must reach, so
-- one worker can route three authorities without parsing the payload. And
-- NOT_REQUIRED is a real terminal state for a fact that has no consumer
-- endpoint: a capability or inspection status change is recorded as a platform
-- fact, and leaving such a row PENDING forever would make a healthy outbox look
-- permanently backed up.
ALTER TABLE verification_outbox
  ADD COLUMN IF NOT EXISTS delivery_target TEXT NOT NULL DEFAULT '';

ALTER TABLE verification_outbox DROP CONSTRAINT IF EXISTS verification_outbox_delivery_status_check;
ALTER TABLE verification_outbox
  ADD CONSTRAINT verification_outbox_delivery_status_check
  CHECK (delivery_status IN ('PENDING','FAILED','DELIVERED','DEAD_LETTER','NOT_REQUIRED'));

CREATE INDEX IF NOT EXISTS verification_outbox_asset_delivery_idx
  ON verification_outbox (delivery_status, next_attempt_at, lease_expires_at)
  WHERE event_type IN ('asset.capability.status_changed',
                       'asset.service_tier.status_changed',
                       'asset.inspection.status_changed');
