-- Section 10: Fleet private corporate taxi execution.
--
-- Dispatch and Haulage have carried Fleet's private-order execution since
-- Section 15; Taxi never did, which is why Fleet's adapter has been returning
-- "taxi private execution is disabled" rather than calling anything.
--
-- The shape mirrors what already works. There is deliberately no corporate
-- trip table and no parallel lifecycle: a private execution creates an ordinary
-- taxi_bookings row carrying the corporate metadata migration 012 already
-- added, so every lifecycle guard, payment rule and state transition that
-- protects an independent ride protects this one unchanged.
--
-- What this migration adds is only the MAPPING: the durable, idempotent link
-- between one Fleet execution operation and the taxi booking it produced.

CREATE TABLE IF NOT EXISTS taxi_private_execution_mappings (
    -- Fleet's own operation identity. Primary key because Fleet reads back by
    -- it after an ambiguous create, and that read must be exact.
    fleet_operation_id           TEXT PRIMARY KEY,

    -- Exactly-once. The key decides whether a retry is the same operation; the
    -- fingerprint decides whether it is still asking for the same thing. A
    -- retry that changed its mind is a conflict, never a second trip.
    idempotency_key              TEXT NOT NULL UNIQUE,
    request_fingerprint          TEXT NOT NULL,

    organization_id              TEXT NOT NULL,
    settlement_organization_id   TEXT NOT NULL,
    organization_membership_id   TEXT NOT NULL,

    -- Taxi's own identities, as real foreign keys: unlike Fleet, this service
    -- owns these rows and a mapping to a provider or vehicle that does not
    -- exist here would be a mapping to nothing.
    executing_provider_id        UUID NOT NULL REFERENCES taxi_providers(id),
    vehicle_id                   UUID NOT NULL REFERENCES vehicles(id),

    fleet_private_order_id       TEXT NOT NULL,
    fleet_shipment_id            TEXT NOT NULL,
    fleet_assignment_id          TEXT NOT NULL,
    fleet_provider_response_id   TEXT NOT NULL,

    service_category             TEXT NOT NULL DEFAULT 'TAXI'
                                   CHECK (service_category = 'TAXI'),
    contract_reference           TEXT NOT NULL DEFAULT '',
    quotation_reference          TEXT NOT NULL DEFAULT '',
    correlation_id               TEXT NOT NULL DEFAULT '',

    canonical_booking_id         UUID REFERENCES taxi_bookings(id),

    status                       TEXT NOT NULL DEFAULT 'CONFIRMED'
                                   CHECK (status IN ('CONFIRMED', 'CANCELLED', 'FAILED')),
    cancellation_status          TEXT NOT NULL DEFAULT 'NONE'
                                   CHECK (cancellation_status IN ('NONE', 'PENDING', 'CONFIRMED')),
    cancellation_idempotency_key TEXT,
    cancellation_fingerprint     TEXT,
    last_error_code              TEXT NOT NULL DEFAULT '',
    source_version               BIGINT NOT NULL DEFAULT 1 CHECK (source_version > 0),

    created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- A confirmed execution must point at the booking it created. Without this
    -- a crash between the two inserts could leave Fleet holding a confirmation
    -- for a trip that does not exist.
    CONSTRAINT chk_taxi_private_execution_confirmed_has_booking
        CHECK (status <> 'CONFIRMED' OR canonical_booking_id IS NOT NULL)
);

-- One Fleet assignment, one taxi execution. taxi_bookings already enforces
-- this for live bookings; repeating it here covers the mapping itself, which
-- outlives the booking's active window.
CREATE UNIQUE INDEX IF NOT EXISTS uq_taxi_private_execution_assignment
    ON taxi_private_execution_mappings (fleet_assignment_id)
    WHERE fleet_assignment_id <> '';

CREATE UNIQUE INDEX IF NOT EXISTS uq_taxi_private_execution_booking
    ON taxi_private_execution_mappings (canonical_booking_id)
    WHERE canonical_booking_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_taxi_private_execution_organization
    ON taxi_private_execution_mappings (organization_id, created_at DESC);

COMMENT ON TABLE taxi_private_execution_mappings IS
'Durable link between one Fleet private execution operation and the taxi booking it created. Taxi owns the booking; Fleet owns the corporate planning record that asked for it.';
