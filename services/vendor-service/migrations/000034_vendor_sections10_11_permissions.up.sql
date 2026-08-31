-- Sections 10/11 production and domain permissions. Commercial quote and
-- pastry authorities are intentionally separate from kitchen operations.
INSERT INTO vendor_permissions (id, code, description, category, is_high_risk)
VALUES
 ('perm_vendor_production_read', 'vendor.production.read', 'Read production capacity, reservations and jobs.', 'production', false),
 ('perm_vendor_production_manage', 'vendor.production.manage', 'Manage production pools, lines, rules, slots and blackouts.', 'production', false),
 ('perm_production_operate', 'vendor.production.operate', 'Operate reservations, production jobs and milestones.', 'production', false),
 ('perm_cloud_kitchen_manage', 'vendor.cloud_kitchen.manage', 'Manage Cloud Kitchen packages and plans.', 'commerce', false),
 ('perm_corporate_plan_manage', 'vendor.corporate_plan.manage', 'Manage corporate production plans.', 'commerce', false),
 ('perm_pastry_read', 'vendor.pastry.read', 'Read pastry products and requests.', 'pastry', false),
 ('perm_pastry_manage', 'vendor.pastry.manage', 'Manage pastry products, options and requests.', 'pastry', false),
 ('perm_pastry_quote_manage', 'vendor.pastry.quote.manage', 'Create and revise pastry quotes.', 'pastry', false),
 ('perm_pastry_quote_approve', 'vendor.pastry.quote.approve', 'Approve high-value pastry quotes.', 'pastry', true),
 ('perm_pastry_production', 'vendor.pastry.production', 'Operate pastry production and milestones.', 'pastry', false)
ON CONFLICT (id) DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_owner', id FROM vendor_permissions WHERE code IN (
 'vendor.production.read','vendor.production.manage','vendor.production.operate',
 'vendor.cloud_kitchen.manage','vendor.corporate_plan.manage','vendor.pastry.read',
 'vendor.pastry.manage','vendor.pastry.quote.manage','vendor.pastry.quote.approve',
 'vendor.pastry.production') ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_general_manager', id FROM vendor_permissions WHERE code IN (
 'vendor.production.read','vendor.production.manage','vendor.production.operate',
 'vendor.cloud_kitchen.manage','vendor.corporate_plan.manage','vendor.pastry.read',
 'vendor.pastry.manage','vendor.pastry.quote.manage','vendor.pastry.production') ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_branch_manager', id FROM vendor_permissions WHERE code IN (
 'vendor.production.read','vendor.production.manage','vendor.production.operate',
 'vendor.cloud_kitchen.manage','vendor.pastry.read','vendor.pastry.manage',
 'vendor.pastry.production') ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_kitchen_pastry', id FROM vendor_permissions WHERE code IN (
 'vendor.production.read','vendor.production.operate','vendor.pastry.read',
 'vendor.pastry.production') ON CONFLICT DO NOTHING;

-- Section 5's generic kitchen role originally carried production.manage
-- before configuration and operation were separate authorities. Keep the
-- operational role least-privileged now that Sections 10/11 expose pools,
-- rules, slots and blackouts as real administrative resources.
DELETE FROM vendor_role_permissions rp
USING vendor_permissions p
WHERE rp.permission_id=p.id
  AND rp.role_id='role_kitchen_pastry'
  AND p.code='vendor.production.manage';

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_analyst_auditor', id FROM vendor_permissions WHERE code IN (
 'vendor.production.read','vendor.pastry.read') ON CONFLICT DO NOTHING;
