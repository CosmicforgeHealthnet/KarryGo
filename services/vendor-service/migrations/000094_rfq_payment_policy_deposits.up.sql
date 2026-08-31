-- Section 21 Prompt 2: immutable RFQ payment and cancellation evidence.
-- Wallet remains the sole money authority; these rows are commerce bindings
-- and recovery projections only.

ALTER TABLE commerce_rfq_quote_revisions
    ADD COLUMN vendor_terms jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN payment_policy jsonb NOT NULL DEFAULT '{"type":"FULL_PAYMENT"}'::jsonb,
    ADD COLUMN deposit_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN cancellation_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN created_by text NOT NULL DEFAULT '';

ALTER TABLE commerce_rfq_checkout_bindings
    ADD COLUMN payment_policy jsonb NOT NULL DEFAULT '{"type":"FULL_PAYMENT"}'::jsonb,
    ADD COLUMN cancellation_policy jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE commerce_rfq_payment_obligations (
    id text PRIMARY KEY,
    rfq_id text NOT NULL UNIQUE REFERENCES commerce_rfqs(id) ON DELETE RESTRICT,
    binding_id text NOT NULL UNIQUE REFERENCES commerce_rfq_checkout_bindings(id) ON DELETE RESTRICT,
    checkout_quote_id text NOT NULL UNIQUE REFERENCES commerce_quotes(id) ON DELETE RESTRICT,
    payment_policy jsonb NOT NULL,
    cancellation_policy jsonb NOT NULL,
    total_minor bigint NOT NULL CHECK (total_minor >= 0),
    required_minor bigint NOT NULL CHECK (required_minor >= 0 AND required_minor <= total_minor),
    remaining_minor bigint NOT NULL CHECK (remaining_minor >= 0 AND remaining_minor <= total_minor),
    currency text NOT NULL,
    status text NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('NOT_REQUIRED','PENDING','AUTHORIZING','HELD','UNKNOWN','FAILED','REFUND_PENDING','REFUNDED','ORDER_CREATED','CONVERTED')),
    wallet_operation_ref text,
    wallet_idempotency_key text NOT NULL UNIQUE,
    parent_order_id text,
    payment_operation_id text,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (status <> 'HELD' OR wallet_operation_ref IS NOT NULL),
    CHECK (status <> 'REFUNDED' OR wallet_operation_ref IS NOT NULL)
);
CREATE INDEX commerce_rfq_payment_obligations_recovery_idx
    ON commerce_rfq_payment_obligations (status, updated_at, id);
