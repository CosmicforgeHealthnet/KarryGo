-- Rollback the Section 15 service catalogue.

DELETE FROM fleet_partner_role_permissions WHERE permission_code LIKE 'servicecatalog.%';
DELETE FROM fleet_partner_permissions WHERE code LIKE 'servicecatalog.%';

DROP INDEX IF EXISTS idx_fleet_servicecatalog_branch;
DROP INDEX IF EXISTS idx_fleet_servicecatalog_category;
DROP INDEX IF EXISTS idx_fleet_servicecatalog_status;
DROP INDEX IF EXISTS idx_fleet_servicecatalog_cursor;
DROP INDEX IF EXISTS idx_fleet_servicecatalog_code;
DROP TABLE IF EXISTS fleet_partner_service_catalog_items;
