DELETE FROM vendor_role_permissions WHERE permission_id IN (
 'perm_vendor_onboarding_read','perm_vendor_onboarding_manage','perm_vendor_verification_read',
 'perm_vendor_verification_submit','perm_vendor_bank_profile_read','perm_vendor_bank_profile_manage');
DELETE FROM vendor_permissions WHERE id IN (
 'perm_vendor_onboarding_read','perm_vendor_onboarding_manage','perm_vendor_verification_read',
 'perm_vendor_verification_submit','perm_vendor_bank_profile_read','perm_vendor_bank_profile_manage');
