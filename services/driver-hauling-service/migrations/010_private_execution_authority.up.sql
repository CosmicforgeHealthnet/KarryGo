-- Section 15 private-order execution authority. Fleet identifiers are opaque
-- TEXT values and intentionally have no foreign keys into another service.
-- Migration 008 predated Fleet's opaque identifier contract and used UUID for
-- organization-owned references.  These columns have no local FK target, so
-- convert them losslessly to TEXT before accepting real org_/mem_/brn_ IDs.
ALTER TABLE hauling_organization_provider_links
  ALTER COLUMN organization_id TYPE TEXT USING organization_id::text,
  ALTER COLUMN membership_id TYPE TEXT USING membership_id::text,
  ALTER COLUMN branch_id TYPE TEXT USING branch_id::text;
ALTER TABLE hauling_organization_truck_links
  ALTER COLUMN organization_id TYPE TEXT USING organization_id::text,
  ALTER COLUMN membership_id TYPE TEXT USING membership_id::text,
  ALTER COLUMN branch_id TYPE TEXT USING branch_id::text;
ALTER TABLE hauling_organization_link_history
  ALTER COLUMN organization_id TYPE TEXT USING organization_id::text,
  ALTER COLUMN branch_id TYPE TEXT USING branch_id::text;

CREATE TABLE IF NOT EXISTS hauling_private_execution_mappings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  fleet_operation_id TEXT NOT NULL UNIQUE,
  idempotency_key TEXT NOT NULL UNIQUE,
  request_fingerprint TEXT NOT NULL,
  organization_id TEXT NOT NULL,
  settlement_organization_id TEXT NOT NULL,
  executing_provider_id UUID NOT NULL REFERENCES truck_providers(id) ON DELETE RESTRICT,
  organization_membership_id TEXT NOT NULL,
  fleet_private_order_id TEXT NOT NULL,
  fleet_shipment_id TEXT NOT NULL,
  fleet_assignment_id TEXT NOT NULL UNIQUE,
  fleet_provider_response_id TEXT NOT NULL,
  truck_id UUID NOT NULL REFERENCES trucks(id) ON DELETE RESTRICT,
  service_category TEXT NOT NULL CHECK(service_category='HAULING'),
  contract_reference TEXT NOT NULL DEFAULT '',
  quotation_reference TEXT NOT NULL DEFAULT '',
  correlation_id TEXT NOT NULL DEFAULT '',
  canonical_booking_id UUID REFERENCES haulage_bookings(id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'PREPARING' CHECK(status IN(
    'PREPARING','CONFIRMED','IN_PROGRESS','COMPLETED','FAILED','CANCELLED','OUTCOME_UNKNOWN')),
  source_version BIGINT NOT NULL DEFAULT 0 CHECK(source_version>=0),
  cancellation_status TEXT NOT NULL DEFAULT 'NONE' CHECK(cancellation_status IN(
    'NONE','PENDING','CONFIRMED','REJECTED','OUTCOME_UNKNOWN')),
  cancellation_idempotency_key TEXT,
  cancellation_fingerprint TEXT,
  last_error_code TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_hauling_private_booking
  ON hauling_private_execution_mappings(canonical_booking_id)
  WHERE canonical_booking_id IS NOT NULL;
