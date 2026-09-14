-- Section 12 WAL-017: the durable record of a managed job whose money is still
-- sitting in escrow.
--
-- A job marked ORGANIZATION_MANAGED is deliberately closed to the legacy
-- direct-payout path, so if Fleet never instructs a settlement the money simply
-- stays held -- correctly, and invisibly. Nobody is overpaid and nothing is
-- corrupt, which is exactly why nothing surfaces it: there is no error, no
-- failed row and no alert. The provider finds out by asking where their money
-- is.
--
-- This table is what makes that state visible, and one row per job is what
-- keeps it from becoming noise: a worker that ran every five minutes would
-- otherwise report the same stuck job forever.
CREATE TABLE IF NOT EXISTS managed_settlement_reconciliations (
    id                      text PRIMARY KEY,

    source_service          text NOT NULL,
    source_job_id           text NOT NULL,
    payment_intent_id       text NOT NULL,

    currency                text NOT NULL,
    held_amount_minor       bigint NOT NULL CHECK (held_amount_minor >= 0),

    -- OPEN while the money is still unsettled; RESOLVED once a settlement
    -- posted. Resolved rows are kept rather than deleted: "this job was stuck
    -- for two days and then settled" is the thing worth being able to look up.
    status                  text NOT NULL DEFAULT 'OPEN'
                                CHECK (status IN ('OPEN', 'RESOLVED')),

    -- How the reconciliation ended, when it ended.
    resolution              text NOT NULL DEFAULT '',

    -- The settlement that resolved it, if one did.
    instruction_id          text NOT NULL DEFAULT '',
    instruction_version     bigint NOT NULL DEFAULT 0,

    held_since              timestamptz NOT NULL,
    first_observed_at       timestamptz NOT NULL DEFAULT now(),
    last_observed_at        timestamptz NOT NULL DEFAULT now(),
    observations            int NOT NULL DEFAULT 1 CHECK (observations > 0),
    resolved_at             timestamptz,

    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

-- WAL-017A. One reconciliation per job, so re-running the worker updates the
-- existing row instead of opening a second one. Without this, idempotency
-- would depend on the worker never running twice, which is not something a
-- worker can promise.
CREATE UNIQUE INDEX IF NOT EXISTS uq_managed_settlement_reconciliation_job
    ON managed_settlement_reconciliations (source_service, source_job_id);

-- The worker's own scan: what is still open, oldest first.
CREATE INDEX IF NOT EXISTS idx_managed_settlement_reconciliation_open
    ON managed_settlement_reconciliations (status, held_since)
    WHERE status = 'OPEN';
