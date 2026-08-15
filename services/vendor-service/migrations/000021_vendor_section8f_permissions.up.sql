INSERT INTO vendor_permissions(id,code,description,category,is_high_risk) VALUES
 ('perm_vendor_crm_campaigns_execute','vendor.crm.campaigns.execute','Create and start campaign runs from prepared audiences.','crm',true),
 ('perm_vendor_crm_campaigns_runs_read','vendor.crm.campaigns.runs.read','View campaign runs and aggregate execution results.','crm',false),
 ('perm_vendor_crm_campaigns_runs_manage','vendor.crm.campaigns.runs.manage','Pause, resume, and cancel campaign runs.','crm',true),
 ('perm_vendor_crm_campaigns_deliveries_read','vendor.crm.campaigns.deliveries.read','View privacy-safe campaign delivery status and history.','crm',true)
ON CONFLICT(id) DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM vendor_roles r CROSS JOIN vendor_permissions p
WHERE r.id IN ('role_owner','role_general_manager','role_marketing')
  AND p.code IN ('vendor.crm.campaigns.execute','vendor.crm.campaigns.runs.read','vendor.crm.campaigns.runs.manage','vendor.crm.campaigns.deliveries.read')
ON CONFLICT DO NOTHING;
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_branch_manager',id FROM vendor_permissions
WHERE code IN ('vendor.crm.campaigns.execute','vendor.crm.campaigns.runs.read','vendor.crm.campaigns.runs.manage','vendor.crm.campaigns.deliveries.read')
ON CONFLICT DO NOTHING;
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM vendor_roles r CROSS JOIN vendor_permissions p
WHERE r.id IN ('role_crm_support','role_analyst_auditor')
  AND p.code IN ('vendor.crm.campaigns.runs.read','vendor.crm.campaigns.deliveries.read')
ON CONFLICT DO NOTHING;

