-- Section 8 completion permissions.
--
-- Three, not thirty. Every projection this completion adds is READ through a
-- surface that already has a permission: affinities, service recovery, value
-- and relationship signals all reach staff through Customer 360, which is
-- already gated by vendor.crm.customer360.read, and the automatic follow-up
-- tasks are ordinary CRM tasks under the Section 8C grants. Adding a permission
-- per projection would have grown the matrix without changing who can see what.
--
-- What genuinely needed its own grant:
--
--   * loyalty tier CONFIGURATION, because defining the thresholds that decide a
--     customer's public status is a different act from reading a tier, and it
--     is the only new mutation in the whole completion;
--   * customer VALUE, because a value band is a business-wide commercial
--     judgement that a branch-scoped CRM user should not automatically inherit
--     just because they can open a customer record;
--   * follow-up trigger visibility, so an operator can be shown why an
--     automatic task exists without that also being a licence to see every
--     commercial score behind it.
--
-- There is deliberately NO permission to write a value band, a relationship
-- signal, an affinity, a support count or an RFQ conversion. Those are derived
-- from authoritative facts; a grant that let a staff member type one in would
-- make every other one unfalsifiable.
INSERT INTO vendor_permissions(id,code,description,category,is_high_risk) VALUES
 ('perm_vendor_crm_customer_value_read','vendor.crm.customer_value.read','View derived customer value bands and relationship signals.','crm',false),
 ('perm_vendor_crm_loyalty_tiers_manage','vendor.crm.loyalty.tiers.manage','Define the loyalty tier thresholds for a business.','crm',true),
 ('perm_vendor_crm_followup_triggers_read','vendor.crm.followup_triggers.read','View why an automatic CRM follow-up task was created.','crm',false)
ON CONFLICT(id) DO NOTHING;

-- Owner and General Manager hold the full set, including tier configuration.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM vendor_roles r CROSS JOIN vendor_permissions p
WHERE r.id IN ('role_owner','role_general_manager')
  AND p.code IN ('vendor.crm.customer_value.read','vendor.crm.loyalty.tiers.manage',
                 'vendor.crm.followup_triggers.read')
ON CONFLICT DO NOTHING;

-- Branch manager reads value and trigger provenance for their own branch. The
-- grant does not widen reach: a business-wide value band is withheld from a
-- branch-scoped actor at read time regardless of this permission, because the
-- band summarises branches they cannot see.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_branch_manager',id FROM vendor_permissions
WHERE code IN ('vendor.crm.customer_value.read','vendor.crm.followup_triggers.read')
ON CONFLICT DO NOTHING;

-- CRM/Support works the follow-up queue, so it needs to see why a task exists
-- and how valuable the relationship is. It cannot redefine the tiers.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_crm_support',id FROM vendor_permissions
WHERE code IN ('vendor.crm.customer_value.read','vendor.crm.followup_triggers.read')
ON CONFLICT DO NOTHING;

-- Marketing reads value bands, which is the segmentation input it already
-- targets on. It has no business changing loyalty status thresholds.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_marketing',id FROM vendor_permissions
WHERE code IN ('vendor.crm.customer_value.read')
ON CONFLICT DO NOTHING;

-- Analyst/Auditor reads both derived signals and trigger provenance: auditing
-- why the system acted is exactly the job.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_analyst_auditor',id FROM vendor_permissions
WHERE code IN ('vendor.crm.customer_value.read','vendor.crm.followup_triggers.read')
ON CONFLICT DO NOTHING;
