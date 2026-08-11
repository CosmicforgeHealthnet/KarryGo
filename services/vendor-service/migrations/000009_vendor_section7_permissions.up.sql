INSERT INTO vendor_permissions(id,code,description,category,is_high_risk) VALUES
 ('perm_vendor_onboarding_read','vendor.onboarding.read','View onboarding progress, requirements and blockers.','onboarding',false),
 ('perm_vendor_onboarding_manage','vendor.onboarding.manage','Manage onboarding representatives, documents and submissions.','onboarding',false),
 ('perm_vendor_verification_read','vendor.verification.read','View authoritative verification projections.','onboarding',false),
 ('perm_vendor_verification_submit','vendor.verification.submit','Submit or remediate verification cases.','onboarding',false),
 ('perm_vendor_bank_profile_read','vendor.bank_profile.read','View the masked settlement-bank onboarding profile.','finance',false),
 ('perm_vendor_bank_profile_manage','vendor.bank_profile.manage','Replace the settlement-bank onboarding profile.','finance',true)
ON CONFLICT(id) DO NOTHING;

INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_owner',id FROM vendor_permissions WHERE code LIKE 'vendor.onboarding.%' OR code LIKE 'vendor.verification.%' OR code LIKE 'vendor.bank_profile.%'
ON CONFLICT DO NOTHING;
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_general_manager',id FROM vendor_permissions WHERE code LIKE 'vendor.onboarding.%' OR code LIKE 'vendor.verification.%' OR code='vendor.bank_profile.read'
ON CONFLICT DO NOTHING;
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_finance',id FROM vendor_permissions WHERE code IN ('vendor.onboarding.read','vendor.verification.read','vendor.bank_profile.read','vendor.bank_profile.manage')
ON CONFLICT DO NOTHING;
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_branch_manager',id FROM vendor_permissions WHERE code IN ('vendor.onboarding.read','vendor.verification.read')
ON CONFLICT DO NOTHING;
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_analyst_auditor',id FROM vendor_permissions WHERE code IN ('vendor.onboarding.read','vendor.verification.read','vendor.bank_profile.read')
ON CONFLICT DO NOTHING;
