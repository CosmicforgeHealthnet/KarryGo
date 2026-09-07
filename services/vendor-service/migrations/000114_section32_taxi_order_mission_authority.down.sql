-- Rollback intentionally refuses if Taxi mission evidence exists.
ALTER TABLE commerce_order_missions DROP CONSTRAINT commerce_order_missions_service_check;
ALTER TABLE commerce_order_missions ADD CONSTRAINT commerce_order_missions_service_check
    CHECK (service IN ('DISPATCH','HAULING'));
