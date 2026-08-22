-- Corrective Vendor Order Lifecycle evidence. 000084 remains immutable.

ALTER TABLE commerce_vendor_orders
    ADD COLUMN IF NOT EXISTS refund_refunded_minor BIGINT NOT NULL DEFAULT 0
        CHECK (refund_refunded_minor >= 0);

ALTER TABLE commerce_order_events
    DROP CONSTRAINT IF EXISTS commerce_order_events_action_check;
ALTER TABLE commerce_order_events
    ADD CONSTRAINT commerce_order_events_action_check CHECK (action IN (
        'commerce.order.created','commerce.order.state_changed','commerce.vendor_order.created',
        'commerce.vendor_order.paid','commerce.vendor_order.sent_to_vendor','commerce.vendor_order.accepted',
        'commerce.vendor_order.rejected','commerce.vendor_order.preparing',
        'commerce.vendor_order.picking','commerce.vendor_order.substitution_required',
        'commerce.vendor_order.substitution_proposed','commerce.vendor_order.substitution_approved',
        'commerce.vendor_order.substitution_rejected','commerce.vendor_order.packed',
        'commerce.vendor_order.ready','commerce.vendor_order.ready_for_pickup',
        'commerce.vendor_order.handed_over','commerce.vendor_order.picked_up',
        'commerce.vendor_order.delivering','commerce.vendor_order.delivered',
        'commerce.vendor_order.completed','commerce.vendor_order.cancelled',
        'commerce.vendor_order.refund_pending','commerce.vendor_order.refund_unknown','commerce.vendor_order.refund_failed','commerce.vendor_order.refunded',
        'commerce.vendor_order.partially_refunded','commerce.vendor_order.logistics_picked_up',
        'commerce.vendor_order.logistics_delivering','commerce.vendor_order.logistics_delivered'
    ));

ALTER TABLE commerce_logistics_order_events
    ADD COLUMN IF NOT EXISTS received_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS commerce_logistics_event_dedupe_idx
    ON commerce_logistics_order_events(vendor_order_id, source, event_type, source_version);

COMMENT ON COLUMN commerce_vendor_orders.refund_refunded_minor IS
    'Amount confirmed by Wallet; never a client-supplied financial value.';
