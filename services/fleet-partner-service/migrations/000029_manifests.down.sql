DELETE FROM fleet_partner_role_permissions WHERE permission_code LIKE 'manifests.%';DELETE FROM fleet_partner_permissions WHERE code LIKE 'manifests.%';
DROP TRIGGER IF EXISTS trg_fleet_manifest_snapshots_append_only ON fleet_partner_manifest_snapshots;DROP TABLE IF EXISTS fleet_partner_manifest_snapshots;
DROP TRIGGER IF EXISTS trg_fleet_manifest_history_append_only ON fleet_partner_manifest_history;DROP TABLE IF EXISTS fleet_partner_manifest_history;
DROP INDEX IF EXISTS idx_fleet_manifest_items_resource;DROP TABLE IF EXISTS fleet_partner_manifest_items;DROP INDEX IF EXISTS idx_fleet_manifests_cursor;DROP TABLE IF EXISTS fleet_partner_manifests;
