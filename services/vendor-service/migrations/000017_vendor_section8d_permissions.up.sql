-- Section 8D permissions.
--
-- Four, not one: reading a balance, configuring the programme, granting or
-- removing points by hand, and undoing a posted entry are genuinely different
-- authorities. Adjustment in particular is the abuse-sensitive one and is
-- granted to no role that does not already own the customer relationship.
INSERT INTO vendor_permissions(id,code,description,category,is_high_risk) VALUES
 ('perm_vendor_crm_loyalty_read','vendor.crm.loyalty.read','View the loyalty programme, customer point balances and points history.','crm',false),
 ('perm_vendor_crm_loyalty_manage','vendor.crm.loyalty.manage','Configure the loyalty programme, its status and its expiry policy.','crm',false),
 ('perm_vendor_crm_loyalty_adjust','vendor.crm.loyalty.adjust','Manually credit or debit customer loyalty points.','crm',true),
 ('perm_vendor_crm_loyalty_reverse','vendor.crm.loyalty.reverse','Reverse a posted loyalty ledger entry.','crm',true)
ON CONFLICT(id) DO NOTHING;

-- Full loyalty authority.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM vendor_roles r CROSS JOIN vendor_permissions p
WHERE r.id IN ('role_owner','role_general_manager')
  AND p.code IN ('vendor.crm.loyalty.read','vendor.crm.loyalty.manage',
                 'vendor.crm.loyalty.adjust','vendor.crm.loyalty.reverse')
ON CONFLICT DO NOTHING;

-- CRM/support owns the customer conversation, so it may correct a balance,
-- but it may not redefine the programme those points are worth.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_crm_support',id FROM vendor_permissions
WHERE code IN ('vendor.crm.loyalty.read','vendor.crm.loyalty.adjust','vendor.crm.loyalty.reverse')
ON CONFLICT DO NOTHING;

-- Read-only visibility. Branch manager reads remain branch-scoped at request
-- time; the permission grant does not widen Customer 360's scope rules.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM vendor_roles r CROSS JOIN vendor_permissions p
WHERE r.id IN ('role_marketing','role_branch_manager','role_analyst_auditor','role_finance')
  AND p.code = 'vendor.crm.loyalty.read'
ON CONFLICT DO NOTHING;

-- Deliberately unmapped: kitchen/pastry, shop/inventory, procurement and
-- vendor rider. Operations roles have no reason to see or move points.
