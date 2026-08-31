DELETE FROM vendor_role_permissions
WHERE permission_id IN (SELECT id FROM vendor_permissions WHERE code IN
 ('vendor.crm.loyalty.read','vendor.crm.loyalty.manage',
  'vendor.crm.loyalty.adjust','vendor.crm.loyalty.reverse'));
DELETE FROM vendor_permissions WHERE code IN
 ('vendor.crm.loyalty.read','vendor.crm.loyalty.manage',
  'vendor.crm.loyalty.adjust','vendor.crm.loyalty.reverse');
