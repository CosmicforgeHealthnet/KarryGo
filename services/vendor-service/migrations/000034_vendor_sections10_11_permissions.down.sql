DELETE FROM vendor_role_permissions WHERE permission_id IN
 (SELECT id FROM vendor_permissions WHERE id LIKE 'perm_production_%' OR id LIKE 'perm_cloud_kitchen_%' OR id LIKE 'perm_corporate_plan_%' OR id LIKE 'perm_pastry_%');
DELETE FROM vendor_permissions WHERE id LIKE 'perm_production_%' OR id LIKE 'perm_cloud_kitchen_%' OR id LIKE 'perm_corporate_plan_%' OR id LIKE 'perm_pastry_%';

-- Restore the pre-Sections-10/11 legacy role shape on rollback.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_kitchen_pastry', id FROM vendor_permissions
WHERE code='vendor.production.manage'
ON CONFLICT DO NOTHING;
