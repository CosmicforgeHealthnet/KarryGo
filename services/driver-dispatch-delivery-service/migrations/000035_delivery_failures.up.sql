-- WP6.4: failed delivery as authoritative dispatch lifecycle data.
--
-- The trip state machine has always had a `failed` status and nothing has ever
-- been able to reach it: no transition, no route, no record. A delivery that
-- genuinely could not be completed had to be recorded as a cancellation, which
-- is a different fact with different downstream consequences — cancellation
-- frees capacity and can carry a penalty, failure does neither by itself.
--
-- This adds the missing transition's evidence table. It deliberately does NOT
-- add a status: `failed` already exists in validTransitions from every live
-- state, so duplicating the state machine would be the mistake, not the fix.

CREATE TABLE IF NOT EXISTS delivery_failures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id UUID NOT NULL REFERENCES trips(id) ON DELETE CASCADE,
    -- The specific reason, and the coarse category consumers branch on. Both
    -- are stored: a category alone loses the detail an operator needs, and a
    -- reason alone forces every consumer to maintain its own mapping.
    reason_code TEXT NOT NULL,
    reason_category TEXT NOT NULL
        CHECK (reason_category IN ('RECIPIENT', 'ADDRESS', 'PACKAGE', 'PROVIDER', 'EXTERNAL', 'OTHER')),
    -- Free text is permitted but capped and treated as operator-facing only.
    -- It never travels on an event payload.
    operator_notes TEXT NULL CHECK (operator_notes IS NULL OR length(operator_notes) <= 500),
    from_status TEXT NOT NULL,
    failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    responsible_actor_type TEXT NOT NULL
        CHECK (responsible_actor_type IN ('provider', 'customer', 'system', 'admin')),
    responsible_actor_id TEXT NOT NULL,
    -- Whether another attempt is permitted. Server-derived from the reason, so
    -- a client cannot declare its own failure re-attemptable.
    retry_eligible BOOLEAN NOT NULL DEFAULT false,
    -- The specific "nobody was there" classification, which downstream
    -- workflows treat differently from every other failure.
    recipient_unavailable BOOLEAN NOT NULL DEFAULT false,
    -- Evidence of the attempt, when the provider captured any.
    attempt_reference TEXT NULL,
    -- Opaque pointer to a case in the support-dispute service. No foreign key:
    -- that service owns it.
    incident_reference TEXT NULL,
    -- Command envelope.
    expected_source_version BIGINT NOT NULL DEFAULT 0,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    causation_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Exactly one failure record per trip; `failed` is terminal.
CREATE UNIQUE INDEX IF NOT EXISTS uq_delivery_failure_trip
    ON delivery_failures (trip_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_delivery_failure_idempotency
    ON delivery_failures (idempotency_key);

CREATE INDEX IF NOT EXISTS idx_delivery_failure_category
    ON delivery_failures (reason_category, failed_at DESC);

CREATE INDEX IF NOT EXISTS idx_delivery_failure_retry
    ON delivery_failures (failed_at DESC)
    WHERE retry_eligible;

ALTER TABLE trips
    ADD COLUMN IF NOT EXISTS failed_at TIMESTAMPTZ NULL;
