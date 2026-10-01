-- People, their accounts, codes and who referred whom.
--
-- A person is a human; an actor is one of their accounts in one service
-- (customer, hauling, dispatch, taxi). Accounts are matched to one person by
-- keyed hashes of phone and email -- raw values never reach this service.

CREATE TABLE IF NOT EXISTS referral_persons (
    id               text PRIMARY KEY,
    status           text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'frozen', 'blocked')),
    status_reason    text NOT NULL DEFAULT '',
    -- Earnings are paid to the account that first received the code.
    home_actor_type  text NOT NULL,
    home_actor_id    text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS referral_actors (
    actor_type              text NOT NULL CHECK (actor_type IN ('customer', 'hauling', 'dispatch', 'taxi')),
    actor_id                text NOT NULL,
    person_id               text NOT NULL REFERENCES referral_persons (id),
    registered_at           timestamptz NOT NULL,
    is_new_account          boolean NOT NULL,
    -- What happened to the code (if any) supplied at sign-up; for support.
    attribution_outcome     text NOT NULL DEFAULT '',
    location_state          text NOT NULL DEFAULT '',
    created_at              timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_type, actor_id)
);
CREATE INDEX IF NOT EXISTS referral_actors_person_idx ON referral_actors (person_id);

-- Keyed hashes seen for each person. A phone or email belongs to exactly one
-- person; devices and bank accounts may be shared and are only used to raise
-- review flags.
CREATE TABLE IF NOT EXISTS person_fingerprints (
    kind           text NOT NULL CHECK (kind IN ('phone', 'email', 'device', 'bank')),
    value_hash     text NOT NULL,
    person_id      text NOT NULL REFERENCES referral_persons (id),
    first_seen_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, value_hash, person_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS person_fingerprints_identity_uidx
    ON person_fingerprints (kind, value_hash) WHERE kind IN ('phone', 'email');
CREATE INDEX IF NOT EXISTS person_fingerprints_person_idx ON person_fingerprints (person_id);

-- One code per person, shared by all of their accounts.
CREATE TABLE IF NOT EXISTS referral_codes (
    code        text PRIMARY KEY,
    person_id   text NOT NULL UNIQUE REFERENCES referral_persons (id),
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Who referred whom. Enforced by the database, not just by code:
--   * a person can be referred at most once, ever;
--   * nobody can refer themselves.
CREATE TABLE IF NOT EXISTS referrals (
    id                  text PRIMARY KEY,
    referrer_person_id  text NOT NULL REFERENCES referral_persons (id),
    referred_person_id  text NOT NULL REFERENCES referral_persons (id),
    code                text NOT NULL REFERENCES referral_codes (code),
    code_source         text NOT NULL CHECK (code_source IN ('link', 'typed')),
    attributed_at       timestamptz NOT NULL,
    status              text NOT NULL DEFAULT 'active'
                            CHECK (status IN ('active', 'under_review', 'suspended', 'disqualified')),
    status_reason       text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT referrals_one_per_person UNIQUE (referred_person_id),
    CONSTRAINT referrals_no_self CHECK (referrer_person_id <> referred_person_id)
);
CREATE INDEX IF NOT EXISTS referrals_referrer_idx ON referrals (referrer_person_id);

-- Each referral can run a customer track, a driver track, or both (a referred
-- customer who later becomes a driver). Qualification and the 90-day window
-- live here.
CREATE TABLE IF NOT EXISTS referral_tracks (
    referral_id                 text NOT NULL REFERENCES referrals (id),
    model                       text NOT NULL CHECK (model IN ('customer', 'provider')),
    status                      text NOT NULL DEFAULT 'pending_qualification'
                                    CHECK (status IN ('pending_qualification', 'qualified', 'expired')),
    jobs_required               int NOT NULL CHECK (jobs_required > 0),
    jobs_counted                int NOT NULL DEFAULT 0 CHECK (jobs_counted >= 0),
    onboarding_completed_at     timestamptz,
    qualified_at                timestamptz,
    qualifying_source_service   text NOT NULL DEFAULT '',
    qualifying_source_reference text NOT NULL DEFAULT '',
    window_starts_at            timestamptz,
    window_ends_at              timestamptz,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (referral_id, model)
);
CREATE INDEX IF NOT EXISTS referral_tracks_window_idx
    ON referral_tracks (window_ends_at) WHERE model = 'provider' AND status = 'qualified';

-- Things a human should look at. Nothing here moves money on its own.
CREATE TABLE IF NOT EXISTS fraud_flags (
    id               text PRIMARY KEY,
    person_id        text REFERENCES referral_persons (id),
    referral_id      text REFERENCES referrals (id),
    flag_type        text NOT NULL CHECK (flag_type IN (
                         'self_referral', 'duplicate_identity', 'identity_conflict', 'duplicate_device',
                         'duplicate_bank', 'repeated_reversals', 'manual')),
    severity         text NOT NULL CHECK (severity IN ('info', 'review')),
    evidence         jsonb NOT NULL DEFAULT '{}'::jsonb,
    status           text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'cleared', 'upheld')),
    created_at       timestamptz NOT NULL DEFAULT now(),
    resolved_at      timestamptz,
    resolved_by      text NOT NULL DEFAULT '',
    resolution_note  text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS fraud_flags_open_idx ON fraud_flags (status, created_at) WHERE status = 'open';
CREATE INDEX IF NOT EXISTS fraud_flags_person_idx ON fraud_flags (person_id);
