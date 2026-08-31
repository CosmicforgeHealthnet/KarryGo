DELETE FROM vendor_role_permissions rp USING vendor_permissions p
WHERE rp.permission_id=p.id AND (p.code LIKE 'vendor.shop.inventory.%' OR p.code='vendor.shop.bulk_import.manage');
DELETE FROM vendor_permissions WHERE code LIKE 'vendor.shop.inventory.%' OR code='vendor.shop.bulk_import.manage';
