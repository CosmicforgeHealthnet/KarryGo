-- Section 15 private-order execution authority.
-- Fleet identifiers are deliberately opaque TEXT values.  Only canonical
-- Dispatch identifiers have local foreign keys; there are no cross-service
-- database dependencies.
CREATE TABLE IF NOT EXISTS dispatch_private_execution_mappings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fleet_operation_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    organization_id TEXT NOT NULL,
    settlement_organization_id TEXT NOT NULL,
    executing_provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    organization_membership_id TEXT NOT NULL,
    fleet_private_order_id TEXT NOT NULL,
    fleet_shipment_id TEXT NOT NULL,
    fleet_assignment_id TEXT NOT NULL,
    fleet_provider_response_id TEXT NOT NULL,
    vehicle_id UUID NULL REFERENCES bikes(id) ON DELETE RESTRICT,
    service_category TEXT NOT NULL,
    contract_reference TEXT NOT NULL DEFAULT '',
    quotation_reference TEXT NOT NULL DEFAULT '',
    correlation_id TEXT NOT NULL DEFAULT '',
    canonical_request_id UUID NULL REFERENCES request_broadcasts(id) ON DELETE RESTRICT,
    canonical_reservation_id UUID NULL REFERENCES dispatch_organization_reservations(id) ON DELETE RESTRICT,
    canonical_trip_id UUID NULL REFERENCES trips(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'PREPARING',
    source_version BIGINT NOT NULL DEFAULT 0 CHECK (source_version >= 0),
    cancellation_status TEXT NOT NULL DEFAULT 'NONE',
    cancellation_idempotency_key TEXT NULL,
    cancellation_fingerprint TEXT NULL,
    last_error_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_dispatch_private_execution_status CHECK (
        status IN ('PREPARING','READY','CONFIRMED','IN_PROGRESS','COMPLETED','FAILED','CANCELLED','OUTCOME_UNKNOWN')
    ),
    CONSTRAINT chk_dispatch_private_cancellation_status CHECK (
        cancellation_status IN ('NONE','PENDING','CONFIRMED','REJECTED','OUTCOME_UNKNOWN')
    ),
    CONSTRAINT uq_dispatch_private_operation UNIQUE (fleet_operation_id),
    CONSTRAINT uq_dispatch_private_idempotency UNIQUE (idempotency_key),
    CONSTRAINT uq_dispatch_private_assignment UNIQUE (fleet_assignment_id)
);

CREATE INDEX IF NOT EXISTS idx_dispatch_private_request
    ON dispatch_private_execution_mappings(canonical_request_id)
    WHERE canonical_request_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_dispatch_private_trip
    ON dispatch_private_execution_mappings(canonical_trip_id)
    WHERE canonical_trip_id IS NOT NULL;
