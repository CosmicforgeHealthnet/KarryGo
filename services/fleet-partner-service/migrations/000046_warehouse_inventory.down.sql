DELETE FROM fleet_partner_role_permissions WHERE permission_code IN('warehouses.read','warehouses.manage','warehouse_locations.read','warehouse_locations.manage','warehouse_inventory.read','warehouse_inventory.receive','warehouse_inventory.move','warehouse_inventory.adjust');
DELETE FROM fleet_partner_permissions WHERE code IN('warehouses.read','warehouses.manage','warehouse_locations.read','warehouse_locations.manage','warehouse_inventory.read','warehouse_inventory.receive','warehouse_inventory.move','warehouse_inventory.adjust');
DROP TABLE IF EXISTS fleet_partner_inventory_reservations;
DROP TRIGGER IF EXISTS trg_fleet_inventory_movements_append_only ON fleet_partner_inventory_movements;
DROP TABLE IF EXISTS fleet_partner_inventory_movements;
DROP TABLE IF EXISTS fleet_partner_inventory_items;
DROP TABLE IF EXISTS fleet_partner_storage_locations;
DROP TABLE IF EXISTS fleet_partner_warehouses;
