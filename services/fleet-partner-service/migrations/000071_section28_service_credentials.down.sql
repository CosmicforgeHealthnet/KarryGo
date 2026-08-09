DELETE FROM fleet_partner_role_permissions WHERE permission_code IN
    ('security.service_credentials.read', 'security.service_credentials.manage');
DELETE FROM fleet_partner_permissions WHERE code IN
    ('security.service_credentials.read', 'security.service_credentials.manage');
DROP INDEX IF EXISTS idx_fleet_partner_service_credentials_lookup;
DROP TABLE IF EXISTS fleet_partner_service_credentials;
