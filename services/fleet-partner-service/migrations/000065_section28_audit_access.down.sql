DELETE FROM fleet_partner_role_permissions WHERE permission_code IN ('audit.read', 'audit.read_sensitive');
DELETE FROM fleet_partner_permissions WHERE code IN ('audit.read', 'audit.read_sensitive');
