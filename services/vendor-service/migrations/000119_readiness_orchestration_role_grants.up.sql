-- Grant the readiness and orchestration permissions to the roles that need them.
--
-- 000117 added the four permissions and granted them to nobody, so the
-- capability existed in the catalogue and was unreachable through any seeded
-- role -- including Owner, who by the platform's own rule holds every
-- vendor-scoped permission. A permission nobody holds is not a safe default:
-- it is a feature that silently 403s for everyone, which reads in production as
-- a bug in the feature rather than a gap in the seed.
--
-- The grants are split by what each role is actually for, not by seniority:
--
--   Owner and General Manager get everything, including the override, because
--   the override is a business decision about where an order goes.
--
--   Branch Manager gets read and manage but NOT the override. Declaring a
--   branch's own order ready is exactly a branch manager's job; choosing which
--   execution authority carries the order is not, and it affects vendors
--   outside their branch when the order is consolidated.
--
--   The kitchen, pastry and shop-floor roles get read and manage only. They are
--   the people actually standing at the counter when the food is done, and
--   requiring a manager to press "ready" is how the signal goes stale.
--
--   Analyst gets read. Explaining after the fact why an order went by truck is
--   the entire point of keeping the decision evidence.

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT role.id, p.id
FROM (VALUES ('role_owner'), ('role_general_manager')) role(id)
CROSS JOIN vendor_permissions p
WHERE p.code IN (
    'vendor.readiness.read',
    'vendor.readiness.manage',
    'vendor.orchestration.read',
    'vendor.orchestration.override'
)
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT role.id, p.id
FROM (VALUES
    ('role_branch_manager'),
    ('role_kitchen_pastry'),
    ('role_shop_inventory')
) role(id)
CROSS JOIN vendor_permissions p
WHERE p.code IN (
    'vendor.readiness.read',
    'vendor.readiness.manage',
    'vendor.orchestration.read'
)
ON CONFLICT DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_analyst_auditor', p.id
FROM vendor_permissions p
WHERE p.code IN ('vendor.readiness.read', 'vendor.orchestration.read')
ON CONFLICT DO NOTHING;
