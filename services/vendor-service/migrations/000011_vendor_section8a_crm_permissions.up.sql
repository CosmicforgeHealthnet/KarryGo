-- Section 8A CRM permissions.
--
-- Read-oriented by design. There is deliberately no consent-override,
-- privacy-override or force-merge permission: those decisions belong to the
-- external identity/privacy authority and are not delegable to Vendor staff,
-- so no role — including Owner — can be granted them.
INSERT INTO vendor_permissions(id,code,description,category,is_high_risk) VALUES
 ('perm_vendor_crm_customers_read','vendor.crm.customers.read','View vendor-local customer relationship profiles.','crm',false),
 ('perm_vendor_crm_customers_manage','vendor.crm.customers.manage','Archive or restore vendor-local customer relationship profiles.','crm',false),
 ('perm_vendor_crm_customer360_read','vendor.crm.customer360.read','View the Customer 360 relationship summary.','crm',false),
 ('perm_vendor_crm_consent_read','vendor.crm.consent.read','View projected marketing consent and eligibility.','crm',false),
 ('perm_vendor_crm_privacy_read','vendor.crm.privacy.read','View projected customer privacy state.','crm',false)
ON CONFLICT(id) DO NOTHING;

-- Owner / Account controller: full Section 8A read surface plus lifecycle.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_owner',id FROM vendor_permissions WHERE code LIKE 'vendor.crm.%'
ON CONFLICT DO NOTHING;

-- General manager: same operational surface as Owner for 8A.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_general_manager',id FROM vendor_permissions WHERE code LIKE 'vendor.crm.%'
ON CONFLICT DO NOTHING;

-- CRM / Support: the primary day-to-day CRM role.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_crm_support',id FROM vendor_permissions WHERE code LIKE 'vendor.crm.%'
ON CONFLICT DO NOTHING;

-- Marketing: safe read plus consent eligibility. No lifecycle management, no
-- privacy read, and (structurally) no access to canonical customer contact
-- data, which CRM never stores or returns.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_marketing',id FROM vendor_permissions
WHERE code IN ('vendor.crm.customers.read','vendor.crm.customer360.read','vendor.crm.consent.read')
ON CONFLICT DO NOTHING;

-- Branch manager: read only. The grant is branch-scoped at assignment time by
-- Section 5, and every CRM query joins the branch relation table, so this
-- never yields business-wide commercial totals.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_branch_manager',id FROM vendor_permissions
WHERE code IN ('vendor.crm.customers.read','vendor.crm.customer360.read')
ON CONFLICT DO NOTHING;

-- Analyst / Auditor: read-only, including privacy state for compliance review.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_analyst_auditor',id FROM vendor_permissions
WHERE code IN ('vendor.crm.customers.read','vendor.crm.customer360.read','vendor.crm.consent.read','vendor.crm.privacy.read')
ON CONFLICT DO NOTHING;

-- Deliberately unmapped: role_finance, role_kitchen_pastry, role_shop_inventory,
-- role_procurement, role_vendor_rider. None has a Section 8A relationship need.
