-- Durability foundation: transactional outbox (for events vendor-service
-- produces) and inbox (dedupe for events vendor-service consumes from other
-- services). See docs/architecture/08_EVENT_OWNERSHIP.md and
-- docs/architecture/10_WORKER_AND_RECOVERY_MODEL.md.
--
-- vendor_id is nullable: some events are platform-scoped (e.g. a
-- COMMERCE_GLOBAL_KILL_SWITCH audit event) rather than owned by one vendor
-- business.

CREATE TABLE IF NOT EXISTS vendor_outbox (
    id text PRIMARY KEY,
    event_type text NOT NULL,
    schema_version integer NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id text NOT NULL,
    aggregate_version bigint NOT NULL,
    vendor_id text,
    correlation_id text NOT NULL,
    causation_id text,
    payload jsonb NOT NULL,
    headers jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'PUBLISHED', 'DEAD_LETTERED')),
    available_at timestamptz NOT NULL DEFAULT now(),
    attempts integer NOT NULL DEFAULT 0,
    lease_owner text,
    lease_expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    last_error_redacted text NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS vendor_outbox_claim_idx
    ON vendor_outbox (status, available_at)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS vendor_outbox_aggregate_idx
    ON vendor_outbox (aggregate_type, aggregate_id);

-- Inbox: records the (source_service, event_id) pairs vendor-service has
-- already processed, so a redelivered event from an at-least-once transport
-- (HTTP retry, stream redelivery after a crash) is a safe no-op rather than a
-- duplicate side effect.
CREATE TABLE IF NOT EXISTS vendor_inbox_processed_events (
    source_service text NOT NULL,
    event_id text NOT NULL,
    event_type text NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source_service, event_id)
);
