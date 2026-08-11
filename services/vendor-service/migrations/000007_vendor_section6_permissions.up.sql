INSERT INTO vendor_permissions (id, code, description, category, is_high_risk) VALUES
    ('perm_vendor_capabilities_read', 'vendor.capabilities.read', 'View configured business and branch capabilities.', 'business', false),
    ('perm_vendor_capabilities_manage', 'vendor.capabilities.manage', 'Request and manage business capabilities.', 'business', false),
    ('perm_vendor_brands_read', 'vendor.brands.read', 'View vendor brands.', 'business', false),
    ('perm_vendor_brands_manage', 'vendor.brands.manage', 'Create and update vendor brands and media references.', 'business', false),
    ('perm_vendor_branch_hours_manage', 'vendor.branch.hours.manage', 'Manage operating hours for an authorized branch.', 'business', false),
    ('perm_vendor_branch_settings_manage', 'vendor.branch.settings.manage', 'Manage supported operations settings for an authorized branch.', 'business', false),
    ('perm_vendor_branch_capabilities_manage', 'vendor.branch.capabilities.manage', 'Configure capabilities for an authorized branch.', 'business', false)
ON CONFLICT (id) DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_owner', id FROM vendor_permissions
WHERE code IN ('vendor.capabilities.read','vendor.capabilities.manage','vendor.brands.read','vendor.brands.manage',
               'vendor.branch.hours.manage','vendor.branch.settings.manage','vendor.branch.capabilities.manage')
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_general_manager', id FROM vendor_permissions
WHERE code IN ('vendor.capabilities.read','vendor.capabilities.manage','vendor.brands.read','vendor.brands.manage',
               'vendor.branch.hours.manage','vendor.branch.settings.manage','vendor.branch.capabilities.manage')
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_branch_manager', id FROM vendor_permissions
WHERE code IN ('vendor.capabilities.read','vendor.brands.read','vendor.branch.hours.manage',
               'vendor.branch.settings.manage','vendor.branch.capabilities.manage')
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_analyst_auditor', id FROM vendor_permissions
WHERE code IN ('vendor.capabilities.read','vendor.brands.read')
ON CONFLICT DO NOTHING;

