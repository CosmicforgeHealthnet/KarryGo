DELETE FROM fleet_partner_role_permissions WHERE permission_code IN
    ('privacy.retention.read', 'privacy.retention.manage');
DELETE FROM fleet_partner_permissions WHERE code IN
    ('privacy.retention.read', 'privacy.retention.manage');

DROP TABLE IF EXISTS fleet_partner_retention_policies;
