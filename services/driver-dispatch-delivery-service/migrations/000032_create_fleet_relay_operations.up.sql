-- Section 11: durable dispatch-to-fleet provider relay.
--
-- A provider's Accept or Decline is a decision a human made. It must never be
-- lost because fleet happened to be unreachable at that instant, and it must
-- never be delivered twice as two separate decisions. This table is the
-- durable record of that decision and of our attempts to relay it.
--
-- Additive: no existing table is altered.

CREATE TABLE IF NOT EXISTS dispatch_fleet_relay_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_type TEXT NOT NULL,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE RESTRICT,
    request_id UUID NULL REFERENCES request_broadcasts(id) ON DELETE SET NULL,
    offer_id TEXT NULL,
    assignment_id TEXT NULL,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    payload JSONB NOT NULL,
    payload_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    attempt_count INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_owner TEXT NULL,
    lease_expires_at TIMESTAMPTZ NULL,
    response_snapshot JSONB NULL,
    last_error TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ NULL,
    CONSTRAINT chk_relay_operation_type CHECK (
        operation_type IN ('PROVIDER_OFFER_ACCEPT', 'PROVIDER_OFFER_DECLINE',
                           'PROVIDER_ASSIGNMENT_ACCEPT', 'PROVIDER_ASSIGNMENT_DECLINE')
    ),
    CONSTRAINT chk_relay_status CHECK (
        status IN ('PENDING', 'DELIVERING', 'CONFIRMED', 'REJECTED',
                   'UNKNOWN_OUTCOME', 'RETRY_SCHEDULED', 'DEAD_LETTER')
    ),
    CONSTRAINT chk_relay_attempts CHECK (attempt_count >= 0),
    -- An offer relay must name an offer; an assignment relay must name an
    -- assignment. Without this a relay could be persisted that no worker
    -- could ever route.
    CONSTRAINT chk_relay_target CHECK (
        (operation_type IN ('PROVIDER_OFFER_ACCEPT', 'PROVIDER_OFFER_DECLINE')
         AND offer_id IS NOT NULL)
        OR
        (operation_type IN ('PROVIDER_ASSIGNMENT_ACCEPT', 'PROVIDER_ASSIGNMENT_DECLINE')
         AND assignment_id IS NOT NULL)
    )
);

-- One relay per idempotency key. A retried provider tap reuses the key and
-- therefore reuses the ORIGINAL decision rather than creating a second one.
CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_relay_idempotency
    ON dispatch_fleet_relay_operations (operation_type, idempotency_key);

-- At most one live relay per provider decision target, so two concurrent taps
-- cannot both become in-flight relays for the same offer or assignment.
CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_relay_live_assignment
    ON dispatch_fleet_relay_operations (assignment_id)
    WHERE assignment_id IS NOT NULL
      AND status IN ('PENDING', 'DELIVERING', 'RETRY_SCHEDULED', 'UNKNOWN_OUTCOME');

CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_relay_live_offer
    ON dispatch_fleet_relay_operations (offer_id, provider_id)
    WHERE offer_id IS NOT NULL
      AND status IN ('PENDING', 'DELIVERING', 'RETRY_SCHEDULED', 'UNKNOWN_OUTCOME');

-- Worker hot path.
CREATE INDEX IF NOT EXISTS idx_dispatch_relay_pending
    ON dispatch_fleet_relay_operations (next_attempt_at)
    WHERE status IN ('PENDING', 'RETRY_SCHEDULED', 'UNKNOWN_OUTCOME');

CREATE INDEX IF NOT EXISTS idx_dispatch_relay_lease
    ON dispatch_fleet_relay_operations (lease_expires_at)
    WHERE status = 'DELIVERING';

CREATE INDEX IF NOT EXISTS idx_dispatch_relay_dead_letter
    ON dispatch_fleet_relay_operations (created_at DESC)
    WHERE status = 'DEAD_LETTER';

CREATE INDEX IF NOT EXISTS idx_dispatch_relay_provider
    ON dispatch_fleet_relay_operations (provider_id, created_at DESC);
