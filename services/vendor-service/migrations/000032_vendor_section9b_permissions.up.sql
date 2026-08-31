-- Section 9B permissions.
--
-- Two, and they exist to split an authority Section 9A had merged.
--
-- Under 9A a single `vendor.food.catalogue.manage` grant let a holder edit both
-- the menu and its prices. That was defensible while a menu was a static
-- document. It stops being defensible the moment the same grant is what a cook
-- needs to mark a dish sold out during service: nobody should have to hand out
-- business-wide pricing authority to let the kitchen do its job.
--
-- So 9B separates them:
--
--   vendor.food.pricing.manage     — change what something costs
--   vendor.food.operations.manage  — change when and whether it can be sold
--
-- Reads stay on the existing `vendor.food.catalogue.read`; there is no third
-- read permission, because a schedule is not a more sensitive fact than the
-- menu it belongs to.
INSERT INTO vendor_permissions(id,code,description,category,is_high_risk) VALUES
 ('perm_vendor_food_pricing_manage','vendor.food.pricing.manage','Change food catalogue prices: item base price, variant overrides, modifier deltas and branch price overrides.','food',false),
 ('perm_vendor_food_operations_manage','vendor.food.operations.manage','Manage food operations: menu schedules, sold-out and temporary availability, preparation, handling and availability-source configuration.','food',false)
ON CONFLICT(id) DO NOTHING;

-- Owner and General Manager hold both, business-wide.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM vendor_roles r CROSS JOIN vendor_permissions p
WHERE r.id IN ('role_owner','role_general_manager')
  AND p.code IN ('vendor.food.pricing.manage','vendor.food.operations.manage')
ON CONFLICT DO NOTHING;

-- Branch manager runs their branch's commercial and operational catalogue. The
-- grant does not widen reach: scope is resolved per request from the
-- assignment, so a branch-scoped manager prices and schedules only their own
-- branches. Business-level modifier definitions remain out of reach, exactly as
-- Section 9A decided.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_branch_manager',id FROM vendor_permissions
WHERE code IN ('vendor.food.pricing.manage','vendor.food.operations.manage')
ON CONFLICT DO NOTHING;

-- Kitchen/pastry gets operations and NOT pricing. This is the whole reason the
-- split exists: marking a dish sold out, pausing it, and stating how long it
-- takes to cook are the job; deciding what it costs is not.
INSERT INTO vendor_role_permissions(role_id,permission_id)
SELECT 'role_kitchen_pastry',id FROM vendor_permissions
WHERE code = 'vendor.food.operations.manage'
ON CONFLICT DO NOTHING;

-- Analyst/Auditor receives neither. It already holds
-- `vendor.food.catalogue.read`, which is the entire role.
--
-- Also deliberately unmapped: marketing and finance. A marketing role with
-- price authority is how an unreviewed discount reaches a live menu, and a
-- finance role does not set menu prices merely because prices are money —
-- Section 21.1 of the specification says so and the seed agrees.

-- Section 9A's `vendor.food.catalogue.manage` keeps every non-price editorial
-- power it already had (names, descriptions, sections, ordering, media,
-- dietary metadata, archival). Nothing is revoked here; price-bearing fields
-- now simply require the additional pricing grant on top of it, which the
-- service enforces per field rather than per route.
