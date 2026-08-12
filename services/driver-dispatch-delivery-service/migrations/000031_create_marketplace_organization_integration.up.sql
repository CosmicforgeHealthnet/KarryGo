-- Section 11/12: organization-managed marketplace integration.
--
-- Everything here is ADDITIVE. The existing customer booking flow and the
-- existing independent-provider flow must keep working untouched:
--   * every new column on an existing table is nullable or has a default;
--   * no existing constraint is dropped or narrowed;
--   * no existing index is removed.
-- An independent-provider trip simply leaves the organization columns NULL.

-- ---------------------------------------------------------------------------
-- Source versioning on the canonical request.
--
-- Fleet sends expectedRequestVersion and orders projections by source version.
-- request_broadcasts had no version column, so add one defaulted to 1: every
-- existing row becomes version 1 rather than NULL, which keeps the optimistic
-- concurrency comparison total.
-- ---------------------------------------------------------------------------
ALTER TABLE request_broadcasts
    ADD COLUMN IF NOT EXISTS source_version BIGINT NOT NULL DEFAULT 1;

ALTER TABLE request_broadcasts
    ADD COLUMN IF NOT EXISTS reserved_organization_id TEXT NULL;

ALTER TABLE request_broadcasts
    ADD COLUMN IF NOT EXISTS reservation_status TEXT NOT NULL DEFAULT 'NONE';

ALTER TABLE request_broadcasts
    DROP CONSTRAINT IF EXISTS chk_broadcast_reservation_status;
ALTER TABLE request_broadcasts
    ADD CONSTRAINT chk_broadcast_reservation_status CHECK (
        reservation_status IN ('NONE', 'PENDING', 'RESERVED', 'RELEASED', 'EXECUTED')
    );

CREATE INDEX IF NOT EXISTS idx_broadcasts_reserved_org
    ON request_broadcasts (reserved_organization_id)
    WHERE reserved_organization_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Organization-managed execution metadata on the canonical trip.
--
-- settlement_organization_id and executing_provider_id are deliberately
-- SEPARATE columns. Collapsing them would either bill the provider or leak the
-- organization's settlement relationship into provider-facing data. provider_id
-- remains the existing executing-provider FK and is unchanged; the organization
-- columns describe who the platform settles with, which is a different party.
-- ---------------------------------------------------------------------------
ALTER TABLE trips ADD COLUMN IF NOT EXISTS organization_id TEXT NULL;
ALTER TABLE trips ADD COLUMN IF NOT EXISTS settlement_organization_id TEXT NULL;
ALTER TABLE trips ADD COLUMN IF NOT EXISTS organization_membership_id TEXT NULL;
ALTER TABLE trips ADD COLUMN IF NOT EXISTS fleet_assignment_id TEXT NULL;
ALTER TABLE trips ADD COLUMN IF NOT EXISTS provider_response_id TEXT NULL;
ALTER TABLE trips ADD COLUMN IF NOT EXISTS acceptance_source TEXT NULL;
ALTER TABLE trips ADD COLUMN IF NOT EXISTS execution_source TEXT NOT NULL DEFAULT 'MARKETPLACE_REQUEST';
ALTER TABLE trips ADD COLUMN IF NOT EXISTS execution_mode TEXT NOT NULL DEFAULT 'INDEPENDENT_PROVIDER';
ALTER TABLE trips ADD COLUMN IF NOT EXISTS fleet_correlation_id TEXT NULL;
ALTER TABLE trips ADD COLUMN IF NOT EXISTS source_version BIGINT NOT NULL DEFAULT 1;

ALTER TABLE trips DROP CONSTRAINT IF EXISTS chk_trip_execution_mode;
ALTER TABLE trips
    ADD CONSTRAINT chk_trip_execution_mode CHECK (
        execution_mode IN ('INDEPENDENT_PROVIDER', 'ORGANIZATION_MANAGED')
    );

ALTER TABLE trips DROP CONSTRAINT IF EXISTS chk_trip_acceptance_source;
ALTER TABLE trips
    ADD CONSTRAINT chk_trip_acceptance_source CHECK (
        acceptance_source IS NULL
        OR acceptance_source IN ('ORGANIZATION_DISPATCHER', 'PROVIDER_DIRECT', 'INDEPENDENT_PROVIDER')
    );

-- An organization-managed trip must carry its full provenance. This is the
-- constraint that makes "no execution without a recorded provider acceptance"
-- a database fact rather than an application convention: an ORGANIZATION_MANAGED
-- trip cannot exist without a fleet assignment AND the provider's response id.
ALTER TABLE trips DROP CONSTRAINT IF EXISTS chk_trip_organization_managed_complete;
ALTER TABLE trips
    ADD CONSTRAINT chk_trip_organization_managed_complete CHECK (
        execution_mode <> 'ORGANIZATION_MANAGED'
        OR (
            organization_id IS NOT NULL
            AND settlement_organization_id IS NOT NULL
            AND organization_membership_id IS NOT NULL
            AND fleet_assignment_id IS NOT NULL
            AND provider_response_id IS NOT NULL
            AND acceptance_source IS NOT NULL
        )
    );

-- Exactly one canonical trip per fleet assignment. This is what makes the
-- execution-creation endpoint safe to retry after a lost response: a duplicate
-- submission loses on this index instead of creating a second trip.
CREATE UNIQUE INDEX IF NOT EXISTS uq_trips_fleet_assignment
    ON trips (fleet_assignment_id)
    WHERE fleet_assignment_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_trips_organization
    ON trips (organization_id, created_at DESC)
    WHERE organization_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Organization reservations.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS dispatch_organization_reservations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id UUID NOT NULL REFERENCES request_broadcasts(id) ON DELETE RESTRICT,
    organization_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    acceptance_source TEXT NOT NULL,
    accepting_provider_id UUID NULL REFERENCES providers(id),
    organization_membership_id TEXT NULL,
    source_request_version BIGINT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    reserved_at TIMESTAMPTZ NULL,
    released_at TIMESTAMPTZ NULL,
    release_reason TEXT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_reservation_status CHECK (
        status IN ('PENDING', 'RESERVED', 'RELEASED', 'REJECTED',
                   'CANCELLED', 'EXPIRED', 'RECONCILIATION_REQUIRED')
    ),
    CONSTRAINT chk_reservation_acceptance_source CHECK (
        acceptance_source IN ('ORGANIZATION_DISPATCHER', 'PROVIDER_DIRECT')
    ),
    -- A provider-direct reservation must name the provider who accepted; a
    -- dispatcher reservation must not, because no provider has decided yet.
    CONSTRAINT chk_reservation_provider_direct CHECK (
        (acceptance_source = 'PROVIDER_DIRECT' AND accepting_provider_id IS NOT NULL)
        OR (acceptance_source = 'ORGANIZATION_DISPATCHER' AND accepting_provider_id IS NULL)
    )
);

-- Only ONE organization may hold a live reservation on a request. Two
-- organizations racing resolve here, in the database, and the loser gets a
-- definite request_already_claimed rather than an ambiguous outcome.
CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_reservation_live
    ON dispatch_organization_reservations (request_id)
    WHERE status IN ('PENDING', 'RESERVED');

CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_reservation_idempotency
    ON dispatch_organization_reservations (idempotency_key);

CREATE INDEX IF NOT EXISTS idx_dispatch_reservation_org
    ON dispatch_organization_reservations (organization_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_dispatch_reservation_request
    ON dispatch_organization_reservations (request_id);

-- ---------------------------------------------------------------------------
-- Durable command operations (the dispatch side of unknown-outcome handling).
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS dispatch_marketplace_command_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_type TEXT NOT NULL,
    request_id UUID NULL REFERENCES request_broadcasts(id) ON DELETE SET NULL,
    trip_id UUID NULL REFERENCES trips(id) ON DELETE SET NULL,
    fleet_assignment_id TEXT NULL,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    response_snapshot JSONB NULL,
    error_code TEXT NULL,
    attempt_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_command_operation_type CHECK (
        operation_type IN ('RESERVE_REQUEST', 'RELEASE_RESERVATION', 'VALIDATE_PROVIDER',
                           'VALIDATE_VEHICLE', 'CREATE_EXECUTION', 'CANCEL_EXECUTION')
    ),
    CONSTRAINT chk_command_status CHECK (
        status IN ('PENDING', 'CONFIRMED', 'REJECTED', 'UNKNOWN_OUTCOME', 'FAILED')
    ),
    CONSTRAINT chk_command_attempts CHECK (attempt_count >= 0)
);

-- The read-back key. A replayed command with the same key returns the stored
-- response snapshot instead of executing again.
CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_command_idempotency
    ON dispatch_marketplace_command_operations (operation_type, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_dispatch_command_assignment
    ON dispatch_marketplace_command_operations (fleet_assignment_id)
    WHERE fleet_assignment_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Transactional outbox.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS dispatch_marketplace_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type TEXT NOT NULL,
    schema_version INT NOT NULL DEFAULT 1,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    aggregate_version BIGINT NOT NULL,
    payload JSONB NOT NULL,
    payload_hash TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'PENDING',
    attempt_count INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_owner TEXT NULL,
    lease_expires_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ NULL,
    last_error TEXT NULL,
    CONSTRAINT chk_outbox_status CHECK (
        status IN ('PENDING', 'IN_FLIGHT', 'PUBLISHED', 'FAILED', 'DEAD_LETTER')
    ),
    CONSTRAINT chk_outbox_attempts CHECK (attempt_count >= 0)
);

-- Pending-delivery scan: the worker's hot path.
CREATE INDEX IF NOT EXISTS idx_dispatch_outbox_pending
    ON dispatch_marketplace_outbox (next_attempt_at)
    WHERE status IN ('PENDING', 'FAILED');

CREATE INDEX IF NOT EXISTS idx_dispatch_outbox_lease
    ON dispatch_marketplace_outbox (lease_expires_at)
    WHERE status = 'IN_FLIGHT';

CREATE INDEX IF NOT EXISTS idx_dispatch_outbox_aggregate
    ON dispatch_marketplace_outbox (aggregate_type, aggregate_id, aggregate_version);

CREATE INDEX IF NOT EXISTS idx_dispatch_outbox_dead_letter
    ON dispatch_marketplace_outbox (created_at DESC)
    WHERE status = 'DEAD_LETTER';

-- ---------------------------------------------------------------------------
-- Delivery attempts, kept separate from the event so retry history survives.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS dispatch_marketplace_outbox_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    outbox_id UUID NOT NULL REFERENCES dispatch_marketplace_outbox(id) ON DELETE CASCADE,
    target_service TEXT NOT NULL,
    attempt_number INT NOT NULL,
    http_status INT NULL,
    error_code TEXT NULL,
    duration_ms INT NULL,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_delivery_attempt CHECK (attempt_number > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_delivery_attempt
    ON dispatch_marketplace_outbox_deliveries (outbox_id, target_service, attempt_number);

CREATE INDEX IF NOT EXISTS idx_dispatch_delivery_outbox
    ON dispatch_marketplace_outbox_deliveries (outbox_id, attempted_at DESC);
