DELETE FROM vendor_role_permissions
WHERE permission_id IN (SELECT id FROM vendor_permissions WHERE code IN
 ('vendor.crm.interactions.read','vendor.crm.interactions.create','vendor.crm.interactions.manage',
  'vendor.crm.tasks.read','vendor.crm.tasks.create','vendor.crm.tasks.manage'));
DELETE FROM vendor_permissions WHERE code IN
 ('vendor.crm.interactions.read','vendor.crm.interactions.create','vendor.crm.interactions.manage',
  'vendor.crm.tasks.read','vendor.crm.tasks.create','vendor.crm.tasks.manage');
