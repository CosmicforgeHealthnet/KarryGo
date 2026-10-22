-- Section 17 gap closure: a durable obligation to move a booking's held Wallet
-- payment to the provider recovery bound in place of the original one.
--
-- Before this table the move was a single best-effort call made after the
-- replacement committed, and the operator-directed replacement made no call at
-- all. A lost response, a Wallet outage or a crash in between left the payment
-- bound to the ORIGINAL provider with nothing recording that it still had to
-- move. The settlement-time guard caught it at the very end, but nothing drove
-- it before then and nothing showed it was outstanding.
--
-- A row is written in the SAME transaction that rebinds haulage_bookings, so
-- "the booking has a new provider" and "the payment must follow" commit or
-- roll back together. A worker then drives the row to a terminal state through
-- Wallet's own execution-provider contract; Hauling never touches Wallet data.
--
-- States:
--   PENDING                 nothing sent yet, or a definite refusal to retry
--   UNKNOWN_OUTCOME         a rebind was sent and no answer came back; the next
--                           attempt reads the authoritative Wallet binding
--                           BEFORE it may send anything again
--   COMPLETED               Wallet's binding names this provider
--   COMPLETED_NOT_HELD      Wallet holds nothing for this booking (cash, refunded,
--                           already settled): there is nothing to move
--   SUPERSEDED              Hauling no longer names this provider on the booking
--                           or the case; the obligation is stale and Wallet was
--                           never called for it
--   RECONCILIATION_REQUIRED attempts exhausted; left for an operator, never dropped

CREATE TABLE IF NOT EXISTS hauling_payment_rebind_obligations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id UUID NOT NULL REFERENCES haulage_bookings(id) ON DELETE RESTRICT,
    recovery_case_id UUID NOT NULL REFERENCES hauling_recovery_cases(id) ON DELETE RESTRICT,
    provider_id UUID NOT NULL REFERENCES truck_providers(id),

    state TEXT NOT NULL DEFAULT 'PENDING' CHECK (state IN (
        'PENDING', 'UNKNOWN_OUTCOME', 'COMPLETED', 'COMPLETED_NOT_HELD',
        'SUPERSEDED', 'RECONCILIATION_REQUIRED')),

    attempts INT NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    -- A stable code only (a Wallet error code or a Hauling classification).
    -- Never a message body, a signature or a secret.
    last_error_code TEXT NOT NULL DEFAULT '',
    -- The Wallet binding version observed when the obligation reached a
    -- terminal Wallet answer, and the Wallet rebind audit id when known.
    wallet_binding_version BIGINT,
    wallet_rebind_id TEXT NOT NULL DEFAULT '',

    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_owner TEXT NOT NULL DEFAULT '',
    lease_expires_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,

    -- Original-action identity: one obligation per (case, replacement). A
    -- retried command, a replayed admin command or a duplicate write collapses
    -- onto the existing row instead of creating a second obligation.
    CONSTRAINT uq_hauling_payment_rebind_case_provider UNIQUE (recovery_case_id, provider_id),

    -- Terminal exactly when completed_at is set.
    CONSTRAINT chk_hauling_payment_rebind_terminal CHECK (
        (state IN ('COMPLETED', 'COMPLETED_NOT_HELD', 'SUPERSEDED', 'RECONCILIATION_REQUIRED'))
        = (completed_at IS NOT NULL))
);

-- The worker's claim scan: due, non-terminal rows only.
CREATE INDEX IF NOT EXISTS idx_hauling_payment_rebind_due
    ON hauling_payment_rebind_obligations (next_attempt_at)
    WHERE state IN ('PENDING', 'UNKNOWN_OUTCOME');
CREATE INDEX IF NOT EXISTS idx_hauling_payment_rebind_booking
    ON hauling_payment_rebind_obligations (booking_id, created_at DESC);

COMMENT ON TABLE hauling_payment_rebind_obligations IS
'Durable obligation to move a booking''s held Wallet payment to the provider recovery bound. Written in the same transaction as the booking rebind; driven to a terminal state by the payment rebind worker through Wallet''s execution-provider contract.';
