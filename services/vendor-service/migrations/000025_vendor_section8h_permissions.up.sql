-- Section 8H permissions.
--
-- Six, split along the two lines that actually matter.
--
-- Intelligence separates reading a score from ordering a rebuild, because a
-- rebuild is expensive scope-wide work rather than a per-record edit. There is
-- deliberately no permission to WRITE a score: intelligence is derived, and a
-- grant that let a staff member declare a customer high-value would make every
-- other score unfalsifiable.
--
-- Exports separate create, read, download and cancel because they are four
-- different disclosures. Reading that an export exists is not reading its
-- contents; the download grant is the one that actually releases customer data
-- and is therefore the one step-up protects most tightly.
INSERT INTO vendor_permissions(id,code,description,category,is_high_risk) VALUES
 ('perm_vendor_crm_intelligence_read','vendor.crm.intelligence.read','View CRM intelligence scores, distributions and availability.','crm',false),
 ('perm_vendor_crm_intelligence_rebuild','vendor.crm.intelligence.rebuild','Order a rebuild of CRM intelligence for a scope and model.','crm',true),
 ('perm_vendor_crm_exports_read','vendor.crm.exports.read','View CRM export jobs and their status.','crm',false),
 ('perm_vendor_crm_exports_create','vendor.crm.exports.create','Request a CRM data export.','crm',true),
 ('perm_vendor_crm_exports_download','vendor.crm.exports.download','Obtain short-lived access to a completed CRM export artifact.','crm',true),
 ('perm_vendor_crm_exports_cancel','vendor.crm.exports.cancel','Cancel a CRM export job and revoke its artifact.','crm',false)
ON CONFLICT(id) DO NOTHING;

-- Full intelligence and export authority.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM vendor_roles r CROSS JOIN vendor_permissions p
WHERE r.id IN ('role_owner','role_general_manager')
  AND p.code IN ('vendor.crm.intelligence.read','vendor.crm.intelligence.rebuild',
                 'vendor.crm.exports.read','vendor.crm.exports.create',
                 'vendor.crm.exports.download','vendor.crm.exports.cancel')
ON CONFLICT DO NOTHING;

-- Branch manager runs their own branch's book. The grant does not widen reach:
-- scope is decided per request from the assignment, so a branch-scoped manager
-- gets branch-scoped intelligence and branch-scoped exports, never the
-- business-wide figures those would otherwise reveal.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_branch_manager',id FROM vendor_permissions
WHERE code IN ('vendor.crm.intelligence.read','vendor.crm.exports.read',
               'vendor.crm.exports.create','vendor.crm.exports.download',
               'vendor.crm.exports.cancel')
ON CONFLICT DO NOTHING;

-- Marketing and CRM/Support read intelligence and may take approved exports.
-- Neither may order a rebuild: recomputing a whole scope is operational work
-- with a cost, and a marketing user reaching for a fresher number is not the
-- signal that should trigger it.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM vendor_roles r CROSS JOIN vendor_permissions p
WHERE r.id IN ('role_marketing','role_crm_support')
  AND p.code IN ('vendor.crm.intelligence.read','vendor.crm.exports.read',
                 'vendor.crm.exports.create','vendor.crm.exports.download')
ON CONFLICT DO NOTHING;

-- Analyst/Auditor reads intelligence and may take exports, which is the job.
-- It cannot cancel someone else's export or order a rebuild.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_analyst_auditor',id FROM vendor_permissions
WHERE code IN ('vendor.crm.intelligence.read','vendor.crm.exports.read',
               'vendor.crm.exports.create','vendor.crm.exports.download')
ON CONFLICT DO NOTHING;

-- Finance sees that exports exist, because an export is an event it may need
-- to account for, and nothing else. It receives no customer-level intelligence
-- and no ability to take CRM data out of the system.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_finance',id FROM vendor_permissions
WHERE code = 'vendor.crm.exports.read'
ON CONFLICT DO NOTHING;

-- Deliberately unmapped: kitchen/pastry, shop/inventory, procurement and
-- vendor rider. None of them works the customer book, and an export permission
-- is the widest customer-data grant this service issues.
