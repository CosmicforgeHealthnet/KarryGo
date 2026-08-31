DELETE FROM vendor_role_permissions
WHERE permission_id IN (SELECT id FROM vendor_permissions WHERE code IN
 ('vendor.crm.campaigns.read','vendor.crm.campaigns.manage',
  'vendor.crm.campaigns.preview','vendor.crm.campaigns.prepare',
  'vendor.crm.campaigns.targets.read'));
DELETE FROM vendor_permissions WHERE code IN
 ('vendor.crm.campaigns.read','vendor.crm.campaigns.manage',
  'vendor.crm.campaigns.preview','vendor.crm.campaigns.prepare',
  'vendor.crm.campaigns.targets.read');
