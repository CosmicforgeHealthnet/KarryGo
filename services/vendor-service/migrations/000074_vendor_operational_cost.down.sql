-- Reverse of 000074_vendor_operational_cost.
--
-- Triggers are dropped before their tables so the immutability guard cannot
-- refuse the rollback it is meant to protect against ordinary writes.

DROP TRIGGER IF EXISTS vendor_cost_snapshot_components_immutable_trg
    ON vendor_cost_snapshot_components;
DROP FUNCTION IF EXISTS vendor_cost_snapshot_component_immutable();

DROP TRIGGER IF EXISTS vendor_cost_snapshots_immutable_trg
    ON vendor_cost_snapshots;
DROP FUNCTION IF EXISTS vendor_cost_snapshot_immutable();

DELETE FROM vendor_role_permissions
 WHERE permission_id IN (
     SELECT id FROM vendor_permissions
      WHERE code IN (
          'vendor.cost.read',
          'vendor.cost.policy.manage',
          'vendor.production.cost.read'
      )
 );

DELETE FROM vendor_permissions
 WHERE code IN (
     'vendor.cost.read',
     'vendor.cost.policy.manage',
     'vendor.production.cost.read'
 );

DROP TABLE IF EXISTS vendor_cost_snapshot_components;
DROP TABLE IF EXISTS vendor_cost_snapshots;
DROP TABLE IF EXISTS vendor_cost_policies;
