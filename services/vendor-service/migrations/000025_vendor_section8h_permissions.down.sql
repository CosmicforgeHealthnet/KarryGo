DELETE FROM vendor_role_permissions
WHERE permission_id IN (SELECT id FROM vendor_permissions WHERE code IN
 ('vendor.crm.intelligence.read','vendor.crm.intelligence.rebuild',
  'vendor.crm.exports.read','vendor.crm.exports.create',
  'vendor.crm.exports.download','vendor.crm.exports.cancel'));
DELETE FROM vendor_permissions WHERE code IN
 ('vendor.crm.intelligence.read','vendor.crm.intelligence.rebuild',
  'vendor.crm.exports.read','vendor.crm.exports.create',
  'vendor.crm.exports.download','vendor.crm.exports.cancel');
