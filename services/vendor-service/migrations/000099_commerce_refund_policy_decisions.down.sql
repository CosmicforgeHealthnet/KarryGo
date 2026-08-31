DROP TRIGGER IF EXISTS commerce_refund_policy_evidence_immutable ON commerce_refund_policy_decisions;
DROP FUNCTION IF EXISTS prevent_commerce_refund_policy_evidence_mutation();
DROP TABLE IF EXISTS commerce_refund_policy_decisions;
ALTER TABLE commerce_vendor_orders DROP COLUMN IF EXISTS refund_policy_version;
ALTER TABLE commerce_logistics_order_events
    DROP CONSTRAINT IF EXISTS commerce_logistics_order_events_event_type_check;
ALTER TABLE commerce_logistics_order_events
    ADD CONSTRAINT commerce_logistics_order_events_event_type_check
    CHECK (event_type IN ('PICKED_UP','DELIVERING','DELIVERED')) NOT VALID;
