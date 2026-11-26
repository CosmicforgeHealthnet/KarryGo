DELETE FROM fleet_partner_role_permissions WHERE permission_code IN('exceptions.read','exceptions.manage','exceptions.resolve','notification_preferences.manage','notification_preferences.manage_organization');
DELETE FROM fleet_partner_permissions WHERE code IN('exceptions.read','exceptions.manage','exceptions.resolve','notification_preferences.manage','notification_preferences.manage_organization');
DROP TABLE IF EXISTS fleet_partner_notification_preferences;
DROP TRIGGER IF EXISTS trg_fleet_operational_exception_history_append_only ON fleet_partner_operational_exception_history;
DROP TABLE IF EXISTS fleet_partner_operational_exception_history;
DROP TABLE IF EXISTS fleet_partner_operational_exceptions;
