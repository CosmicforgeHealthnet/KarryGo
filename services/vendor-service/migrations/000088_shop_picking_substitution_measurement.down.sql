DROP TABLE IF EXISTS commerce_order_adjustment_evidence;
DROP TABLE IF EXISTS commerce_payment_capability_snapshots;
DROP TABLE IF EXISTS commerce_variable_measurements;
DROP TABLE IF EXISTS commerce_order_line_quality_results;
DROP TABLE IF EXISTS commerce_order_line_quality_preferences;
DROP TABLE IF EXISTS commerce_substitution_case_proposals;
DROP TABLE IF EXISTS commerce_substitution_cases;
DROP TABLE IF EXISTS commerce_shop_picking_snapshots;

ALTER TABLE commerce_vendor_order_line_outcomes
    DROP COLUMN IF EXISTS measurement_mode,
    DROP COLUMN IF EXISTS quality_result,
    DROP COLUMN IF EXISTS picking_state,
    DROP COLUMN IF EXISTS substitution_policy,
    DROP COLUMN IF EXISTS substitution_policy_snapshot,
    DROP COLUMN IF EXISTS quality_preference_snapshot,
    DROP COLUMN IF EXISTS expected_vendor_order_version,
    DROP COLUMN IF EXISTS correlation_ref,
    DROP COLUMN IF EXISTS idempotency_ref;

ALTER TABLE commerce_order_events DROP CONSTRAINT IF EXISTS commerce_order_events_action_check;
ALTER TABLE commerce_order_events ADD CONSTRAINT commerce_order_events_action_check CHECK (action IN (
    'commerce.order.created','commerce.order.state_changed','commerce.vendor_order.created','commerce.vendor_order.paid','commerce.vendor_order.sent_to_vendor','commerce.vendor_order.accepted','commerce.vendor_order.rejected','commerce.vendor_order.preparing','commerce.vendor_order.picking','commerce.vendor_order.substitution_required','commerce.vendor_order.substitution_proposed','commerce.vendor_order.substitution_approved','commerce.vendor_order.substitution_rejected','commerce.vendor_order.packed','commerce.vendor_order.ready','commerce.vendor_order.ready_for_pickup','commerce.vendor_order.handed_over','commerce.vendor_order.picked_up','commerce.vendor_order.delivering','commerce.vendor_order.delivered','commerce.vendor_order.completed','commerce.vendor_order.cancelled','commerce.vendor_order.refund_pending','commerce.vendor_order.refund_unknown','commerce.vendor_order.refund_failed','commerce.vendor_order.refunded','commerce.vendor_order.partially_refunded','commerce.vendor_order.logistics_picked_up','commerce.vendor_order.logistics_delivering','commerce.vendor_order.logistics_delivered'));
