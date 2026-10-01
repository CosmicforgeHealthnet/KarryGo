-- Sections 40-42: semantic idempotency metadata and policy-driven
-- receiver/location minimization. This migration is additive: immutable order
-- and financial evidence remains in commerce_parent_orders while a separate
-- operational projection can be minimized after the support window.

ALTER TABLE vendor_idempotency_keys
    ADD COLUMN IF NOT EXISTS actor_ref text,
    ADD COLUMN IF NOT EXISTS operation_type text,
    ADD COLUMN IF NOT EXISTS resource_target text,
    ADD COLUMN IF NOT EXISTS completed_at timestamptz,
    ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS vendor_idempotency_keys_actor_operation_idx
    ON vendor_idempotency_keys (actor_ref, operation_type, created_at DESC);

ALTER TABLE commerce_parent_orders
    ADD COLUMN IF NOT EXISTS operational_address_snapshot jsonb
        CHECK (operational_address_snapshot IS NULL OR jsonb_typeof(operational_address_snapshot) = 'object'),
    ADD COLUMN IF NOT EXISTS operational_receiver_snapshot jsonb
        CHECK (operational_receiver_snapshot IS NULL OR jsonb_typeof(operational_receiver_snapshot) = 'object');

CREATE TABLE vendor_privacy_retention_policies (
    id text PRIMARY KEY,
    data_class text NOT NULL,
    retain_for interval NOT NULL CHECK (retain_for > interval '0'),
    version bigint NOT NULL CHECK (version > 0),
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (data_class, version)
);

INSERT INTO vendor_privacy_retention_policies (id, data_class, retain_for, version)
VALUES ('vrp_order_receiver_location_v1', 'ORDER_RECEIVER_LOCATION', interval '90 days', 1)
ON CONFLICT (id) DO NOTHING;

CREATE UNIQUE INDEX vendor_privacy_retention_policy_active_uq
    ON vendor_privacy_retention_policies (data_class) WHERE active;

CREATE TABLE vendor_data_legal_holds (
    id text PRIMARY KEY,
    scope_type text NOT NULL CHECK (scope_type IN ('PARENT_ORDER','CUSTOMER','BUSINESS')),
    scope_ref text NOT NULL,
    reason_code text NOT NULL,
    created_by_ref text NOT NULL,
    correlation_id text,
    starts_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz,
    released_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at IS NULL OR expires_at > starts_at),
    CHECK (released_at IS NULL OR released_at >= starts_at)
);

CREATE INDEX vendor_data_legal_holds_active_scope_idx
    ON vendor_data_legal_holds (scope_type, scope_ref, starts_at)
    WHERE released_at IS NULL;

CREATE TABLE commerce_order_restricted_evidence (
    parent_order_id text PRIMARY KEY REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    customer_id text NOT NULL,
    address_snapshot jsonb NOT NULL CHECK (jsonb_typeof(address_snapshot) = 'object'),
    receiver_snapshot jsonb NOT NULL CHECK (jsonb_typeof(receiver_snapshot) = 'object'),
    policy_ref text NOT NULL REFERENCES vendor_privacy_retention_policies(id) ON DELETE RESTRICT,
    sealed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX commerce_order_restricted_evidence_customer_idx
    ON commerce_order_restricted_evidence (customer_id, sealed_at DESC);

CREATE TABLE vendor_privacy_retention_jobs (
    id text PRIMARY KEY,
    parent_order_id text NOT NULL UNIQUE REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    customer_id text NOT NULL,
    policy_ref text NOT NULL REFERENCES vendor_privacy_retention_policies(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','MINIMIZED','DEAD_LETTERED')),
    eligible_at timestamptz NOT NULL,
    next_attempt_at timestamptz NOT NULL,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error_redacted text NOT NULL DEFAULT '',
    last_outcome text NOT NULL DEFAULT '',
    minimized_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'MINIMIZED') = (minimized_at IS NOT NULL))
);

CREATE INDEX vendor_privacy_retention_jobs_due_idx
    ON vendor_privacy_retention_jobs (next_attempt_at, id) WHERE status = 'PENDING';

CREATE OR REPLACE FUNCTION vendor_enqueue_order_privacy_retention()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    policy vendor_privacy_retention_policies%ROWTYPE;
BEGIN
    IF NEW.terminal_at IS NOT NULL AND (TG_OP = 'INSERT' OR OLD.terminal_at IS NULL) THEN
        SELECT * INTO policy FROM vendor_privacy_retention_policies
        WHERE data_class = 'ORDER_RECEIVER_LOCATION' AND active
        ORDER BY version DESC LIMIT 1;
        IF FOUND THEN
            INSERT INTO vendor_privacy_retention_jobs
                (id,parent_order_id,customer_id,policy_ref,eligible_at,next_attempt_at)
            VALUES
                ('vprj_' || gen_random_uuid()::text,NEW.id,NEW.customer_id,policy.id,
                 NEW.terminal_at + policy.retain_for,NEW.terminal_at + policy.retain_for)
            ON CONFLICT (parent_order_id) DO NOTHING;
        END IF;
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER commerce_parent_orders_privacy_retention_trg
AFTER INSERT OR UPDATE OF terminal_at ON commerce_parent_orders
FOR EACH ROW EXECUTE FUNCTION vendor_enqueue_order_privacy_retention();

-- Existing rows remain online throughout the additive change. Operational
-- reads use COALESCE until each row is touched or minimized. The worker
-- discovers pre-migration terminal rows in bounded batches; there is no
-- blocking application-data backfill in this migration.
