-- Milestones, reversals of paid-out jobs, collusion flags and the admin trail.

-- Each milestone is granted once per person, ever: the primary key is what
-- makes it exactly-once, however many referrals qualify at the same moment.
CREATE TABLE IF NOT EXISTS referral_milestones (
    person_id    text NOT NULL REFERENCES referral_persons (id),
    milestone    text NOT NULL CHECK (milestone IN ('qualified_5', 'qualified_10')),
    achieved_at  timestamptz NOT NULL,
    reward_id    text REFERENCES reward_ledger (id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (person_id, milestone)
);

-- The "Power User" badge from the 10-referral milestone.
ALTER TABLE referral_persons ADD COLUMN IF NOT EXISTS power_user_since timestamptz;

-- Every reversal of a paid-out job the wallet has reported, one row per
-- reversal reference, so a replayed reversal is applied once. A reversal can
-- arrive before the settlement it reverses; it is then applied when the
-- settlement lands (applied_at stays NULL until then).
CREATE TABLE IF NOT EXISTS settled_job_reversals (
    reversal_reference          text PRIMARY KEY,
    source_service              text NOT NULL,
    source_reference            text NOT NULL,
    reversed_amount_kobo        bigint NOT NULL CHECK (reversed_amount_kobo >= 0),
    reversed_platform_fee_kobo  bigint NOT NULL CHECK (reversed_platform_fee_kobo >= 0),
    reversal_kind               text NOT NULL DEFAULT '',
    fully_reversed              boolean NOT NULL,
    occurred_at                 timestamptz NOT NULL,
    applied_at                  timestamptz,
    received_at                 timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS settled_job_reversals_job_idx ON settled_job_reversals (source_service, source_reference);

-- A reversed job stops counting towards a driver track that has not
-- qualified yet (a qualified track keeps its qualification).
ALTER TABLE referral_track_jobs ADD COLUMN IF NOT EXISTS reversed_at timestamptz;

-- Take-backs are found through the reward they reverse.
CREATE INDEX IF NOT EXISTS reward_ledger_reverses_idx ON reward_ledger (reverses_reward_id) WHERE reverses_reward_id IS NOT NULL;

-- A job between a referred person and their own referrer (or themselves)
-- earns nothing and is flagged.
ALTER TABLE fraud_flags DROP CONSTRAINT IF EXISTS fraud_flags_flag_type_check;
ALTER TABLE fraud_flags ADD CONSTRAINT fraud_flags_flag_type_check CHECK (flag_type IN (
    'self_referral', 'duplicate_identity', 'identity_conflict', 'duplicate_device',
    'duplicate_bank', 'repeated_reversals', 'collusive_job', 'manual'));
CREATE INDEX IF NOT EXISTS fraud_flags_referral_idx ON fraud_flags (referral_id, flag_type) WHERE referral_id IS NOT NULL;

-- Every admin decision: who made it, through which service, and why.
CREATE TABLE IF NOT EXISTS referral_admin_actions (
    id            text PRIMARY KEY,
    action        text NOT NULL,
    subject_kind  text NOT NULL CHECK (subject_kind IN ('person', 'referral', 'reward', 'flag')),
    subject_id    text NOT NULL,
    performed_by  text NOT NULL CHECK (performed_by <> ''),
    caller        text NOT NULL,
    reason        text NOT NULL CHECK (reason <> ''),
    details       jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS referral_admin_actions_subject_idx ON referral_admin_actions (subject_kind, subject_id, created_at);
