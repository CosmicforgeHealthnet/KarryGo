-- Referral programme feed: identity.customer.registered.v1 events, appended in
-- the same transaction that creates a customer and relayed to
-- referral-service. Only keyed hashes of phone/email/device are ever written
-- here. Shape: shared/go/outbox. Idempotent: every migration in this service
-- re-runs on start.
CREATE TABLE IF NOT EXISTS referral_outbox (
    id                  text PRIMARY KEY,
    event_type          text NOT NULL,
    schema_version      int NOT NULL CHECK (schema_version > 0),
    aggregate_type      text NOT NULL,
    aggregate_id        text NOT NULL,
    payload             jsonb NOT NULL,
    status              text NOT NULL DEFAULT 'PENDING'
                            CHECK (status IN ('PENDING', 'PUBLISHED', 'DEAD_LETTERED')),
    available_at        timestamptz NOT NULL DEFAULT now(),
    attempts            int NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    lease_owner         text,
    lease_expires_at    timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    published_at        timestamptz,
    last_error_redacted text NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS referral_outbox_due_idx
    ON referral_outbox (status, available_at) WHERE status = 'PENDING';
