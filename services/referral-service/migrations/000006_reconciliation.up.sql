-- Accounts whose referral postings disagree with payment-wallet's ledger.
-- One open row per account: a mismatch seen again updates it, and the row is
-- resolved the first time the two agree again.
CREATE TABLE IF NOT EXISTS reconciliation_mismatches (
    id                      text PRIMARY KEY,
    actor_type              text NOT NULL,
    actor_id                text NOT NULL,
    posted_credit_kobo      bigint NOT NULL,
    posted_reversal_kobo    bigint NOT NULL,
    in_flight_credit_kobo   bigint NOT NULL,
    in_flight_reversal_kobo bigint NOT NULL,
    wallet_credit_kobo      bigint NOT NULL,
    wallet_reversal_kobo    bigint NOT NULL,
    first_seen_at           timestamptz NOT NULL DEFAULT now(),
    last_checked_at         timestamptz NOT NULL DEFAULT now(),
    resolved_at             timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS reconciliation_mismatches_open_uidx
    ON reconciliation_mismatches (actor_type, actor_id) WHERE resolved_at IS NULL;

-- Finding who to reconcile: every account that has had a posting.
CREATE INDEX IF NOT EXISTS reward_ledger_beneficiary_idx
    ON reward_ledger (beneficiary_actor_type, beneficiary_actor_id) WHERE wallet_state <> 'not_sent';
