-- Section 10 (HLG-016, HLG-017, HLG-018, HLG-019, HLG-020): canonical Fleet
-- completed-work submission for organization-managed Hauling execution.
--
-- Corporate Hauling work runs on the NORMAL hauling booking engine. There is
-- deliberately no corporate_hauling_jobs table and no parallel lifecycle:
-- corporate identity is metadata attached to the same haulage_bookings row
-- every independent job already uses, so every existing lifecycle guard keeps
-- applying unchanged.

-- ---------------------------------------------------------------------------
-- Corporate execution metadata on the authoritative booking (HLG-016)
-- ---------------------------------------------------------------------------
ALTER TABLE haulage_bookings
    ADD COLUMN IF NOT EXISTS is_corporate_managed     BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS corporate_organization_id TEXT,
    ADD COLUMN IF NOT EXISTS membership_id            TEXT,
    ADD COLUMN IF NOT EXISTS fleet_assignment_id      TEXT,
    ADD COLUMN IF NOT EXISTS fleet_assignment_version BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS execution_mode           TEXT NOT NULL DEFAULT 'INDEPENDENT',
    ADD COLUMN IF NOT EXISTS settlement_mode          TEXT NOT NULL DEFAULT 'INDEPENDENT',
    ADD COLUMN IF NOT EXISTS completion_version       BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS fleet_correlation_id     TEXT;

-- A corporate booking is only usable as a settlement authority when it names
-- its organization AND membership. Enforced in the database so a partially
-- populated corporate booking can never reach the completed-work contract.
ALTER TABLE haulage_bookings DROP CONSTRAINT IF EXISTS chk_hauling_booking_corporate_complete;
ALTER TABLE haulage_bookings
    ADD CONSTRAINT chk_hauling_booking_corporate_complete CHECK (
        is_corporate_managed = FALSE
        OR (
            corporate_organization_id IS NOT NULL AND corporate_organization_id <> ''
            AND membership_id IS NOT NULL AND membership_id <> ''
        )
    );

ALTER TABLE haulage_bookings DROP CONSTRAINT IF EXISTS chk_hauling_booking_execution_mode;
ALTER TABLE haulage_bookings
    ADD CONSTRAINT chk_hauling_booking_execution_mode
        CHECK (execution_mode IN ('INDEPENDENT', 'ORGANIZATION_MANAGED'));

ALTER TABLE haulage_bookings DROP CONSTRAINT IF EXISTS chk_hauling_booking_settlement_mode;
ALTER TABLE haulage_bookings
    ADD CONSTRAINT chk_hauling_booking_settlement_mode
        CHECK (settlement_mode IN ('INDEPENDENT', 'ORGANIZATION_MANAGED'));

-- HLG-015: one Fleet assignment can never produce two live hauling
-- executions. Partial unique index so independent bookings (NULL assignment)
-- are unaffected, and a cancelled/unmatched booking releases the assignment
-- for a legitimate re-dispatch.
CREATE UNIQUE INDEX IF NOT EXISTS uq_hauling_bookings_fleet_assignment
    ON haulage_bookings (fleet_assignment_id)
    WHERE fleet_assignment_id IS NOT NULL
      AND status NOT IN ('cancelled', 'unmatched');

CREATE INDEX IF NOT EXISTS idx_hauling_bookings_organization
    ON haulage_bookings (corporate_organization_id, created_at DESC)
    WHERE corporate_organization_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Completed-work delivery on the EXISTING outbox (HLG-018, HLG-019, HLG-020)
-- ---------------------------------------------------------------------------
-- hauling_domain_outbox already carries leasing, attempts, backoff,
-- dead-lettering and last_error. Section 10 needs two more things from it: a
-- delivery TARGET (so Fleet completed-work rows and the existing Fleet domain
-- events share one durable mechanism without being delivered to the wrong
-- endpoint), and an explicit reconciliation state an operator can query and
-- repair without hand-written SQL.
ALTER TABLE hauling_domain_outbox
    ADD COLUMN IF NOT EXISTS target               TEXT NOT NULL DEFAULT 'fleet-events',
    ADD COLUMN IF NOT EXISTS reconciliation_state TEXT NOT NULL DEFAULT 'PENDING',
    ADD COLUMN IF NOT EXISTS last_error_code      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS idempotency_key      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS next_retry_at        TIMESTAMPTZ;

ALTER TABLE hauling_domain_outbox DROP CONSTRAINT IF EXISTS chk_hauling_outbox_reconciliation_state;
ALTER TABLE hauling_domain_outbox
    ADD CONSTRAINT chk_hauling_outbox_reconciliation_state CHECK (
        reconciliation_state IN ('PENDING', 'DELIVERED', 'RETRY_SCHEDULED',
                                 'FAILED_TERMINAL', 'RECONCILIATION_REQUIRED')
    );

-- HLG-019: one completed-work row per idempotency identity. An ambiguous
-- Fleet response is retried with the SAME key, never re-enqueued under a new
-- one, so this index is what makes "exactly once" structural rather than a
-- property of the worker's own care.
CREATE UNIQUE INDEX IF NOT EXISTS uq_hauling_outbox_idempotency
    ON hauling_domain_outbox (target, idempotency_key)
    WHERE idempotency_key <> '';

CREATE INDEX IF NOT EXISTS idx_hauling_outbox_target_ready
    ON hauling_domain_outbox (target, available_at)
    WHERE delivered_at IS NULL AND dead_lettered_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_hauling_outbox_reconciliation
    ON hauling_domain_outbox (target, reconciliation_state);
