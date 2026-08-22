DROP INDEX IF EXISTS commerce_order_mission_allocations_mission_idx;
DROP TABLE IF EXISTS commerce_order_mission_allocations;

DROP INDEX IF EXISTS commerce_order_missions_pending_idx;
DROP INDEX IF EXISTS commerce_order_missions_parent_idx;
DROP TABLE IF EXISTS commerce_order_missions;

DROP INDEX IF EXISTS commerce_vendor_orders_inventory_pending_idx;
ALTER TABLE commerce_vendor_orders
    DROP CONSTRAINT IF EXISTS commerce_vendor_orders_inventory_state_chk;
ALTER TABLE commerce_vendor_orders DROP COLUMN IF EXISTS inventory_state;
