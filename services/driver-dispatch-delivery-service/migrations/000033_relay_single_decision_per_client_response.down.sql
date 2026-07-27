-- Reverse of 000033: restore the per-type index, drop the provider-scoped one.
CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_relay_idempotency
    ON dispatch_fleet_relay_operations (operation_type, idempotency_key);
DROP INDEX IF EXISTS uq_dispatch_relay_single_decision;
