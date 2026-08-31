DELETE FROM vendor_role_permissions
WHERE permission_id IN (
    SELECT id
      FROM vendor_permissions
     WHERE code IN (
        'vendor.recipes.read',
        'vendor.recipes.manage'
     )
);

DELETE FROM vendor_permissions
WHERE code IN (
    'vendor.recipes.read',
    'vendor.recipes.manage'
);

DROP TRIGGER IF EXISTS
    vendor_recipe_version_immutability_guard
ON vendor_recipe_versions;

DROP FUNCTION IF EXISTS
    vendor_recipe_version_immutability_guard();

DROP TRIGGER IF EXISTS
    vendor_recipe_packaging_mutability_guard
ON vendor_recipe_packaging_requirements;

DROP TRIGGER IF EXISTS
    vendor_recipe_components_mutability_guard
ON vendor_recipe_components;

DROP FUNCTION IF EXISTS
    vendor_recipe_definition_child_mutability_guard();

DROP TABLE IF EXISTS
    vendor_recipe_packaging_requirements;

DROP TABLE IF EXISTS
    vendor_recipe_components;

DROP TABLE IF EXISTS
    vendor_recipe_versions;

DROP TABLE IF EXISTS
    vendor_recipes;