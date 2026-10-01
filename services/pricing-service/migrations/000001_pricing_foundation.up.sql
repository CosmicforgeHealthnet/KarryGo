-- Pricing service foundation.
--
-- This migration creates only the machinery every later migration relies on:
-- the immutability guards, the signed-request nonce log, administrative
-- idempotency, the audit trail, the outbox and the operator register. No
-- commercial data lives here. Rates, markets, quotes and cost indices arrive in
-- later migrations, and no commercial VALUE is ever written by a migration --
-- policy versions are created as drafts and activated through the approval API,
-- so that every rate in production has an approver's name against it.

-- ---------------------------------------------------------------------------
-- Immutability guards
-- ---------------------------------------------------------------------------

-- Two guards, because "append-only" has two different strengths here.
--
-- pricing_reject_mutation is ABSOLUTE: it refuses UPDATE and DELETE with no
-- escape hatch whatsoever. It protects the records that prove what happened --
-- the audit trail, policy activations, approval decisions and lifecycle events.
-- Those must be undeletable even by an operator holding a psql prompt, because
-- they are the evidence that a price change was authorised.
--
-- pricing_reject_mutation_purgeable additionally permits DELETE while the
-- pricing.allow_purge flag is set. It exists for ONE case: expired quotes that
-- were never bound to a booking, which are operational exhaust rather than
-- financial records and would otherwise grow without limit.
--
-- These are separate functions rather than one function with a flag because a
-- shared escape hatch leaks: attaching a purge-capable guard to the audit log
-- would let any session delete audit rows simply by setting the flag. They are
-- enforced in the database rather than in Go so the guarantee holds against a
-- future handler, a migration, and a well-meaning bug fix.
CREATE OR REPLACE FUNCTION pricing_reject_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION
        '% on % is refused: this table is append-only evidence of an economic decision',
        TG_OP, TG_TABLE_NAME
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION pricing_reject_mutation() IS
    'Absolute append-only guard, no exceptions. For audit, activations, '
    'decisions and lifecycle events. See migration 000001.';

CREATE OR REPLACE FUNCTION pricing_reject_mutation_purgeable() RETURNS trigger AS $$
BEGIN
    -- UPDATE is never permitted: a stored economic decision is not editable.
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION
            'UPDATE on % is refused: a recorded economic decision is immutable',
            TG_TABLE_NAME
            USING ERRCODE = 'restrict_violation';
    END IF;

    -- DELETE is permitted only under the retention purge. The flag is a guard
    -- against accidents, NOT an authority check: any session can set it, so the
    -- purge function that sets it is also responsible for selecting only rows
    -- that are genuinely expired and unbound, and for writing its own audit row.
    IF current_setting('pricing.allow_purge', true) = 'on' THEN
        RETURN OLD;
    END IF;

    RAISE EXCEPTION
        'DELETE on % is refused outside the retention purge',
        TG_TABLE_NAME
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION pricing_reject_mutation_purgeable() IS
    'Append-only guard that allows the retention purge to DELETE. Only for '
    'expired unbound quotes, never for evidence tables. See migration 000001.';

-- ---------------------------------------------------------------------------
-- Signed-request nonces
-- ---------------------------------------------------------------------------

-- Every signed internal request is single-use. Replaying a quote request would
-- mint a second sealed quote for one intent; replaying a bind or reconcile
-- would attempt a second economic decision on one booking.
CREATE TABLE IF NOT EXISTS pricing_service_nonces (
    caller     text NOT NULL,
    nonce      text NOT NULL,
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (caller, nonce)
);

CREATE INDEX IF NOT EXISTS pricing_service_nonces_expiry_idx
    ON pricing_service_nonces (expires_at);

-- ---------------------------------------------------------------------------
-- Administrative idempotency
-- ---------------------------------------------------------------------------

-- Quotes, bindings and reconciliations are NOT made idempotent here: they use
-- natural keys and unique constraints on the records themselves, which survives
-- this table being pruned. This covers administrative commands only.
CREATE TABLE IF NOT EXISTS pricing_idempotency_keys (
    caller              text NOT NULL,
    endpoint            text NOT NULL,
    idempotency_key     text NOT NULL,
    request_fingerprint text NOT NULL,
    status              text NOT NULL
                            CHECK (status IN ('IN_PROGRESS', 'COMPLETED')),
    response_body       bytea,
    created_at          timestamptz NOT NULL DEFAULT now(),
    completed_at        timestamptz,
    expires_at          timestamptz NOT NULL,
    PRIMARY KEY (caller, endpoint, idempotency_key),
    CONSTRAINT pricing_idempotency_completed_has_time
        CHECK ((status = 'COMPLETED') = (completed_at IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS pricing_idempotency_keys_expiry_idx
    ON pricing_idempotency_keys (expires_at);

-- ---------------------------------------------------------------------------
-- Audit trail
-- ---------------------------------------------------------------------------

-- Separate from the outbox on purpose. Events tell other services what
-- happened; this answers a reviewer's question: which human asked, which
-- service carried it, what reason did they give, what did it look like before.
--
-- actor_caller is proven by HMAC signature. performed_by is asserted by that
-- caller, so both are stored rather than collapsed into one "actor".
CREATE TABLE IF NOT EXISTS pricing_audit_log (
    id              bigserial PRIMARY KEY,
    actor_caller    text NOT NULL CHECK (actor_caller <> ''),
    performed_by    text NOT NULL CHECK (performed_by <> ''),
    action          text NOT NULL CHECK (action <> ''),
    subject_type    text NOT NULL CHECK (subject_type <> ''),
    subject_id      text NOT NULL CHECK (subject_id <> ''),
    reason          text NOT NULL CHECK (reason <> ''),
    before_snapshot jsonb,
    after_snapshot  jsonb,
    request_id      text NOT NULL DEFAULT '',
    recorded_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS pricing_audit_log_subject_idx
    ON pricing_audit_log (subject_type, subject_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS pricing_audit_log_recorded_idx
    ON pricing_audit_log (recorded_at DESC);

DROP TRIGGER IF EXISTS pricing_audit_log_append_only ON pricing_audit_log;
CREATE TRIGGER pricing_audit_log_append_only
    BEFORE UPDATE OR DELETE ON pricing_audit_log
    FOR EACH ROW EXECUTE FUNCTION pricing_reject_mutation();

-- ---------------------------------------------------------------------------
-- Transactional outbox
-- ---------------------------------------------------------------------------

-- Shape required by shared/go/outbox. Pricing appends an event in the same
-- transaction as the change it describes, and a relay worker publishes it
-- later. Consumers dedupe on the event id.
CREATE TABLE IF NOT EXISTS pricing_outbox (
    id                  text PRIMARY KEY,
    event_type          text NOT NULL,
    schema_version      int  NOT NULL CHECK (schema_version > 0),
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

CREATE INDEX IF NOT EXISTS pricing_outbox_due_idx
    ON pricing_outbox (status, available_at) WHERE status = 'PENDING';

-- ---------------------------------------------------------------------------
-- Operator register
-- ---------------------------------------------------------------------------

-- Pricing has no login of its own. Administration arrives as a signed request
-- from admin-backoffice-service naming a person, and that person's authority is
-- checked here.
--
-- Roles are separated so that proposing a price change never implies approving
-- one: that separation is the whole point of four-eye approval.
CREATE TABLE IF NOT EXISTS pricing_operators (
    operator_id text PRIMARY KEY CHECK (operator_id <> ''),
    display_name text NOT NULL DEFAULT '',
    roles       text[] NOT NULL DEFAULT '{}',
    active      boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pricing_operators_known_roles CHECK (
        roles <@ ARRAY[
            'EDITOR', 'APPROVER', 'INDEX_OPERATOR', 'INDEX_APPROVER',
            'ROLLOUT_APPROVER', 'VIEWER'
        ]::text[]
    )
);

COMMENT ON TABLE pricing_operators IS
    'People who may change pricing configuration. Until admin-backoffice-service '
    'has real staff authentication, performed_by is an assertion by that service, '
    'so the admin caller allowlist is kept deliberately short.';
