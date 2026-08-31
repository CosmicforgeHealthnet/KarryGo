DELETE FROM vendor_role_permissions
WHERE permission_id IN (SELECT id FROM vendor_permissions WHERE code IN
 ('vendor.food.pricing.manage','vendor.food.operations.manage'));
DELETE FROM vendor_permissions WHERE code IN
 ('vendor.food.pricing.manage','vendor.food.operations.manage');
