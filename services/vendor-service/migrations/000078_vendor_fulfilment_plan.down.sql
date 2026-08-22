-- Reverses Section 17C slice 3.
--
-- Dropped child-first: plan groups hold a composite foreign key onto missions,
-- and both hold one onto plans.

DELETE FROM vendor_role_permissions rp
USING vendor_permissions p
WHERE rp.permission_id = p.id
  AND p.code = 'vendor.fulfilment.plan.read';

DELETE FROM vendor_permissions
WHERE code = 'vendor.fulfilment.plan.read';

DROP TABLE IF EXISTS commerce_fulfilment_plan_events;
DROP TABLE IF EXISTS commerce_fulfilment_plan_groups;
DROP TABLE IF EXISTS commerce_fulfilment_missions;
DROP TABLE IF EXISTS commerce_fulfilment_plans;
