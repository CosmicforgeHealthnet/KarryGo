DELETE FROM fleet_partner_role_permissions WHERE permission_code IN
    ('security.settings.read', 'security.settings.manage', 'security.sessions.revoke', 'security.credentials.manage');
DELETE FROM fleet_partner_permissions WHERE code IN
    ('security.settings.read', 'security.settings.manage', 'security.sessions.revoke', 'security.credentials.manage');

DROP TABLE IF EXISTS fleet_partner_security_settings;
