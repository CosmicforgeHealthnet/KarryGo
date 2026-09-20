-- Section 9: Haulage-owned recovery after assignment.
--
-- Until now Hauling's only recovery was RunMatchRecoveryWorker, which re-adopts
-- a booking whose MATCHER goroutine was lost. That covers the window before a
-- provider accepts and nothing after it. Once a provider has accepted -- and
-- especially once they are holding the cargo -- a breakdown had no represented
-- outcome at all: the booking simply sat in an in-trip status with nobody
-- driving it.
--
-- Section 9 puts that recovery in Haulage, not in Fleet and not in Vendor,
-- because the only service that can decide whether a REPLACEMENT truck can
-- carry this cargo is the one that owns the cargo requirement and the fit
-- engine.
--
-- The state vocabulary is deliberately the one driver-taxi-service already
-- uses (taxi_recovery_cases, migration 014). A second spelling of the same
-- lifecycle would make a cross-authority operator console impossible, and the
-- vocabulary is not improved by being reinvented.

CREATE TABLE IF NOT EXISTS hauling_recovery_cases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    booking_id UUID NOT NULL REFERENCES haulage_bookings(id) ON DELETE RESTRICT,

    -- The commerce mission this booking executes, when it has one. Recovery
    -- never creates a new mission identity: a replacement continues the SAME
    -- work, which is what keeps the customer's payment reference, evidence and
    -- handover binding valid across it.
    mission_ref TEXT NOT NULL DEFAULT '',

    trigger_code TEXT NOT NULL,

    state TEXT NOT NULL CHECK (state IN (
        'DETECTED', 'EVALUATING', 'REPLACEMENT_SEARCH', 'TRANSFER_REQUIRED',
        'IN_PROGRESS', 'RESOLVED', 'OPS_REQUIRED', 'FAILED')),

    original_provider_id UUID REFERENCES truck_providers(id),
    original_truck_id UUID REFERENCES trucks(id),
    replacement_provider_id UUID REFERENCES truck_providers(id),
    replacement_truck_id UUID REFERENCES trucks(id),

    -- Set when the failed provider already holds the cargo. A replacement then
    -- requires a controlled custody transfer rather than a field rewrite:
    -- silently repointing provider_id would leave the ledger saying one person
    -- holds the goods while the booking says another does.
    custody_transfer_required BOOLEAN NOT NULL DEFAULT FALSE,
    custody_transfer_ref TEXT NOT NULL DEFAULT '',
    evidence_ref TEXT NOT NULL DEFAULT '',

    reason_code TEXT NOT NULL DEFAULT '',

    -- operation_ref is the caller's idempotency key. UNIQUE is what makes a
    -- retried recovery command return the original case instead of opening a
    -- second one for the same failure.
    operation_ref TEXT NOT NULL UNIQUE,

    correlation_id TEXT NOT NULL DEFAULT '',

    detected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    resolved_at TIMESTAMPTZ,

    -- version is the optimistic-concurrency guard. Every advance asserts the
    -- version it read, so two operators (or an operator and a worker) racing
    -- the same case produce one winner and one conflict rather than two
    -- replacements.
    version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),

    -- Immutable append-only audit. Each advance appends; nothing rewrites.
    audit_json JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(audit_json) = 'array'),

    -- A replacement that is the same provider is not a replacement, and would
    -- quietly resolve a case without fixing anything.
    CONSTRAINT chk_hauling_recovery_distinct_provider
        CHECK (replacement_provider_id IS NULL
               OR replacement_provider_id IS DISTINCT FROM original_provider_id),

    -- Terminal exactly when resolved. Letting the two disagree makes "is this
    -- case still open" a question with two answers.
    CONSTRAINT chk_hauling_recovery_terminal
        CHECK ((state IN ('RESOLVED', 'FAILED')) = (resolved_at IS NOT NULL))
);

-- At most one OPEN recovery per booking.
--
-- This is the concurrency control that matters most: without it, two breakdown
-- reports for the same booking would open two cases, each search a
-- replacement, and each bind a different provider to the same cargo.
CREATE UNIQUE INDEX IF NOT EXISTS uq_hauling_recovery_open_booking
    ON hauling_recovery_cases (booking_id)
    WHERE state NOT IN ('RESOLVED', 'FAILED');

CREATE INDEX IF NOT EXISTS idx_hauling_recovery_state
    ON hauling_recovery_cases (state, detected_at);
CREATE INDEX IF NOT EXISTS idx_hauling_recovery_booking
    ON hauling_recovery_cases (booking_id, detected_at DESC);

COMMENT ON TABLE hauling_recovery_cases IS
'Haulage-owned recovery after assignment. A replacement runs the SAME cargofit engine as the original match; the booking identity, payment reference and cargo requirement survive it unchanged.';
COMMENT ON COLUMN hauling_recovery_cases.custody_transfer_required IS
'True when the failed provider already holds the cargo. A replacement then needs a controlled custody transfer, never a rewrite of provider_id.';
COMMENT ON INDEX uq_hauling_recovery_open_booking IS
'At most one open case per booking: two concurrent breakdown reports must not each bind a different replacement to the same cargo.';

-- The booking gains a pointer to its active recovery, so the lifecycle can ask
-- "is this booking under recovery" without scanning the case table on every
-- transition.
ALTER TABLE haulage_bookings
    ADD COLUMN IF NOT EXISTS active_recovery_id UUID REFERENCES hauling_recovery_cases(id),
    -- recovery_count is evidence, not state: a booking that has been recovered
    -- twice is a booking an operator should look at even after it completes.
    ADD COLUMN IF NOT EXISTS recovery_count INT NOT NULL DEFAULT 0
        CHECK (recovery_count >= 0);

COMMENT ON COLUMN haulage_bookings.active_recovery_id IS
'The open recovery case, if any. The booking id itself never changes across a replacement -- that is what keeps the payment source reference and handover binding valid.';
