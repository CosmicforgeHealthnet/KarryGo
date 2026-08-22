-- Reverses Section 13I's permission vocabulary. Role grants are removed before
-- the permissions they reference, because vendor_role_permissions holds a
-- foreign key onto vendor_permissions.

DELETE FROM vendor_role_permissions rp
USING vendor_permissions p
WHERE rp.permission_id = p.id
  AND p.code IN (
    'vendor.taxonomy.read',
    'vendor.taxonomy.mapping.manage',
    'vendor.taxonomy.platform.admin',
    'vendor.search.read',
    'vendor.search.ranking.admin',
    'vendor.search.projection.admin'
  );

DELETE FROM vendor_permissions
WHERE code IN (
  'vendor.taxonomy.read',
  'vendor.taxonomy.mapping.manage',
  'vendor.taxonomy.platform.admin',
  'vendor.search.read',
  'vendor.search.ranking.admin',
  'vendor.search.projection.admin'
);
