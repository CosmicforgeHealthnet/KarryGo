DELETE FROM fleet_partner_role_permissions WHERE permission_code = 'privacy.retention.execute';
DELETE FROM fleet_partner_permissions WHERE code = 'privacy.retention.execute';
DROP INDEX IF EXISTS idx_fleet_partner_retention_run_records_run;
DROP TABLE IF EXISTS fleet_partner_retention_run_records;
DROP INDEX IF EXISTS idx_fleet_partner_retention_runs_org;
DROP TABLE IF EXISTS fleet_partner_retention_runs;
