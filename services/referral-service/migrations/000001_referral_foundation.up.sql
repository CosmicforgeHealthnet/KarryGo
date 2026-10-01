-- referral-service foundation: event intake and the activity record.

-- One row per received event. Begin/MarkProcessed run inside the same
-- transaction as the event's business effect, so an event is never marked
-- processed unless its effect committed. REJECTED is a malformed event that
-- retrying cannot fix: acknowledged, kept as evidence, never applied.
CREATE TABLE IF NOT EXISTS referral_inbox (
    event_id      text PRIMARY KEY,
    event_type    text NOT NULL,
    producer      text NOT NULL,
    payload_hash  text NOT NULL,
    status        text NOT NULL CHECK (status IN ('PROCESSING', 'PROCESSED', 'REJECTED', 'FAILED')),
    error_code    text,
    received_at   timestamptz NOT NULL DEFAULT now(),
    processed_at  timestamptz
);

-- Signed internal requests carry a nonce; each may be used once.
CREATE TABLE IF NOT EXISTS referral_service_nonces (
    caller      text NOT NULL,
    nonce       text NOT NULL,
    expires_at  timestamptz NOT NULL,
    PRIMARY KEY (caller, nonce)
);
CREATE INDEX IF NOT EXISTS referral_service_nonces_expiry_idx ON referral_service_nonces (expires_at);

-- Every paid-out job the wallet has told us about, keyed by the job's natural
-- key. This key -- not the event id -- is what makes every referral effect of
-- a job happen at most once.
CREATE TABLE IF NOT EXISTS settled_jobs (
    source_service          text NOT NULL,
    source_reference        text NOT NULL,
    payment_reference       text NOT NULL DEFAULT '',
    customer_id             text NOT NULL DEFAULT '',
    provider_actor_type     text NOT NULL DEFAULT '',
    provider_id             text NOT NULL DEFAULT '',
    wallet_provider_type    text NOT NULL DEFAULT '',
    amount_kobo             bigint NOT NULL CHECK (amount_kobo > 0),
    platform_fee_kobo       bigint NOT NULL CHECK (platform_fee_kobo >= 0),
    currency                text NOT NULL DEFAULT 'NGN',
    job_completed_at        timestamptz NOT NULL,
    job_completed_at_source text NOT NULL DEFAULT '',
    settled_at              timestamptz NOT NULL,
    in_programme            boolean NOT NULL,
    reversed_at             timestamptz,
    received_at             timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source_service, source_reference)
);
CREATE INDEX IF NOT EXISTS settled_jobs_customer_idx ON settled_jobs (customer_id, job_completed_at) WHERE in_programme;
CREATE INDEX IF NOT EXISTS settled_jobs_provider_idx ON settled_jobs (provider_actor_type, provider_id, job_completed_at) WHERE in_programme;

-- One row per (account, paid job): the evidence behind the "active referrer"
-- rule. Counting a time range answers "was X active as of T" for any T, which
-- a running counter cannot.
CREATE TABLE IF NOT EXISTS actor_activity_jobs (
    actor_type        text NOT NULL CHECK (actor_type IN ('customer', 'hauling', 'dispatch', 'taxi')),
    actor_id          text NOT NULL,
    source_service    text NOT NULL,
    source_reference  text NOT NULL,
    job_completed_at  timestamptz NOT NULL,
    reversed_at       timestamptz,
    recorded_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_type, actor_id, source_service, source_reference)
);
CREATE INDEX IF NOT EXISTS actor_activity_recent_idx
    ON actor_activity_jobs (actor_type, actor_id, job_completed_at DESC)
    WHERE reversed_at IS NULL;
