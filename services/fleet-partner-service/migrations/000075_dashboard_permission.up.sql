-- Dashboard read permission. The dashboard is a read-only aggregation over
-- other features' data and owns no tables of its own, so this migration
-- seeds only the permission and its role grants.

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('dashboard.read', 'dashboard', 'Read the aggregated operations dashboard', true)
ON CONFLICT (code) DO NOTHING;

-- The owner receives every active permission, matching the pattern used by
-- every other feature migration.
INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_owner', code
FROM fleet_partner_permissions
WHERE code IN ('dashboard.read')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code) VALUES
    ('rol_system_admin',      'dashboard.read'),
    ('rol_system_operations', 'dashboard.read'),
    ('rol_system_dispatcher', 'dashboard.read'),
    ('rol_system_fleet',      'dashboard.read'),
    ('rol_system_analyst',    'dashboard.read'),
    ('rol_system_view_only',  'dashboard.read')
ON CONFLICT DO NOTHING;