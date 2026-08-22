-- Section 18: operational VendorOrder lifecycle evidence.
-- This migration extends the Section 17 aggregates; it does not introduce a
-- second order, payment, inventory or logistics authority.

ALTER TABLE commerce_parent_orders DROP CONSTRAINT commerce_parent_orders_state_check;
ALTER TABLE commerce_parent_orders ADD CONSTRAINT commerce_parent_orders_state_check
    CHECK (state IN ('CREATED','IN_PROGRESS','COMPLETED','PARTIALLY_COMPLETED',
                     'CANCELLED','REFUND_PENDING','REFUNDED','PARTIALLY_REFUNDED'));
ALTER TABLE commerce_parent_orders DROP CONSTRAINT commerce_parent_orders_terminal_chk;
ALTER TABLE commerce_parent_orders ADD CONSTRAINT commerce_parent_orders_terminal_chk CHECK (
    (state IN ('COMPLETED','PARTIALLY_COMPLETED','CANCELLED','REFUNDED','PARTIALLY_REFUNDED')) = (terminal_at IS NOT NULL)
);

ALTER TABLE commerce_vendor_orders DROP CONSTRAINT commerce_vendor_orders_status_check;
ALTER TABLE commerce_vendor_orders ADD CONSTRAINT commerce_vendor_orders_status_check
    CHECK (status IN ('CREATED','PREPARING','READY','HANDED_OVER','DELIVERED',
        'COMPLETED','CANCELLED','PENDING_PAYMENT','PAID','SENT_TO_VENDOR',
        'ACCEPTED','PICKING','SUBSTITUTION_REQUIRED','AWAITING_CUSTOMER',
        'PACKING','READY_FOR_PICKUP','PICKED_UP','DELIVERING','REJECTED',
        'REFUND_PENDING','REFUNDED','PARTIALLY_REFUNDED'));

ALTER TABLE commerce_vendor_orders ADD COLUMN IF NOT EXISTS refund_state TEXT NOT NULL DEFAULT 'NONE'
    CHECK (refund_state IN ('NONE','REFUND_PENDING','REFUNDED','PARTIALLY_REFUNDED','UNKNOWN','FAILED'));
ALTER TABLE commerce_vendor_orders ADD COLUMN IF NOT EXISTS refund_operation_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE commerce_vendor_orders ADD COLUMN IF NOT EXISTS rejection_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE commerce_vendor_order_lines ADD CONSTRAINT commerce_vendor_order_lines_id_vendor_uq
    UNIQUE (id, vendor_order_id);
ALTER TABLE commerce_vendor_orders DROP CONSTRAINT commerce_vendor_orders_cancel_evidence_chk;
ALTER TABLE commerce_vendor_orders ADD CONSTRAINT commerce_vendor_orders_cancel_evidence_chk CHECK (
    (status <> 'CANCELLED' OR cancelled_by <> '')
    AND (status = 'CANCELLED' OR cancelled_by = '')
    AND (status <> 'REJECTED' OR length(btrim(rejection_reason)) > 0)
);
ALTER TABLE commerce_vendor_orders DROP CONSTRAINT commerce_vendor_orders_terminal_chk;
ALTER TABLE commerce_vendor_orders ADD CONSTRAINT commerce_vendor_orders_terminal_chk CHECK (
    (status IN ('COMPLETED','CANCELLED','REJECTED','REFUNDED','PARTIALLY_REFUNDED')) = (terminal_at IS NOT NULL)
);
ALTER TABLE commerce_vendor_orders DROP CONSTRAINT commerce_vendor_orders_refund_chk;
ALTER TABLE commerce_vendor_orders ADD CONSTRAINT commerce_vendor_orders_refund_chk CHECK (
    status IN ('CANCELLED','REJECTED','REFUND_PENDING','REFUNDED','PARTIALLY_REFUNDED') OR refund_allocated_minor = 0
);
ALTER TABLE commerce_order_events DROP CONSTRAINT commerce_order_events_action_check;
ALTER TABLE commerce_order_events ADD CONSTRAINT commerce_order_events_action_check CHECK (action IN (
    'commerce.order.created','commerce.order.state_changed','commerce.vendor_order.created',
    'commerce.vendor_order.accepted','commerce.vendor_order.ready','commerce.vendor_order.handed_over',
    'commerce.vendor_order.delivered','commerce.vendor_order.completed','commerce.vendor_order.cancelled',
    'commerce.vendor_order.rejected','commerce.vendor_order.picking','commerce.vendor_order.packed',
    'commerce.vendor_order.ready_for_pickup','commerce.vendor_order.substitution_required',
    'commerce.vendor_order.logistics_picked_up','commerce.vendor_order.logistics_delivering',
    'commerce.vendor_order.logistics_delivered'));
ALTER TABLE commerce_order_adjustments DROP CONSTRAINT commerce_order_adjustments_adjustment_type_check;
ALTER TABLE commerce_order_adjustments ADD CONSTRAINT commerce_order_adjustments_adjustment_type_check
    CHECK (adjustment_type IN ('POSITIVE_DELTA','CHEAPER_REPLAN','CANCELLATION_REFUND',
        'PROMOTION','SUBSTITUTION','VARIABLE_WEIGHT','PARTIAL_QUANTITY','REFUND','APPROVED_CHANGE'));

CREATE TABLE commerce_vendor_order_line_outcomes (
    id TEXT PRIMARY KEY,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    vendor_order_id TEXT NOT NULL REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    order_line_id TEXT NOT NULL UNIQUE REFERENCES commerce_vendor_order_lines(id) ON DELETE RESTRICT,
    ordered_quantity BIGINT NOT NULL CHECK (ordered_quantity > 0),
    picked_quantity BIGINT NOT NULL DEFAULT 0 CHECK (picked_quantity >= 0),
    unavailable_quantity BIGINT NOT NULL DEFAULT 0 CHECK (unavailable_quantity >= 0),
    actual_measure_quantity BIGINT NOT NULL DEFAULT 0 CHECK (actual_measure_quantity >= 0),
    outcome TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (outcome IN ('PENDING','FULLY_AVAILABLE','PARTIALLY_AVAILABLE','UNAVAILABLE')),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT commerce_line_outcome_scope_fk FOREIGN KEY (order_line_id, vendor_order_id)
        REFERENCES commerce_vendor_order_lines(id, vendor_order_id),
    CONSTRAINT commerce_line_outcome_quantity_chk CHECK (
        picked_quantity + unavailable_quantity <= ordered_quantity
    )
);
CREATE INDEX commerce_line_outcome_vendor_idx
    ON commerce_vendor_order_line_outcomes(vendor_order_id, outcome, updated_at, id);

CREATE TABLE commerce_substitution_proposals (
    id TEXT PRIMARY KEY,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    vendor_order_id TEXT NOT NULL REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    order_line_id TEXT NOT NULL REFERENCES commerce_vendor_order_lines(id) ON DELETE RESTRICT,
    customer_id TEXT NOT NULL,
    business_id TEXT NOT NULL,
    branch_id TEXT NOT NULL,
    proposed_item_ref TEXT NOT NULL,
    proposed_variant_ref TEXT NOT NULL DEFAULT '',
    proposed_catalogue_ref TEXT NOT NULL DEFAULT '',
    replacement_snapshot JSONB NOT NULL CHECK (jsonb_typeof(replacement_snapshot) = 'object'),
    proposed_quantity BIGINT NOT NULL CHECK (proposed_quantity > 0),
    proposed_price_minor BIGINT NOT NULL CHECK (proposed_price_minor >= 0),
    price_difference_minor BIGINT NOT NULL,
    reason TEXT NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 500),
    expires_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','APPROVED','REJECTED','EXPIRED','CANCELLED')),
    response_ref TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT commerce_substitution_scope_fk FOREIGN KEY (order_line_id, vendor_order_id)
        REFERENCES commerce_vendor_order_lines(id, vendor_order_id),
    CONSTRAINT commerce_substitution_customer_scope_fk FOREIGN KEY (parent_order_id, customer_id)
        REFERENCES commerce_parent_orders(id, customer_id)
);
CREATE INDEX commerce_substitution_customer_idx
    ON commerce_substitution_proposals(customer_id, created_at DESC, id);
CREATE INDEX commerce_substitution_vendor_idx
    ON commerce_substitution_proposals(business_id, branch_id, status, updated_at, id);
CREATE INDEX commerce_substitution_pending_idx
    ON commerce_substitution_proposals(status, expires_at, id) WHERE status = 'PENDING';

CREATE TABLE commerce_logistics_order_events (
    id TEXT PRIMARY KEY,
    event_id TEXT NOT NULL UNIQUE,
    source TEXT NOT NULL CHECK (source IN ('DISPATCH','HAULING')),
    mission_ref TEXT NOT NULL,
    vendor_order_id TEXT NOT NULL REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    parent_order_id TEXT NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL CHECK (event_type IN ('PICKED_UP','DELIVERING','DELIVERED')),
    source_version BIGINT NOT NULL CHECK (source_version > 0),
    occurred_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(payload) = 'object'),
    applied BOOLEAN NOT NULL DEFAULT false,
    ignored_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT commerce_logistics_event_scope_fk FOREIGN KEY (vendor_order_id, parent_order_id)
        REFERENCES commerce_vendor_orders(id, parent_order_id)
);
CREATE UNIQUE INDEX commerce_logistics_event_order_version_idx
    ON commerce_logistics_order_events(vendor_order_id, source, source_version);
CREATE INDEX commerce_logistics_event_lookup_idx
    ON commerce_logistics_order_events(mission_ref, source, source_version);

ALTER TABLE commerce_order_events ADD COLUMN IF NOT EXISTS correlation_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE commerce_order_events ADD COLUMN IF NOT EXISTS idempotency_ref TEXT NOT NULL DEFAULT '';
ALTER TABLE commerce_order_events ADD COLUMN IF NOT EXISTS event_version BIGINT NOT NULL DEFAULT 1;

COMMENT ON TABLE commerce_vendor_order_line_outcomes IS
    'Section 18 mutable picking outcome; the purchased line snapshot remains immutable.';
COMMENT ON TABLE commerce_substitution_proposals IS
    'Section 18 customer decision evidence for a replacement item and its frozen snapshot.';
COMMENT ON TABLE commerce_logistics_order_events IS
    'Section 18 deduplicated, monotonic Dispatch/Hauling evidence; Vendor Service never fabricates it.';
