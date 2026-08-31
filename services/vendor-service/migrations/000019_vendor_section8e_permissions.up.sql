-- Section 8E permissions.
--
-- Five, because reading a campaign definition, editing it, evaluating who it
-- would reach, freezing that audience, and listing the actual customers in it
-- are genuinely different authorities. Listing targets in particular is a
-- strictly larger disclosure than reading the definition, exactly as segment
-- member reads are separated from segment reads in Section 8B.
--
-- There is deliberately no vendor.crm.campaigns.send. Section 8E cannot send,
-- and a permission for an authority the service does not hold would be an
-- invitation to grant it before the guard rails exist.
INSERT INTO vendor_permissions(id,code,description,category,is_high_risk) VALUES
 ('perm_vendor_crm_campaigns_read','vendor.crm.campaigns.read','View campaign definitions, versions and prepared audience summaries.','crm',false),
 ('perm_vendor_crm_campaigns_manage','vendor.crm.campaigns.manage','Create, edit and archive campaign definitions.','crm',false),
 ('perm_vendor_crm_campaigns_preview','vendor.crm.campaigns.preview','Evaluate who a campaign would currently reach, without freezing an audience.','crm',false),
 ('perm_vendor_crm_campaigns_prepare','vendor.crm.campaigns.prepare','Freeze an immutable campaign audience snapshot.','crm',false),
 ('perm_vendor_crm_campaigns_targets_read','vendor.crm.campaigns.targets.read','List the individual customer relationships inside a prepared audience snapshot.','crm',true)
ON CONFLICT(id) DO NOTHING;

-- Full campaign intelligence authority.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM vendor_roles r CROSS JOIN vendor_permissions p
WHERE r.id IN ('role_owner','role_general_manager','role_marketing')
  AND p.code IN ('vendor.crm.campaigns.read','vendor.crm.campaigns.manage',
                 'vendor.crm.campaigns.preview','vendor.crm.campaigns.prepare',
                 'vendor.crm.campaigns.targets.read')
ON CONFLICT DO NOTHING;

-- Branch manager runs campaigns for their own branch. The grant does not widen
-- reach: a branch-scoped assignment still cannot create or see a BUSINESS-scope
-- campaign, because scope is decided per request from the assignment, never
-- from the permission code.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_branch_manager',id FROM vendor_permissions
WHERE code IN ('vendor.crm.campaigns.read','vendor.crm.campaigns.manage',
               'vendor.crm.campaigns.preview','vendor.crm.campaigns.prepare',
               'vendor.crm.campaigns.targets.read')
ON CONFLICT DO NOTHING;

-- CRM/support owns the customer conversation and may plan and evaluate a
-- campaign, but exporting the customer-by-customer target list is a marketing
-- authority they are not granted by default.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_crm_support',id FROM vendor_permissions
WHERE code IN ('vendor.crm.campaigns.read','vendor.crm.campaigns.manage','vendor.crm.campaigns.preview')
ON CONFLICT DO NOTHING;

-- Analyst may read and evaluate; it may not change a definition or freeze one.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_analyst_auditor',id FROM vendor_permissions
WHERE code IN ('vendor.crm.campaigns.read','vendor.crm.campaigns.preview')
ON CONFLICT DO NOTHING;

-- Deliberately unmapped: finance (no default campaign write authority),
-- kitchen/pastry, shop/inventory, procurement and vendor rider. None of them
-- has a reason to define who the business markets to.
