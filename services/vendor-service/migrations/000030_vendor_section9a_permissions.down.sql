DELETE FROM vendor_role_permissions
WHERE permission_id IN (SELECT id FROM vendor_permissions WHERE code IN
 ('vendor.food.catalogue.read','vendor.food.catalogue.manage','vendor.food.catalogue.publish'));
DELETE FROM vendor_permissions WHERE code IN
 ('vendor.food.catalogue.read','vendor.food.catalogue.manage','vendor.food.catalogue.publish');
