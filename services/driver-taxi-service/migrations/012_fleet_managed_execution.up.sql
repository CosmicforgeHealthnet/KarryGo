-- Section 8 (TAX-013, TAX-014, TAX-014A, TAX-016, TAX-016A, TAX-017):
-- organization-managed taxi execution.
--
-- Corporate work runs on the NORMAL taxi trip engine. There is deliberately
-- no corporate_taxi_trips table and no parallel lifecycle: corporate identity
-- is metadata attached to the same taxi_bookings row every independent ride
-- already uses, so every existing lifecycle guard keeps applying unchanged.

-- ---------------------------------------------------------------------------
-- Corporate metadata on the authoritative booking (TAX-014)
-- ---------------------------------------------------------------------------
ALTER TABLE taxi_bookings
    ADD COLUMN IF NOT EXISTS is_corporate_managed   BOOLEAN     NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS organization_id        TEXT,
    ADD COLUMN IF NOT EXISTS membership_id          TEXT,
    ADD COLUMN IF NOT EXISTS fleet_assignment_id    TEXT,
    ADD COLUMN IF NOT EXISTS fleet_assignment_version BIGINT   NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS source_context         TEXT       NOT NULL DEFAULT 'INDEPENDENT',
    ADD COLUMN IF NOT EXISTS settlement_mode        TEXT       NOT NULL DEFAULT 'INDEPENDENT',
    ADD COLUMN IF NOT EXISTS source_version         BIGINT     NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS fleet_correlation_id   TEXT;

-- A corporate booking is only usable as a settlement authority when it names
-- its organization AND membership. Enforced in the database so a partially
-- populated corporate booking can never reach the completed-work contract.
ALTER TABLE taxi_bookings DROP CONSTRAINT IF EXISTS chk_taxi_booking_corporate_complete;
ALTER TABLE taxi_bookings
    ADD CONSTRAINT chk_taxi_booking_corporate_complete CHECK (
        is_corporate_managed = FALSE
        OR (
            organization_id IS NOT NULL AND organization_id <> ''
            AND membership_id IS NOT NULL AND membership_id <> ''
            AND fleet_assignment_id IS NOT NULL AND fleet_assignment_id <> ''
        )
    );

ALTER TABLE taxi_bookings DROP CONSTRAINT IF EXISTS chk_taxi_booking_settlement_mode;
ALTER TABLE taxi_bookings
    ADD CONSTRAINT chk_taxi_booking_settlement_mode
        CHECK (settlement_mode IN ('INDEPENDENT', 'ORGANIZATION_MANAGED'));

ALTER TABLE taxi_bookings DROP CONSTRAINT IF EXISTS chk_taxi_booking_source_context;
ALTER TABLE taxi_bookings
    ADD CONSTRAINT chk_taxi_booking_source_context
        CHECK (source_context IN ('INDEPENDENT', 'FLEET_ASSIGNMENT', 'FLEET_OFFER', 'FLEET_PRIVATE_EXECUTION'));

-- TAX-014A: one Fleet assignment can never produce two taxi executions.
-- Partial unique index rather than a table constraint so independent bookings
-- (NULL assignment) are unaffected, and a cancelled/unmatched booking releases
-- the assignment for a legitimate re-dispatch.
CREATE UNIQUE INDEX IF NOT EXISTS uq_taxi_bookings_fleet_assignment
    ON taxi_bookings (fleet_assignment_id)
    WHERE fleet_assignment_id IS NOT NULL
      AND status NOT IN ('cancelled', 'unmatched');

CREATE INDEX IF NOT EXISTS idx_taxi_bookings_organization
    ON taxi_bookings (organization_id, created_at DESC)
    WHERE organization_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Execution reservation (TAX-013)
-- ---------------------------------------------------------------------------
-- Fleet reserves a taxi job BEFORE any provider is bound to it. The
-- reservation is what makes the whole flow safe against Fleet retries: the
-- same idempotency key always resolves to the same reservation instead of
-- creating a second one, and the partial unique index below makes "one live
-- reservation per booking" a database fact rather than an application hope.
CREATE TABLE IF NOT EXISTS taxi_fleet_execution_reservations (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id            UUID        NOT NULL,
    source_request_id     TEXT        NOT NULL,
    fleet_assignment_id   TEXT        NOT NULL DEFAULT '',
    provider_id           TEXT        NOT NULL DEFAULT '',
    organization_id       TEXT        NOT NULL,
    membership_id         TEXT        NOT NULL DEFAULT '',
    acceptance_source     TEXT        NOT NULL DEFAULT 'DISPATCHER',
    status                TEXT        NOT NULL,
    expected_source_version BIGINT    NOT NULL DEFAULT 0,
    source_version        BIGINT      NOT NULL DEFAULT 1,
    version               BIGINT      NOT NULL DEFAULT 1,
    idempotency_key       TEXT        NOT NULL,
    request_fingerprint   TEXT        NOT NULL DEFAULT '',
    correlation_id        TEXT        NOT NULL DEFAULT '',
    execution_booking_id  UUID,
    expires_at            TIMESTAMPTZ,
    released_at           TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_taxi_reservation_status CHECK (
        status IN ('RESERVED', 'CONFIRMED', 'RELEASED', 'EXPIRED', 'CANCELLED')
    ),
    CONSTRAINT uq_taxi_reservation_idempotency UNIQUE (idempotency_key)
);

-- Exactly one live reservation per booking: the guard against two providers
-- executing one job, and against a repeated Fleet retry double-reserving.
CREATE UNIQUE INDEX IF NOT EXISTS uq_taxi_reservation_live_booking
    ON taxi_fleet_execution_reservations (booking_id)
    WHERE status IN ('RESERVED', 'CONFIRMED');

CREATE INDEX IF NOT EXISTS idx_taxi_reservation_source_request
    ON taxi_fleet_execution_reservations (source_request_id);

-- ---------------------------------------------------------------------------
-- Completed-work outbox extension (TAX-016, TAX-016A, TAX-017)
-- ---------------------------------------------------------------------------
-- The existing taxi_outbox already carries leasing, attempts, backoff,
-- dead-lettering and last_error. Sections 8 needs two more things from it:
-- a delivery TARGET (so Fleet completed-work rows and Vendor lifecycle rows
-- share one durable mechanism without being delivered to each other), and an
-- explicit reconciliation state an operator can query and repair.
ALTER TABLE taxi_outbox
    ADD COLUMN IF NOT EXISTS target               TEXT NOT NULL DEFAULT 'vendor-service',
    ADD COLUMN IF NOT EXISTS reconciliation_state TEXT NOT NULL DEFAULT 'PENDING',
    ADD COLUMN IF NOT EXISTS last_error_code      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS idempotency_key      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS next_retry_at        TIMESTAMPTZ;

ALTER TABLE taxi_outbox DROP CONSTRAINT IF EXISTS chk_taxi_outbox_reconciliation_state;
ALTER TABLE taxi_outbox
    ADD CONSTRAINT chk_taxi_outbox_reconciliation_state CHECK (
        reconciliation_state IN ('PENDING', 'DELIVERED', 'RETRY_SCHEDULED',
                                 'FAILED_TERMINAL', 'RECONCILIATION_REQUIRED')
    );

-- One completed-work row per idempotency identity. TAX-016A: an ambiguous
-- Fleet response is retried with the SAME key, never re-enqueued under a new
-- one, so this index is what makes "exactly once" structural.
CREATE UNIQUE INDEX IF NOT EXISTS uq_taxi_outbox_idempotency
    ON taxi_outbox (target, idempotency_key)
    WHERE idempotency_key <> '';

CREATE INDEX IF NOT EXISTS idx_taxi_outbox_target_ready
    ON taxi_outbox (target, available_at)
    WHERE delivered_at IS NULL AND dead_lettered_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_taxi_outbox_reconciliation
    ON taxi_outbox (target, reconciliation_state);
