DELETE FROM fleet_partner_role_permissions WHERE permission_code IN
 ('packages.read','packages.manage','packages.scan','custody.read','custody.record');
DELETE FROM fleet_partner_permissions WHERE code IN
 ('packages.read','packages.manage','packages.scan','custody.read','custody.record');
DROP TRIGGER IF EXISTS trg_fleet_custody_events_append_only ON fleet_partner_custody_events;
DROP INDEX IF EXISTS idx_fleet_custody_events;
DROP TABLE IF EXISTS fleet_partner_custody_events;
DROP TRIGGER IF EXISTS trg_fleet_package_scans_append_only ON fleet_partner_package_scans;
DROP INDEX IF EXISTS idx_fleet_package_scans;
DROP TABLE IF EXISTS fleet_partner_package_scans;
DROP TRIGGER IF EXISTS trg_fleet_package_history_append_only ON fleet_partner_package_history;
DROP INDEX IF EXISTS idx_fleet_package_history;
DROP TABLE IF EXISTS fleet_partner_package_history;
DROP INDEX IF EXISTS idx_fleet_packages_status;
DROP INDEX IF EXISTS idx_fleet_packages_shipment;
DROP TABLE IF EXISTS fleet_partner_packages;
