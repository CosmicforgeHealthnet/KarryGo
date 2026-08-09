DELETE FROM fleet_partner_role_permissions WHERE permission_code LIKE 'exports.%';
DELETE FROM fleet_partner_permissions WHERE code LIKE 'exports.%';

DROP TRIGGER IF EXISTS trg_fleet_export_attempts_append_only ON fleet_partner_export_attempts;
DROP TABLE IF EXISTS fleet_partner_export_attempts;
DROP TABLE IF EXISTS fleet_partner_export_jobs;
