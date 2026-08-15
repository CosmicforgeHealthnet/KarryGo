-- Section 8C permissions. Read, create and manage are intentionally distinct;
-- assignment and lifecycle commands remain part of manage so roles cannot be
-- left with a confusing half-state workflow.
INSERT INTO vendor_permissions(id,code,description,category,is_high_risk) VALUES
 ('perm_vendor_crm_interactions_read','vendor.crm.interactions.read','View scoped CRM interactions and revision history.','crm',false),
 ('perm_vendor_crm_interactions_create','vendor.crm.interactions.create','Record scoped CRM interactions.','crm',false),
 ('perm_vendor_crm_interactions_manage','vendor.crm.interactions.manage','Correct scoped CRM interactions with immutable history.','crm',false),
 ('perm_vendor_crm_tasks_read','vendor.crm.tasks.read','View scoped CRM tasks and task history.','crm',false),
 ('perm_vendor_crm_tasks_create','vendor.crm.tasks.create','Create scoped CRM tasks.','crm',false),
 ('perm_vendor_crm_tasks_manage','vendor.crm.tasks.manage','Update, assign and transition scoped CRM tasks.','crm',false)
ON CONFLICT(id) DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM vendor_roles r CROSS JOIN vendor_permissions p
WHERE r.id IN ('role_owner','role_general_manager','role_crm_support','role_marketing','role_branch_manager')
  AND p.code IN ('vendor.crm.interactions.read','vendor.crm.interactions.create','vendor.crm.interactions.manage',
                 'vendor.crm.tasks.read','vendor.crm.tasks.create','vendor.crm.tasks.manage')
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_analyst_auditor',id FROM vendor_permissions
WHERE code IN ('vendor.crm.interactions.read','vendor.crm.tasks.read')
ON CONFLICT DO NOTHING;

-- Deliberately unmapped: finance, kitchen/pastry, shop/inventory, procurement
-- and vendor rider. Section 8C is relationship workflow, not operations work.
