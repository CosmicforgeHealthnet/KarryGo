DROP TRIGGER IF EXISTS commerce_payment_events_immutable ON commerce_payment_events;

DROP TABLE IF EXISTS commerce_payment_events;
DROP TABLE IF EXISTS commerce_payment_legs;
DROP TABLE IF EXISTS commerce_payment_operations;

DROP FUNCTION IF EXISTS commerce_payment_reject_mutation();

DROP INDEX IF EXISTS commerce_parent_orders_payment_state_idx;
ALTER TABLE commerce_parent_orders DROP COLUMN IF EXISTS payment_state;
