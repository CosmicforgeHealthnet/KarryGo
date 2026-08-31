-- Reverses Section 17C slice 1.
--
-- Role grants are removed before the permissions they reference, because
-- vendor_role_permissions holds a foreign key onto vendor_permissions.

DELETE FROM vendor_role_permissions rp
USING vendor_permissions p
WHERE rp.permission_id = p.id
  AND p.code IN (
    'vendor.delivery.policy.read',
    'vendor.delivery.policy.manage'
  );

DELETE FROM vendor_permissions
WHERE code IN (
  'vendor.delivery.policy.read',
  'vendor.delivery.policy.manage'
);

DROP TABLE IF EXISTS vendor_delivery_policies;
