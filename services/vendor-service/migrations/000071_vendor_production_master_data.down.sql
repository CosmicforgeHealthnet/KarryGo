DELETE FROM vendor_role_permissions
WHERE permission_id IN (
    SELECT id
      FROM vendor_permissions
     WHERE code IN (
        'vendor.ingredients.read',
        'vendor.ingredients.manage',
        'vendor.ingredients.inventory_mapping',
        'vendor.packaging.read',
        'vendor.packaging.manage',
        'vendor.packaging.inventory_mapping'
     )
);

DELETE FROM vendor_permissions
WHERE code IN (
    'vendor.ingredients.read',
    'vendor.ingredients.manage',
    'vendor.ingredients.inventory_mapping',
    'vendor.packaging.read',
    'vendor.packaging.manage',
    'vendor.packaging.inventory_mapping'
);

DROP TABLE IF EXISTS vendor_packaging_branch_configs;
DROP TABLE IF EXISTS vendor_packaging_unit_conversions;
DROP TABLE IF EXISTS vendor_packaging_materials;

DROP TABLE IF EXISTS vendor_ingredient_branch_configs;
DROP TABLE IF EXISTS vendor_ingredient_unit_conversions;
DROP TABLE IF EXISTS vendor_ingredients;