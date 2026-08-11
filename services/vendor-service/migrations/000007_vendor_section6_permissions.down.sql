DELETE FROM vendor_role_permissions
WHERE permission_id IN (
    SELECT id FROM vendor_permissions WHERE code IN (
        'vendor.capabilities.read','vendor.capabilities.manage','vendor.brands.read','vendor.brands.manage',
        'vendor.branch.hours.manage','vendor.branch.settings.manage','vendor.branch.capabilities.manage'
    )
);
DELETE FROM vendor_permissions WHERE code IN (
    'vendor.capabilities.read','vendor.capabilities.manage','vendor.brands.read','vendor.brands.manage',
    'vendor.branch.hours.manage','vendor.branch.settings.manage','vendor.branch.capabilities.manage'
);
