-- Customer V4 §22: Vendor Commerce publishes one purchase-level Activity row
-- per ParentOrder (authority VENDOR_COMMERCE, movement_type COMMERCE_ORDER).
-- Without it a Food/Shop purchase with vendor-owned delivery, or one
-- cancelled/refunded before any platform mission, never appeared in Activity.
-- Execution rows stay owned by Taxi/Dispatch/Hauling and join the purchase
-- through parent_context_ref. Re-runnable: this runner applies every file.
ALTER TABLE customer_movement_history_projection
    DROP CONSTRAINT IF EXISTS customer_movement_history_projection_authority_check;
ALTER TABLE customer_movement_history_projection
    ADD CONSTRAINT customer_movement_history_projection_authority_check
    CHECK (authority IN ('TAXI','DISPATCH','HAULING','VENDOR_COMMERCE'));

ALTER TABLE customer_movement_history_projection
    DROP CONSTRAINT IF EXISTS customer_movement_history_projection_movement_type_check;
ALTER TABLE customer_movement_history_projection
    ADD CONSTRAINT customer_movement_history_projection_movement_type_check
    CHECK (movement_type IN ('PASSENGER','GOODS','DISPATCH','HAULAGE','COMMERCE_ORDER'));

-- A purchase row is always a Food or Shop purchase.
ALTER TABLE customer_movement_history_projection
    DROP CONSTRAINT IF EXISTS customer_movement_history_projection_commerce_order_chk;
ALTER TABLE customer_movement_history_projection
    ADD CONSTRAINT customer_movement_history_projection_commerce_order_chk
    CHECK ((authority = 'VENDOR_COMMERCE') = (movement_type = 'COMMERCE_ORDER')
           AND (authority <> 'VENDOR_COMMERCE' OR product_code IN ('FOOD','SHOP')));
