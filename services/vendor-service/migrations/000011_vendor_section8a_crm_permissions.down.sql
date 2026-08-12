DELETE FROM vendor_role_permissions WHERE permission_id IN (
 'perm_vendor_crm_customers_read','perm_vendor_crm_customers_manage',
 'perm_vendor_crm_customer360_read','perm_vendor_crm_consent_read','perm_vendor_crm_privacy_read');
DELETE FROM vendor_permissions WHERE id IN (
 'perm_vendor_crm_customers_read','perm_vendor_crm_customers_manage',
 'perm_vendor_crm_customer360_read','perm_vendor_crm_consent_read','perm_vendor_crm_privacy_read');
