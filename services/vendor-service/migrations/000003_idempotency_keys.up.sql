-- Idempotency-Key support for state-changing commands. Scoped per (scope_id,
-- endpoint, idempotency_key): scope_id is typically a vendor business id, but
-- may be any caller-defined scope (e.g. a customer id for checkout commands).
-- See docs/architecture/09_SECURITY_AND_AUTHORITY_MODEL.md.

CREATE TABLE IF NOT EXISTS vendor_idempotency_keys (
    scope_id text NOT NULL,
    endpoint text NOT NULL,
    idempotency_key text NOT NULL,
    request_fingerprint text NOT NULL,
    status text NOT NULL DEFAULT 'IN_PROGRESS'
        CHECK (status IN ('IN_PROGRESS', 'COMPLETED')),
    response_status integer,
    response_body bytea,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (scope_id, endpoint, idempotency_key)
);

CREATE INDEX IF NOT EXISTS vendor_idempotency_keys_expiry_idx
    ON vendor_idempotency_keys (expires_at);
