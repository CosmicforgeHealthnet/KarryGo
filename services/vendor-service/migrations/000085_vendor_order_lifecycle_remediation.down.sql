DROP INDEX IF EXISTS commerce_logistics_event_dedupe_idx;
ALTER TABLE commerce_logistics_order_events DROP COLUMN IF EXISTS received_at;
ALTER TABLE commerce_vendor_orders DROP COLUMN IF EXISTS refund_refunded_minor;

ALTER TABLE commerce_order_events
    DROP CONSTRAINT IF EXISTS commerce_order_events_action_check;
ALTER TABLE commerce_order_events
    ADD CONSTRAINT commerce_order_events_action_check CHECK (action IN (
        'commerce.order.created','commerce.order.state_changed','commerce.vendor_order.created',
        'commerce.vendor_order.accepted','commerce.vendor_order.ready','commerce.vendor_order.handed_over',
        'commerce.vendor_order.delivered','commerce.vendor_order.completed','commerce.vendor_order.cancelled',
        'commerce.vendor_order.rejected','commerce.vendor_order.picking','commerce.vendor_order.packed',
        'commerce.vendor_order.ready_for_pickup','commerce.vendor_order.substitution_required',
        'commerce.vendor_order.logistics_picked_up','commerce.vendor_order.logistics_delivering',
        'commerce.vendor_order.logistics_delivered'
    ));
