DELETE FROM vendor_role_permissions WHERE role_id IN (
    'role_owner', 'role_general_manager', 'role_branch_manager', 'role_kitchen_pastry',
    'role_shop_inventory', 'role_procurement', 'role_crm_support', 'role_marketing',
    'role_finance', 'role_vendor_rider', 'role_analyst_auditor'
);
DELETE FROM vendor_roles WHERE is_system = true;
DELETE FROM vendor_permissions WHERE id LIKE 'perm_vendor_%';
