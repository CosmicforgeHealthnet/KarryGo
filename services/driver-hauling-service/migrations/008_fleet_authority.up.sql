-- Authoritative Fleet/Hauling boundary. All identities are opaque IDs; this
-- database never references Fleet tables or stores Fleet personal data.
CREATE TABLE IF NOT EXISTS hauling_provider_availability (
  provider_id UUID PRIMARY KEY REFERENCES truck_providers(id) ON DELETE CASCADE,
  state TEXT NOT NULL CHECK (state IN ('AVAILABLE','UNAVAILABLE','ON_JOB','SUSPENDED')),
  shift_open BOOLEAN NOT NULL DEFAULT FALSE,
  version BIGINT NOT NULL DEFAULT 1,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  changed_by TEXT NOT NULL,
  reason_code TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS hauling_organization_provider_links (
  organization_id UUID NOT NULL,
  provider_id UUID NOT NULL REFERENCES truck_providers(id) ON DELETE CASCADE,
  membership_id UUID NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('ACTIVE','REVOKED')),
  version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, provider_id),
  UNIQUE (membership_id)
);

CREATE TABLE IF NOT EXISTS hauling_organization_truck_links (
  organization_id UUID NOT NULL,
  truck_id UUID NOT NULL REFERENCES trucks(id) ON DELETE CASCADE,
  state TEXT NOT NULL CHECK (state IN ('ACTIVE','REVOKED')),
  version BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, truck_id)
);

ALTER TABLE haulage_bookings
  ADD COLUMN IF NOT EXISTS source_version BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS winner_organization_id UUID,
  ADD COLUMN IF NOT EXISTS settlement_organization_id UUID,
  ADD COLUMN IF NOT EXISTS organization_membership_id UUID,
  ADD COLUMN IF NOT EXISTS fleet_assignment_id UUID,
  ADD COLUMN IF NOT EXISTS provider_response_id UUID,
  ADD COLUMN IF NOT EXISTS acceptance_source TEXT,
  ADD COLUMN IF NOT EXISTS assignment_status TEXT,
  ADD COLUMN IF NOT EXISTS assignment_acknowledged_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS assignment_response_deadline TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS loading_started_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS loading_completed_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS unloading_started_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS unloading_completed_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS failed_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS failure_reason TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_haulage_fleet_assignment
  ON haulage_bookings(fleet_assignment_id) WHERE fleet_assignment_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS hauling_organization_reservations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  booking_id UUID NOT NULL REFERENCES haulage_bookings(id) ON DELETE CASCADE,
  organization_id UUID NOT NULL,
  accepting_provider_id UUID,
  membership_id UUID,
  acceptance_source TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('RESERVED','RELEASED','CONSUMED','EXPIRED')),
  version BIGINT NOT NULL DEFAULT 1,
  idempotency_key TEXT NOT NULL,
  request_fingerprint TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (idempotency_key)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_hauling_live_reservation
  ON hauling_organization_reservations(booking_id)
  WHERE status = 'RESERVED';

CREATE TABLE IF NOT EXISTS hauling_command_operations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  operation_type TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  request_fingerprint TEXT NOT NULL,
  resource_id UUID,
  response_body JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (operation_type, idempotency_key)
);

CREATE TABLE IF NOT EXISTS hauling_domain_outbox (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  aggregate_type TEXT NOT NULL,
  aggregate_id UUID NOT NULL,
  event_type TEXT NOT NULL,
  event_version BIGINT NOT NULL,
  payload JSONB NOT NULL,
  correlation_id TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  attempts INT NOT NULL DEFAULT 0,
  lease_owner TEXT,
  lease_until TIMESTAMPTZ,
  delivered_at TIMESTAMPTZ,
  dead_lettered_at TIMESTAMPTZ,
  last_error TEXT,
  UNIQUE (aggregate_type, aggregate_id, event_version, event_type)
);
CREATE INDEX IF NOT EXISTS idx_hauling_outbox_delivery
  ON hauling_domain_outbox(available_at, occurred_at) WHERE delivered_at IS NULL AND dead_lettered_at IS NULL;

CREATE TABLE IF NOT EXISTS hauling_service_nonces (
  caller TEXT NOT NULL,
  nonce TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (caller, nonce)
);
