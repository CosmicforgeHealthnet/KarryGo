DELETE FROM vendor_role_permissions
WHERE permission_id IN (SELECT id FROM vendor_permissions WHERE code IN
 ('vendor.crm.corporate.read','vendor.crm.corporate.manage',
  'vendor.crm.corporate.contacts.manage','vendor.crm.corporate.locations.manage',
  'vendor.crm.corporate.cost_centers.manage'));
DELETE FROM vendor_permissions WHERE code IN
 ('vendor.crm.corporate.read','vendor.crm.corporate.manage',
  'vendor.crm.corporate.contacts.manage','vendor.crm.corporate.locations.manage',
  'vendor.crm.corporate.cost_centers.manage');
