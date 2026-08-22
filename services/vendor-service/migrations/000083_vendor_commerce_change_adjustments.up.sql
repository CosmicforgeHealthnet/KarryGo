-- Section 17F closure: customer commercial changes and immutable financial
-- adjustment evidence.  The original quote/order/payment rows remain the
-- historical agreement; these rows form the append-only chain to a later
-- replacement quote or fulfilment benefit.

CREATE TABLE commerce_order_changes (
    id TEXT PRIMARY KEY,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    customer_id TEXT NOT NULL,
    original_quote_id TEXT NOT NULL,
    original_quote_version BIGINT NOT NULL CHECK (original_quote_version > 0),
    original_quote_fingerprint TEXT NOT NULL CHECK (original_quote_fingerprint ~ '^[0-9a-f]{64}$'),
    original_address_snapshot JSONB NOT NULL CHECK (jsonb_typeof(original_address_snapshot) = 'object'),
    original_fulfilment_plan_id TEXT NOT NULL,
    original_total_minor BIGINT NOT NULL CHECK (original_total_minor >= 0),
    requested_checkout_session_id TEXT NOT NULL,
    requested_fulfilment_plan_id TEXT NOT NULL,
    replacement_quote_id TEXT NOT NULL UNIQUE,
    replacement_quote_version BIGINT NOT NULL CHECK (replacement_quote_version > 0),
    replacement_quote_fingerprint TEXT NOT NULL CHECK (replacement_quote_fingerprint ~ '^[0-9a-f]{64}$'),
    replacement_address_snapshot JSONB NOT NULL CHECK (jsonb_typeof(replacement_address_snapshot) = 'object'),
    replacement_fulfilment_plan_id TEXT NOT NULL,
    replacement_total_minor BIGINT NOT NULL CHECK (replacement_total_minor >= 0),
    replacement_quote_expires_at TIMESTAMPTZ NOT NULL,
    delta_minor BIGINT NOT NULL,
    currency TEXT NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    state TEXT NOT NULL DEFAULT 'AWAITING_CUSTOMER_APPROVAL'
        CHECK (state IN ('CHANGE_REQUESTED','REQUOTE_READY','AWAITING_CUSTOMER_APPROVAL','APPROVED','REJECTED','EXPIRED','APPLIED')),
    approval_fingerprint TEXT NOT NULL DEFAULT '',
    wallet_operation_ref TEXT NOT NULL DEFAULT '',
    wallet_status TEXT NOT NULL DEFAULT 'NONE'
        CHECK (wallet_status IN ('NONE','REQUESTED','HELD','RELEASED','REFUND_REQUESTED','REFUNDED','UNKNOWN','FAILED')),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    approved_at TIMESTAMPTZ,
    applied_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT commerce_order_changes_parent_scope_fk
        FOREIGN KEY (parent_order_id, customer_id) REFERENCES commerce_parent_orders(id, customer_id),
    CONSTRAINT commerce_order_changes_original_quote_fk
        FOREIGN KEY (original_quote_id, customer_id) REFERENCES commerce_quotes(id, customer_id),
    CONSTRAINT commerce_order_changes_replacement_quote_fk
        FOREIGN KEY (replacement_quote_id, customer_id) REFERENCES commerce_quotes(id, customer_id),
    CONSTRAINT commerce_order_changes_delta_chk CHECK (delta_minor = replacement_total_minor - original_total_minor),
    CONSTRAINT commerce_order_changes_approval_chk CHECK (state NOT IN ('APPROVED','APPLIED') OR approval_fingerprint <> ''),
    CONSTRAINT commerce_order_changes_applied_chk CHECK ((state = 'APPLIED') = (applied_at IS NOT NULL)),
    UNIQUE (id, parent_order_id),
    UNIQUE (parent_order_id, requested_checkout_session_id, requested_fulfilment_plan_id)
);

CREATE INDEX commerce_order_changes_customer_idx ON commerce_order_changes(customer_id, created_at DESC, id);
CREATE INDEX commerce_order_changes_recovery_idx ON commerce_order_changes(state, wallet_status, updated_at, id)
    WHERE state IN ('APPROVED','AWAITING_CUSTOMER_APPROVAL');

CREATE TABLE commerce_order_change_applications (
    id TEXT PRIMARY KEY,
    change_id TEXT NOT NULL UNIQUE REFERENCES commerce_order_changes(id) ON DELETE RESTRICT,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    customer_id TEXT NOT NULL,
    replacement_quote_id TEXT NOT NULL,
    replacement_quote_fingerprint TEXT NOT NULL,
    applied_address_snapshot JSONB NOT NULL CHECK (jsonb_typeof(applied_address_snapshot) = 'object'),
    applied_fulfilment_plan_id TEXT NOT NULL,
    applied_total_minor BIGINT NOT NULL CHECK (applied_total_minor >= 0),
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT commerce_order_change_applications_scope_fk
        FOREIGN KEY (change_id, parent_order_id) REFERENCES commerce_order_changes(id, parent_order_id)
);

CREATE TABLE commerce_order_adjustments (
    id TEXT PRIMARY KEY,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    change_id TEXT REFERENCES commerce_order_changes(id) ON DELETE RESTRICT,
    vendor_order_id TEXT REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    adjustment_type TEXT NOT NULL CHECK (adjustment_type IN ('POSITIVE_DELTA','CHEAPER_REPLAN','CANCELLATION_REFUND')),
    original_commitment_minor BIGINT NOT NULL CHECK (original_commitment_minor >= 0),
    eligible_final_minor BIGINT NOT NULL CHECK (eligible_final_minor >= 0),
    difference_minor BIGINT NOT NULL CHECK (difference_minor > 0),
    policy TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('AUTHORIZE','RELEASE','REFUND')),
    idempotency_key TEXT NOT NULL UNIQUE,
    wallet_reference TEXT NOT NULL DEFAULT '',
    wallet_status TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (wallet_status IN ('PENDING','HELD','REFUND_REQUESTED','REFUNDED','RELEASED','UNKNOWN','FAILED')),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT commerce_order_adjustments_scope_fk CHECK (change_id IS NOT NULL OR adjustment_type <> 'POSITIVE_DELTA')
);

CREATE INDEX commerce_order_adjustments_recovery_idx ON commerce_order_adjustments(wallet_status, updated_at, id)
    WHERE wallet_status IN ('PENDING','REFUND_REQUESTED','UNKNOWN');
CREATE INDEX commerce_order_adjustments_parent_idx ON commerce_order_adjustments(parent_order_id, created_at, id);

CREATE TABLE commerce_order_change_events (
    id TEXT PRIMARY KEY,
    change_id TEXT NOT NULL REFERENCES commerce_order_changes(id) ON DELETE RESTRICT,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    customer_id TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN (
        'commerce.order.change_requested','commerce.order.requote_created',
        'commerce.order.requote_approved','commerce.order.change_rejected','commerce.order.change_expired',
        'commerce.order.change_applied','commerce.order.adjustment_requested',
        'commerce.order.adjustment_completed','commerce.order.adjustment_failed')),
    actor_type TEXT NOT NULL CHECK (actor_type IN ('CUSTOMER','SYSTEM','AUTHORITY')),
    actor_ref TEXT NOT NULL DEFAULT '',
    from_state TEXT NOT NULL DEFAULT '',
    to_state TEXT NOT NULL DEFAULT '',
    wallet_reference TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX commerce_order_change_events_change_idx ON commerce_order_change_events(change_id, occurred_at, id);

CREATE OR REPLACE FUNCTION commerce_order_change_evidence_immutable()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'commerce commercial change evidence is immutable';
END;
$$;

CREATE TRIGGER commerce_order_change_applications_immutable
    BEFORE UPDATE OR DELETE ON commerce_order_change_applications
    FOR EACH ROW EXECUTE FUNCTION commerce_order_change_evidence_immutable();
CREATE TRIGGER commerce_order_change_events_immutable
    BEFORE UPDATE OR DELETE ON commerce_order_change_events
    FOR EACH ROW EXECUTE FUNCTION commerce_order_change_evidence_immutable();

CREATE OR REPLACE FUNCTION commerce_order_change_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE'
       OR OLD.parent_order_id <> NEW.parent_order_id
       OR OLD.customer_id <> NEW.customer_id
       OR OLD.original_quote_id <> NEW.original_quote_id
       OR OLD.original_quote_version <> NEW.original_quote_version
       OR OLD.original_quote_fingerprint <> NEW.original_quote_fingerprint
       OR OLD.original_address_snapshot <> NEW.original_address_snapshot
       OR OLD.original_fulfilment_plan_id <> NEW.original_fulfilment_plan_id
       OR OLD.original_total_minor <> NEW.original_total_minor
       OR OLD.requested_checkout_session_id <> NEW.requested_checkout_session_id
       OR OLD.requested_fulfilment_plan_id <> NEW.requested_fulfilment_plan_id
       OR OLD.replacement_quote_id <> NEW.replacement_quote_id
       OR OLD.replacement_quote_version <> NEW.replacement_quote_version
       OR OLD.replacement_quote_fingerprint <> NEW.replacement_quote_fingerprint
       OR OLD.replacement_address_snapshot <> NEW.replacement_address_snapshot
       OR OLD.replacement_fulfilment_plan_id <> NEW.replacement_fulfilment_plan_id
       OR OLD.replacement_total_minor <> NEW.replacement_total_minor
       OR OLD.replacement_quote_expires_at <> NEW.replacement_quote_expires_at
       OR OLD.delta_minor <> NEW.delta_minor
       OR OLD.currency <> NEW.currency
    THEN RAISE EXCEPTION 'commercial change evidence is immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER commerce_order_changes_immutable_evidence
    BEFORE UPDATE OR DELETE ON commerce_order_changes
    FOR EACH ROW EXECUTE FUNCTION commerce_order_change_guard();

CREATE OR REPLACE FUNCTION commerce_order_adjustment_guard()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE'
       OR OLD.parent_order_id <> NEW.parent_order_id
       OR COALESCE(OLD.change_id,'') <> COALESCE(NEW.change_id,'')
       OR COALESCE(OLD.vendor_order_id,'') <> COALESCE(NEW.vendor_order_id,'')
       OR OLD.adjustment_type <> NEW.adjustment_type
       OR OLD.original_commitment_minor <> NEW.original_commitment_minor
       OR OLD.eligible_final_minor <> NEW.eligible_final_minor
       OR OLD.difference_minor <> NEW.difference_minor
       OR OLD.policy <> NEW.policy
       OR OLD.action <> NEW.action
       OR OLD.idempotency_key <> NEW.idempotency_key
    THEN RAISE EXCEPTION 'commercial adjustment evidence is immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER commerce_order_adjustments_immutable_evidence
    BEFORE UPDATE OR DELETE ON commerce_order_adjustments
    FOR EACH ROW EXECUTE FUNCTION commerce_order_adjustment_guard();
