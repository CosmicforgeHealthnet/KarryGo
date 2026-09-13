-- Corporate marketplace permissions for the Fleet manager surface.
--
-- The marketplace engine already exists and is exercised by the internal
-- (HMAC) provider routes. These permissions gate the corporate read/write
-- routes that let a Fleet manager see offers, accept or decline them, assign
-- them to a provider, and manage the resulting assignment.
--
-- Every permission is seeded inactive where the feature gate is not yet
-- enabled (see the paired flag migration), so a permission existing does not
-- mean the endpoint works: the handler still consults its feature flag.

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('marketplace.read',     'marketplace', 'Read marketplace offers and assignments', true),
    ('marketplace.accept',   'marketplace', 'Accept a marketplace offer for the organization', true),
    ('marketplace.decline',  'marketplace', 'Decline a marketplace offer for the organization', true),
    ('marketplace.assign',   'marketplace', 'Assign an accepted request to a provider', true),
    ('marketplace.cancel',   'marketplace', 'Cancel a marketplace assignment', true),
    ('marketplace.reassign', 'marketplace', 'Reassign a marketplace assignment to another provider', true)
ON CONFLICT (code) DO NOTHING;

-- Owner receives every marketplace permission, matching the pattern used by
-- every other feature migration.
INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_owner', code
FROM fleet_partner_permissions
WHERE code IN ('marketplace.read', 'marketplace.accept', 'marketplace.decline',
               'marketplace.assign', 'marketplace.cancel', 'marketplace.reassign')
ON CONFLICT DO NOTHING;

-- The operations manager and dispatcher are the primary marketplace actors:
-- they see offers, accept, decline, assign, cancel and reassign.
INSERT INTO fleet_partner_role_permissions (role_id, permission_code) VALUES
    ('rol_system_admin',      'marketplace.read'),
    ('rol_system_admin',      'marketplace.accept'),
    ('rol_system_admin',      'marketplace.decline'),
    ('rol_system_admin',      'marketplace.assign'),
    ('rol_system_admin',      'marketplace.cancel'),
    ('rol_system_admin',      'marketplace.reassign'),

    ('rol_system_operations', 'marketplace.read'),
    ('rol_system_operations', 'marketplace.accept'),
    ('rol_system_operations', 'marketplace.decline'),
    ('rol_system_operations', 'marketplace.assign'),
    ('rol_system_operations', 'marketplace.cancel'),
    ('rol_system_operations', 'marketplace.reassign'),

    ('rol_system_dispatcher', 'marketplace.read'),
    ('rol_system_dispatcher', 'marketplace.accept'),
    ('rol_system_dispatcher', 'marketplace.decline'),
    ('rol_system_dispatcher', 'marketplace.assign'),
    ('rol_system_dispatcher', 'marketplace.cancel'),
    ('rol_system_dispatcher', 'marketplace.reassign'),

    -- Fleet manager can read and reassign but does not commit the
    -- organization to new work.
    ('rol_system_fleet',      'marketplace.read'),
    ('rol_system_fleet',      'marketplace.reassign'),

    -- Analyst and view-only see offers but take no action.
    ('rol_system_analyst',    'marketplace.read'),
    ('rol_system_view_only',  'marketplace.read')
ON CONFLICT DO NOTHING;