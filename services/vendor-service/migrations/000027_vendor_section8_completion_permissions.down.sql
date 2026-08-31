DELETE FROM vendor_role_permissions
WHERE permission_id IN (SELECT id FROM vendor_permissions WHERE code IN
 ('vendor.crm.customer_value.read','vendor.crm.loyalty.tiers.manage',
  'vendor.crm.followup_triggers.read'));
DELETE FROM vendor_permissions WHERE code IN
 ('vendor.crm.customer_value.read','vendor.crm.loyalty.tiers.manage',
  'vendor.crm.followup_triggers.read');
