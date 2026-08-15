DELETE FROM vendor_role_permissions rp
USING vendor_permissions p
WHERE rp.permission_id = p.id AND p.code IN (
 'vendor.shop.read','vendor.shop.catalogue.manage','vendor.shop.pricing.manage',
 'vendor.shop.wholesale.manage','vendor.shop.measurements.manage',
 'vendor.shop.listings.manage','vendor.shop.fresh.manage');

DELETE FROM vendor_permissions WHERE code IN (
 'vendor.shop.read','vendor.shop.catalogue.manage','vendor.shop.pricing.manage',
 'vendor.shop.wholesale.manage','vendor.shop.measurements.manage',
 'vendor.shop.listings.manage','vendor.shop.fresh.manage');
