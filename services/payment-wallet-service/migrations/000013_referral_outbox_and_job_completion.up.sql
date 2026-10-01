-- Referral programme feed.
--
-- referral_outbox carries finance.job.settled.v1 events to referral-service.
-- It is appended in the same transaction as the job settlement (CompleteJob),
-- so a paid-out job and its event commit together or not at all. It is a
-- separate table from finance_outbox so the referral relay and the Fleet relay
-- never compete for, or mark, each other's rows. Shape: shared/go/outbox.
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
CREATE INDEX IF NOT EXISTS referral_outbox_aggregate_idx
    ON referral_outbox (aggregate_type, aggregate_id);

-- When the job itself finished, as reported by the service that ran it. The
-- referral programme's time rules (30-day activity, 90-day earning window) key
-- off this, not off when the money moved. NULL on jobs settled before this
-- column existed or without a reported time.
ALTER TABLE payment_intents ADD COLUMN IF NOT EXISTS job_completed_at timestamptz;
