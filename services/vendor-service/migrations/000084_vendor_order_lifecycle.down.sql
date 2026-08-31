DROP TABLE IF EXISTS commerce_logistics_order_events;
DROP TABLE IF EXISTS commerce_substitution_proposals;
DROP TABLE IF EXISTS commerce_vendor_order_line_outcomes;
ALTER TABLE commerce_vendor_order_lines DROP CONSTRAINT IF EXISTS commerce_vendor_order_lines_id_vendor_uq;
ALTER TABLE commerce_order_events DROP COLUMN IF EXISTS event_version;
ALTER TABLE commerce_order_events DROP COLUMN IF EXISTS idempotency_ref;
ALTER TABLE commerce_order_events DROP COLUMN IF EXISTS correlation_ref;
ALTER TABLE commerce_order_events DROP CONSTRAINT IF EXISTS commerce_order_events_action_check;
ALTER TABLE commerce_order_events ADD CONSTRAINT commerce_order_events_action_check CHECK (action IN (
    'commerce.order.created','commerce.order.state_changed','commerce.vendor_order.created',
    'commerce.vendor_order.accepted','commerce.vendor_order.ready','commerce.vendor_order.handed_over',
    'commerce.vendor_order.delivered','commerce.vendor_order.completed','commerce.vendor_order.cancelled'));
ALTER TABLE commerce_order_adjustments DROP CONSTRAINT IF EXISTS commerce_order_adjustments_adjustment_type_check;
ALTER TABLE commerce_order_adjustments ADD CONSTRAINT commerce_order_adjustments_adjustment_type_check
    CHECK (adjustment_type IN ('POSITIVE_DELTA','CHEAPER_REPLAN','CANCELLATION_REFUND'));
ALTER TABLE commerce_vendor_orders DROP COLUMN IF EXISTS rejection_reason;
ALTER TABLE commerce_vendor_orders DROP COLUMN IF EXISTS refund_operation_ref;
ALTER TABLE commerce_vendor_orders DROP COLUMN IF EXISTS refund_state;
ALTER TABLE commerce_vendor_orders DROP CONSTRAINT IF EXISTS commerce_vendor_orders_status_check;
ALTER TABLE commerce_vendor_orders DROP CONSTRAINT IF EXISTS commerce_vendor_orders_refund_chk;
ALTER TABLE commerce_vendor_orders DROP CONSTRAINT IF EXISTS commerce_vendor_orders_terminal_chk;
ALTER TABLE commerce_vendor_orders DROP CONSTRAINT IF EXISTS commerce_vendor_orders_cancel_evidence_chk;
ALTER TABLE commerce_vendor_orders ADD CONSTRAINT commerce_vendor_orders_status_check
    CHECK (status IN ('CREATED','PREPARING','READY','HANDED_OVER','DELIVERED','COMPLETED','CANCELLED'));
ALTER TABLE commerce_vendor_orders ADD CONSTRAINT commerce_vendor_orders_cancel_evidence_chk
    CHECK ((status = 'CANCELLED') = (cancelled_by <> ''));
ALTER TABLE commerce_vendor_orders ADD CONSTRAINT commerce_vendor_orders_terminal_chk
    CHECK ((status IN ('COMPLETED','CANCELLED')) = (terminal_at IS NOT NULL));
ALTER TABLE commerce_vendor_orders ADD CONSTRAINT commerce_vendor_orders_refund_chk
    CHECK (status = 'CANCELLED' OR refund_allocated_minor = 0);
ALTER TABLE commerce_parent_orders DROP CONSTRAINT IF EXISTS commerce_parent_orders_state_check;
ALTER TABLE commerce_parent_orders DROP CONSTRAINT IF EXISTS commerce_parent_orders_terminal_chk;
ALTER TABLE commerce_parent_orders ADD CONSTRAINT commerce_parent_orders_state_check
    CHECK (state IN ('CREATED','IN_PROGRESS','COMPLETED','PARTIALLY_COMPLETED','CANCELLED'));
ALTER TABLE commerce_parent_orders ADD CONSTRAINT commerce_parent_orders_terminal_chk
    CHECK ((state IN ('COMPLETED','PARTIALLY_COMPLETED','CANCELLED')) = (terminal_at IS NOT NULL));
