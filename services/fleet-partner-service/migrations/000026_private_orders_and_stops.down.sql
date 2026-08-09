-- Rollback Section 15 private orders and order stops.

DELETE FROM fleet_partner_role_permissions WHERE permission_code LIKE 'orders.%';
DELETE FROM fleet_partner_permissions WHERE code LIKE 'orders.%';

DROP INDEX IF EXISTS idx_fleet_conversion_intent_unconsumed;
DROP INDEX IF EXISTS idx_fleet_conversion_intent_order;
ALTER TABLE fleet_partner_quotation_conversion_intents
    DROP CONSTRAINT IF EXISTS fleet_partner_conversion_intent_order_fkey;
ALTER TABLE fleet_partner_quotation_conversion_intents
    DROP COLUMN IF EXISTS private_order_id,
    DROP COLUMN IF EXISTS consumed_at;

DROP INDEX IF EXISTS idx_fleet_order_stops_order;
DROP TABLE IF EXISTS fleet_partner_order_stops;

DROP TRIGGER IF EXISTS trg_fleet_order_history_append_only
    ON fleet_partner_private_order_history;
DROP INDEX IF EXISTS idx_fleet_order_history;
DROP TABLE IF EXISTS fleet_partner_private_order_history;

DROP INDEX IF EXISTS idx_fleet_orders_branch;
DROP INDEX IF EXISTS idx_fleet_orders_customer;
DROP INDEX IF EXISTS idx_fleet_orders_status;
DROP INDEX IF EXISTS idx_fleet_orders_cursor;
DROP INDEX IF EXISTS idx_fleet_orders_number;
DROP TABLE IF EXISTS fleet_partner_private_orders;
