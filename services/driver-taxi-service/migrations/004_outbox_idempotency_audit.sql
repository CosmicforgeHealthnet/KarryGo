-- Transactional outbox (addendum §26), Idempotency-Key store (ported from
-- services/vendor-service/internal/platform/idempotency -- see that package's
-- own header comment: no shared/go/idempotency exists yet, every service that
-- has real command idempotency reimplements it locally; this is taxi's copy,
-- not a third fork of the pattern), and admin audit trail (addendum §24).

CREATE TABLE IF NOT EXISTS taxi_outbox (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type TEXT NOT NULL DEFAULT 'booking',
    aggregate_id   UUID NOT NULL,
    event_type     TEXT NOT NULL,   -- taxi.booking.created, taxi.driver.accepted, ...
    payload        JSONB NOT NULL,
    occurred_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    available_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_owner    TEXT,
    lease_until    TIMESTAMPTZ,
    attempts       INT NOT NULL DEFAULT 0,
    delivered_at   TIMESTAMPTZ,
    dead_lettered_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS taxi_idempotency_keys (
    scope_id             TEXT NOT NULL,   -- authenticated actor id
    endpoint             TEXT NOT NULL,   -- e.g. 'POST /customer/bookings'
    idempotency_key      TEXT NOT NULL,
    request_fingerprint  TEXT NOT NULL,
    status               TEXT NOT NULL DEFAULT 'IN_PROGRESS',  -- IN_PROGRESS|COMPLETED
    response_status      INT,
    response_body        BYTEA,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at           TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (scope_id, endpoint, idempotency_key)
);

CREATE TABLE IF NOT EXISTS taxi_admin_audit_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operator_id TEXT NOT NULL,
    action      TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id   UUID NOT NULL,
    reason      TEXT NOT NULL,
    before      JSONB,
    after       JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_taxi_outbox_ready
  ON taxi_outbox(available_at) WHERE delivered_at IS NULL AND dead_lettered_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_taxi_idempotency_expiry ON taxi_idempotency_keys(expires_at);
CREATE INDEX IF NOT EXISTS idx_taxi_admin_audit_entity  ON taxi_admin_audit_events(entity_type, entity_id);
