-- Section 17: moving a held payment to the provider who actually does the job.
--
-- A job's beneficiary is bound when the provider accepts. When that provider
-- fails mid-job and the owning service's recovery hands the job to a
-- replacement, the booking moves but the held payment did not: CompleteJob
-- would then pay the provider who walked away, and the one who finished the
-- job would get nothing.
--
-- A rebind changes only who the held escrow will be released to. The amount,
-- currency, customer, fee and status are untouched, and no ledger entry is
-- written: the money sits in the job's escrow account either way, and moves
-- exactly once, at completion.

-- Optimistic concurrency for the binding. It starts at 1 for every existing
-- intent, so a caller that has never read it can still name the first version.
ALTER TABLE payment_intents
    ADD COLUMN IF NOT EXISTS execution_binding_version bigint NOT NULL DEFAULT 1
        CHECK (execution_binding_version >= 1);

-- One row per rebind. Append-only: the question it answers -- "why was the
-- money released to this provider and not the one who accepted?" -- is asked
-- after the fact, usually in a dispute, and only an unedited record answers it.
CREATE TABLE IF NOT EXISTS execution_provider_rebind_audit (
    id uuid PRIMARY KEY,
    payment_intent_id uuid NOT NULL REFERENCES payment_intents(id),
    source_service text NOT NULL,
    source_reference text NOT NULL,
    caller_service text NOT NULL,
    previous_provider_type text NOT NULL,
    previous_provider_id text NOT NULL,
    new_provider_type text NOT NULL,
    new_provider_id text NOT NULL CHECK (new_provider_id <> ''),
    from_version bigint NOT NULL,
    to_version bigint NOT NULL,
    -- The owning service's recovery case: the only thing that justifies moving
    -- the beneficiary. Wallet does not judge it; it records it.
    recovery_case_ref text NOT NULL CHECK (recovery_case_ref <> ''),
    reason_code text NOT NULL DEFAULT '',
    -- What was held at the moment of the rebind, so a later reader can see the
    -- rebind moved nothing but the beneficiary.
    amount_kobo bigint NOT NULL,
    currency text NOT NULL,
    idempotency_key text NOT NULL,
    request_fingerprint text NOT NULL CHECK (length(request_fingerprint) = 64),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT execution_provider_rebind_version_step CHECK (to_version = from_version + 1),
    CONSTRAINT execution_provider_rebind_changes_provider
        CHECK (previous_provider_id <> new_provider_id OR previous_provider_type <> new_provider_type)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_execution_provider_rebind_key
    ON execution_provider_rebind_audit (idempotency_key);
CREATE UNIQUE INDEX IF NOT EXISTS uq_execution_provider_rebind_version
    ON execution_provider_rebind_audit (payment_intent_id, to_version);
CREATE INDEX IF NOT EXISTS idx_execution_provider_rebind_source
    ON execution_provider_rebind_audit (source_service, source_reference, created_at);

CREATE OR REPLACE FUNCTION execution_provider_rebind_audit_append_only()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'execution_provider_rebind_audit is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS execution_provider_rebind_audit_no_mutation ON execution_provider_rebind_audit;
CREATE TRIGGER execution_provider_rebind_audit_no_mutation
    BEFORE UPDATE OR DELETE ON execution_provider_rebind_audit
    FOR EACH ROW EXECUTE FUNCTION execution_provider_rebind_audit_append_only();
