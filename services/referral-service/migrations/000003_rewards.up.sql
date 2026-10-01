-- The reward ledger: every amount the programme has decided to pay, or to
-- take back. Amounts never change once written; a reversal is a new negative
-- row pointing at the reward it undoes. Money only moves when a row is
-- posted to payment-wallet (wallet_state = 'posted'), and every row carries
-- its own idempotency key, so neither this table nor the wallet can pay the
-- same reward twice.
CREATE TABLE IF NOT EXISTS reward_ledger (
    id                      text PRIMARY KEY,
    person_id               text NOT NULL REFERENCES referral_persons (id),
    beneficiary_actor_type  text NOT NULL CHECK (beneficiary_actor_type IN ('customer', 'hauling', 'dispatch', 'taxi')),
    beneficiary_actor_id    text NOT NULL,
    referral_id             text REFERENCES referrals (id),
    model                   text NOT NULL DEFAULT '',
    reward_type             text NOT NULL CHECK (reward_type IN (
                                'customer_referral_bonus', 'customer_welcome_credit', 'first_booking_credit',
                                'provider_commission', 'provider_welcome_credit',
                                'milestone_free_delivery', 'milestone_bonus', 'reversal')),
    balance                 text NOT NULL CHECK (balance IN ('earnings', 'promotional')),
    amount_kobo             bigint NOT NULL,
    currency                text NOT NULL DEFAULT 'NGN',
    status                  text NOT NULL CHECK (status IN (
                                'pending_qualification', 'pending_activity', 'available', 'frozen', 'rejected', 'reversed')),
    status_reason           text NOT NULL DEFAULT '',
    -- What the amount was worked out from, snapshotted so a later settings
    -- change never re-prices an old reward.
    basis_kobo              bigint NOT NULL DEFAULT 0,
    rate_bps                bigint NOT NULL DEFAULT 0,
    source_service          text NOT NULL DEFAULT '',
    source_reference        text NOT NULL DEFAULT '',
    -- The referrer must have been active at this moment (the job's
    -- completion) for the reward to be paid.
    activity_as_of          timestamptz,
    -- Not decided before this time, so late-arriving activity is counted.
    decide_after            timestamptz,
    decided_at              timestamptz,
    reverses_reward_id      text REFERENCES reward_ledger (id),
    idempotency_key         text NOT NULL UNIQUE,
    wallet_state            text NOT NULL DEFAULT 'not_sent' CHECK (wallet_state IN ('not_sent', 'pending', 'posted')),
    wallet_reference        text NOT NULL DEFAULT '',
    wallet_attempts         int NOT NULL DEFAULT 0,
    wallet_next_attempt_at  timestamptz,
    wallet_lease_until      timestamptz,
    wallet_last_error       text NOT NULL DEFAULT '',
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT reward_ledger_sign CHECK (
        (reward_type = 'reversal' AND amount_kobo < 0 AND reverses_reward_id IS NOT NULL) OR
        (reward_type <> 'reversal' AND amount_kobo > 0 AND reverses_reward_id IS NULL)),
    CONSTRAINT reward_ledger_posted_needs_approval CHECK (wallet_state <> 'posted' OR status IN ('available', 'reversed'))
);
CREATE INDEX IF NOT EXISTS reward_ledger_decide_idx ON reward_ledger (decide_after) WHERE status = 'pending_activity';
CREATE INDEX IF NOT EXISTS reward_ledger_wallet_idx ON reward_ledger (wallet_next_attempt_at) WHERE wallet_state = 'pending';
CREATE INDEX IF NOT EXISTS reward_ledger_person_idx ON reward_ledger (person_id, created_at);
CREATE INDEX IF NOT EXISTS reward_ledger_referral_idx ON reward_ledger (referral_id, reward_type);
CREATE INDEX IF NOT EXISTS reward_ledger_source_idx ON reward_ledger (source_service, source_reference) WHERE source_reference <> '';
