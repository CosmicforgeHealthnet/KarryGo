DROP TRIGGER IF EXISTS commerce_allocation_snapshot_immutable ON commerce_payment_allocations;
DROP FUNCTION IF EXISTS guard_commerce_allocation_snapshot_immutability();
DROP TRIGGER IF EXISTS commerce_intent_snapshot_immutable ON commerce_payment_intents;
DROP FUNCTION IF EXISTS guard_commerce_intent_snapshot_immutability();
DROP TABLE IF EXISTS commerce_financial_operations;
DROP TABLE IF EXISTS commerce_payment_allocations;
DROP TABLE IF EXISTS commerce_payment_intents;
