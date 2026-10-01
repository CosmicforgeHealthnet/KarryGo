-- The driver half of the programme.

-- When each driver account became fully approved. Kept apart from
-- referral_actors so an approval can arrive before (or without) the account's
-- sign-up announcement -- the two travel through different relays.
CREATE TABLE IF NOT EXISTS actor_onboarding (
    actor_type    text NOT NULL CHECK (actor_type IN ('hauling', 'dispatch', 'taxi')),
    actor_id      text NOT NULL,
    completed_at  timestamptz NOT NULL,
    recorded_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_type, actor_id)
);

-- Which paid jobs counted towards a driver track, and how: one of the first
-- jobs that qualify the referral ("activation"), or a job inside the 90-day
-- earning window ("window"). The key makes counting idempotent, so a replayed
-- job can never be counted twice.
CREATE TABLE IF NOT EXISTS referral_track_jobs (
    referral_id       text NOT NULL REFERENCES referrals (id),
    model             text NOT NULL CHECK (model IN ('customer', 'provider')),
    source_service    text NOT NULL,
    source_reference  text NOT NULL,
    job_completed_at  timestamptz NOT NULL,
    counted_as        text NOT NULL CHECK (counted_as IN ('activation', 'window', 'not_counted')),
    reason            text NOT NULL DEFAULT '',
    recorded_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (referral_id, model, source_service, source_reference)
);
