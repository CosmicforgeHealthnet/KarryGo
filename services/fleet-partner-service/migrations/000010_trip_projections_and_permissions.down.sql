DELETE FROM fleet_partner_role_permissions
WHERE permission_code IN (
    'marketplace.read', 'marketplace.accept', 'marketplace.reject', 'dispatch.assign',
    'assignments.read', 'assignments.cancel', 'assignments.reassign', 'assignments.respond',
    'trips.read', 'reconciliation.read', 'reconciliation.manage');

DELETE FROM fleet_partner_permissions
WHERE code IN (
    'marketplace.read', 'marketplace.accept', 'marketplace.reject', 'dispatch.assign',
    'assignments.read', 'assignments.cancel', 'assignments.reassign', 'assignments.respond',
    'trips.read', 'reconciliation.read', 'reconciliation.manage');

ALTER TABLE fleet_partner_organization_settings
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_operating_mode;
ALTER TABLE fleet_partner_organization_settings
    DROP COLUMN IF EXISTS operating_mode;

DROP TABLE IF EXISTS fleet_partner_projection_reconciliation_cases;
DROP TABLE IF EXISTS fleet_partner_projection_gaps;
DROP TRIGGER IF EXISTS trg_fleet_partner_trip_history_append_only ON fleet_partner_trip_projection_history;
DROP TABLE IF EXISTS fleet_partner_trip_projection_history;
DROP TABLE IF EXISTS fleet_partner_trip_projections;
