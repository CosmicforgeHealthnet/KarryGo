-- Rollback Section 15 shipments and cargo items.

DELETE FROM fleet_partner_role_permissions WHERE permission_code LIKE 'shipments.%';
DELETE FROM fleet_partner_permissions WHERE code LIKE 'shipments.%';

DROP INDEX IF EXISTS idx_fleet_cargo_items_restricted;
DROP INDEX IF EXISTS idx_fleet_cargo_items_shipment;
DROP TABLE IF EXISTS fleet_partner_cargo_items;

DROP TRIGGER IF EXISTS trg_fleet_shipment_history_append_only
    ON fleet_partner_shipment_history;
DROP INDEX IF EXISTS idx_fleet_shipment_history;
DROP TABLE IF EXISTS fleet_partner_shipment_history;

DROP INDEX IF EXISTS idx_fleet_shipments_branch;
DROP INDEX IF EXISTS idx_fleet_shipments_status;
DROP INDEX IF EXISTS idx_fleet_shipments_order;
DROP INDEX IF EXISTS idx_fleet_shipments_cursor;
DROP TABLE IF EXISTS fleet_partner_shipments;
