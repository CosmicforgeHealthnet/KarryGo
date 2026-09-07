-- Section 32's real order-mission projection exposed a remaining admission
-- constraint on the existing execution aggregate. Keep the allowlist closed
-- while admitting the already-approved Section 29 Taxi Goods authority.
ALTER TABLE commerce_order_missions DROP CONSTRAINT commerce_order_missions_service_check;
ALTER TABLE commerce_order_missions ADD CONSTRAINT commerce_order_missions_service_check
    CHECK (service IN ('DISPATCH','HAULING','TAXI'));
