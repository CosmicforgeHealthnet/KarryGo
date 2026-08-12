-- Section 11: durable fleet execution-start work.
--
-- When a provider accepts, three things must become true together: the
-- immutable acceptance is stored, the assignment moves to ACCEPTED, and the
-- work of asking the source service to create the canonical trip is durably
-- queued. If the third were merely an in-memory follow-up, a process restart
-- between acceptance and execution would strand a provider who had said yes.
--
-- Additive: no existing table is altered.

CREATE TABLE IF NOT EXISTS fleet_partner_execution_start_operations (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE CASCADE,
    assignment_id TEXT NOT NULL
        REFERENCES fleet_partner_provider_assignments(id) ON DELETE CASCADE,
    provider_response_id TEXT NOT NULL,
    request_projection_id TEXT NOT NULL
        REFERENCES fleet_partner_marketplace_request_projections(id) ON DELETE CASCADE,
    source_service TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    attempt_count INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_owner TEXT NULL,
    lease_expires_at TIMESTAMPTZ NULL,
    dispatch_response_snapshot JSONB NULL,
    trip_id TEXT NULL,
    last_error TEXT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ NULL,
    CONSTRAINT chk_execution_start_status CHECK (
        status IN ('PENDING', 'CLAIMED', 'EXECUTION_START_PENDING', 'CONFIRMED',
                   'REJECTED', 'UNKNOWN_OUTCOME', 'RETRY_SCHEDULED',
                   'RECONCILIATION_REQUIRED', 'DEAD_LETTER')
    ),
    CONSTRAINT chk_execution_start_attempts CHECK (attempt_count >= 0)
);

-- Exactly one execution-start operation per assignment. Two workers, or a
-- retried acceptance, cannot queue the canonical trip twice.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_execution_start_assignment
    ON fleet_partner_execution_start_operations (assignment_id);

-- The idempotency key is reused verbatim on every retry and read-back, so a
-- lost dispatch response resolves to the SAME command rather than a new one.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_execution_start_idempotency
    ON fleet_partner_execution_start_operations (idempotency_key);

CREATE INDEX IF NOT EXISTS idx_fleet_execution_start_due
    ON fleet_partner_execution_start_operations (next_attempt_at)
    WHERE status IN ('PENDING', 'RETRY_SCHEDULED', 'UNKNOWN_OUTCOME');

CREATE INDEX IF NOT EXISTS idx_fleet_execution_start_lease
    ON fleet_partner_execution_start_operations (lease_expires_at)
    WHERE status IN ('CLAIMED', 'EXECUTION_START_PENDING');

CREATE INDEX IF NOT EXISTS idx_fleet_execution_start_org
    ON fleet_partner_execution_start_operations (organization_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_fleet_execution_start_dead_letter
    ON fleet_partner_execution_start_operations (created_at DESC)
    WHERE status = 'DEAD_LETTER';
