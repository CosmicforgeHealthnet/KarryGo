DROP INDEX IF EXISTS inventory_reservation_allocations_partial_idx;
ALTER TABLE inventory_reservation_allocations
    DROP CONSTRAINT IF EXISTS inventory_reservation_allocations_quantity_reconcile_ck;
ALTER TABLE inventory_reservation_allocations
    DROP COLUMN IF EXISTS committed_quantity,
    DROP COLUMN IF EXISTS released_quantity;
