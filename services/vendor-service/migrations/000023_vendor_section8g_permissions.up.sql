-- Section 8G permissions.
--
-- Five, not nine. Corporate interactions and tasks deliberately reuse the
-- Section 8C activity permissions rather than minting corporate copies of them:
-- recording a call is the same authority whoever it was with, and a parallel
-- set would create roles that can log an individual call but not a corporate
-- one for no reason anybody could explain. Reaching a corporate subject
-- additionally requires vendor.crm.corporate.read, so the corporate wall is a
-- separate grant without duplicating the activity vocabulary.
--
-- The three sub-resource permissions exist because managing an organization's
-- sites and cost-centre taxonomy is a different job from managing the
-- relationship itself — a branch coordinator may legitimately maintain contacts
-- without being able to restructure the account.
INSERT INTO vendor_permissions(id,code,description,category,is_high_risk) VALUES
 ('perm_vendor_crm_corporate_read','vendor.crm.corporate.read','View corporate customer accounts, contacts, locations, cost centres and their activity.','crm',false),
 ('perm_vendor_crm_corporate_manage','vendor.crm.corporate.manage','Create and edit corporate customer accounts, their lifecycle, branch access and account manager.','crm',false),
 ('perm_vendor_crm_corporate_contacts_manage','vendor.crm.corporate.contacts.manage','Manage corporate contacts and their memberships within an organization.','crm',false),
 ('perm_vendor_crm_corporate_locations_manage','vendor.crm.corporate.locations.manage','Manage the corporate customer''s sites and locations.','crm',false),
 ('perm_vendor_crm_corporate_cost_centers_manage','vendor.crm.corporate.cost_centers.manage','Manage the corporate customer''s cost-centre labels.','crm',false)
ON CONFLICT(id) DO NOTHING;

-- Full corporate CRM authority.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM vendor_roles r CROSS JOIN vendor_permissions p
WHERE r.id IN ('role_owner','role_general_manager','role_crm_support')
  AND p.code IN ('vendor.crm.corporate.read','vendor.crm.corporate.manage',
                 'vendor.crm.corporate.contacts.manage','vendor.crm.corporate.locations.manage',
                 'vendor.crm.corporate.cost_centers.manage')
ON CONFLICT DO NOTHING;

-- Branch manager runs the corporate accounts their branch may work. The grant
-- does not widen reach: a branch-scoped assignment still cannot see a
-- BUSINESS-scope account or one scoped to another branch, because scope is
-- decided per request from the assignment, never from the permission code.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_branch_manager',id FROM vendor_permissions
WHERE code IN ('vendor.crm.corporate.read','vendor.crm.corporate.manage',
               'vendor.crm.corporate.contacts.manage','vendor.crm.corporate.locations.manage',
               'vendor.crm.corporate.cost_centers.manage')
ON CONFLICT DO NOTHING;

-- Marketing owns the commercial relationship conversation and may maintain
-- contacts, but restructuring an organization's sites and cost centres is
-- account administration it is not granted by default.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_marketing',id FROM vendor_permissions
WHERE code IN ('vendor.crm.corporate.read','vendor.crm.corporate.contacts.manage')
ON CONFLICT DO NOTHING;

-- Read-only visibility. Finance sees the account and its cost-centre labels
-- because those labels are how a future finance surface will group things, but
-- 8G gives it no relationship-management authority at all.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM vendor_roles r CROSS JOIN vendor_permissions p
WHERE r.id IN ('role_analyst_auditor','role_finance')
  AND p.code = 'vendor.crm.corporate.read'
ON CONFLICT DO NOTHING;

-- Deliberately unmapped: kitchen/pastry, shop/inventory, procurement and
-- vendor rider. None of them manages a commercial relationship with an
-- organization.
