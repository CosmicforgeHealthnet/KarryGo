-- Section 17C (slice 1): Vendor Own Delivery policy.
--
-- Scope:
--   * the vendor-configured delivery policy one branch offers on its own
--     riders/fleet, as an alternative to platform Dispatch/Hauling execution
--   * the minimum-order and free-delivery rules the vendor sets
--   * the vendor-authoritative delivery charge inputs
--   * the narrowing-only reach override on top of the branch service area
--
-- Explicitly NOT here:
--   * any platform logistics quote, mission, rider or provider selection
--   * the fulfilment plan itself (later 17C slices)
--   * money movement, settlement or allocation (17D/17F and the Wallet)
--
-- Authority notes worth stating once, because both are easy to get wrong
-- later:
--
--   1. SERVICE AREA. vendor_branch_service_areas (Section 6) stays the single
--      authority for how far a branch reaches. max_delivery_radius_meters
--      here may only NARROW that reach, never extend it; the planner takes
--      the minimum of the two. A vendor cannot grant itself a wider service
--      area by editing its delivery policy.
--
--   2. FEE BENEFICIARY. Nothing here configures who is paid. Per
--      docs/architecture/05_PAYMENT_ALLOCATION_MODEL.md the beneficiary is
--      DERIVED from the execution authority: vendor-owned delivery pays the
--      vendor, platform delivery pays the executing provider and never the
--      vendor. Storing it as a vendor-editable field would let a vendor claim
--      a platform provider delivery fee, so it is deliberately absent.
--
--   3. FREE DELIVERY IS NOT A FREE DELIVERY. A vendor free-delivery threshold
--      is a VENDOR-FUNDED promotion (same document, "Delivery promotion
--      funding"): the delivery still costs what it costs and the vendor
--      absorbs it. This table therefore configures only the threshold; the
--      gross/customer/vendor-funded split is recorded per plan, so
--      "who actually paid for this delivery" stays answerable.

CREATE TABLE vendor_delivery_policies (
    id text PRIMARY KEY,

    business_id text NOT NULL REFERENCES vendor_businesses (id),
    branch_id   text NOT NULL,

    -- The vendor own switch. VENDOR_OWN_DELIVERY_ENABLED is the platform
    -- switch; both must be true before a plan may choose this authority.
    enabled boolean NOT NULL DEFAULT false,

    currency char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),

    -- Charge inputs. Integer minor units, the platform money convention.
    base_fee_minor   bigint NOT NULL DEFAULT 0
        CHECK (base_fee_minor BETWEEN 0 AND 99999999999),
    per_km_fee_minor bigint NOT NULL DEFAULT 0
        CHECK (per_km_fee_minor BETWEEN 0 AND 99999999999),

    -- The order value below which this vendor will not deliver at all. It is
    -- a serviceability rule, not a surcharge: under it the group has no
    -- vendor-own option and must fall through to platform logistics or fail.
    minimum_order_minor bigint NOT NULL DEFAULT 0
        CHECK (minimum_order_minor BETWEEN 0 AND 99999999999),

    -- NULL means "this vendor never gives free delivery", which is different
    -- from a threshold of zero ("always free").
    free_delivery_threshold_minor bigint
        CHECK (free_delivery_threshold_minor IS NULL
               OR free_delivery_threshold_minor BETWEEN 0 AND 99999999999),

    -- Narrowing-only override on the branch service area. NULL means "use the
    -- branch service area unchanged".
    max_delivery_radius_meters integer
        CHECK (max_delivery_radius_meters IS NULL
               OR max_delivery_radius_meters BETWEEN 1 AND 200000),

    -- A real physical limit of the vendor own fleet. NULL means the vendor
    -- has not stated one, which is different from stating zero.
    max_package_weight_grams bigint
        CHECK (max_package_weight_grams IS NULL
               OR max_package_weight_grams BETWEEN 1 AND 100000000),

    -- Duration evidence, so a vendor-own plan can state an estimated delivery
    -- time from the vendor own operating knowledge rather than from a number
    -- Vendor Service invented.
    estimated_minutes_base   integer NOT NULL DEFAULT 0
        CHECK (estimated_minutes_base BETWEEN 0 AND 1440),
    estimated_minutes_per_km integer NOT NULL DEFAULT 0
        CHECK (estimated_minutes_per_km BETWEEN 0 AND 1440),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    version    bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    -- A free-delivery threshold below the minimum order can never be reached
    -- on its own terms, so it is a configuration mistake rather than a
    -- generous offer.
    CONSTRAINT vendor_delivery_policies_free_threshold_chk
        CHECK (
            free_delivery_threshold_minor IS NULL
            OR minimum_order_minor = 0
            OR free_delivery_threshold_minor >= minimum_order_minor
        ),

    CONSTRAINT vendor_delivery_policies_branch_scope_fk
        FOREIGN KEY (branch_id, business_id)
        REFERENCES vendor_branches (id, business_id) ON DELETE CASCADE,

    -- Tenant isolation enforced by row shape, not only by query predicates.
    UNIQUE (id, business_id),
    UNIQUE (business_id, branch_id)
);

CREATE INDEX vendor_delivery_policies_enabled_idx
    ON vendor_delivery_policies (business_id, enabled, branch_id);

COMMENT ON TABLE vendor_delivery_policies IS
'Section 17C Vendor Own Delivery policy, one per branch. Configures reach, charge and minimum/free rules only; it never configures who is paid - the delivery-fee beneficiary is derived from the execution authority per docs/architecture/05_PAYMENT_ALLOCATION_MODEL.md.';

COMMENT ON COLUMN vendor_delivery_policies.max_delivery_radius_meters IS
'Narrowing-only override of vendor_branch_service_areas.radius_meters. The planner takes the minimum of the two; a vendor can never widen its service area from here.';

COMMENT ON COLUMN vendor_delivery_policies.free_delivery_threshold_minor IS
'Vendor-funded delivery promotion threshold. The delivery still costs what it costs; the gross/customer/vendor-funded split is recorded per fulfilment plan so settlement attribution survives.';


-- Permission vocabulary for the vendor-own delivery policy.
--
-- manage is high risk: it directly changes what a customer is charged for
-- delivery and how far the vendor claims to reach. It is not, however, a
-- money-movement capability - nothing here settles, holds or transfers.
INSERT INTO vendor_permissions (id, code, description, category, is_high_risk) VALUES
 ('perm_delivery_policy_read',
  'vendor.delivery.policy.read',
  'Read the vendor-owned delivery policy configured for a branch.',
  'delivery',
  false),
 ('perm_delivery_policy_manage',
  'vendor.delivery.policy.manage',
  'Configure the vendor-owned delivery policy for a branch: reach, charge, minimum order and free-delivery threshold.',
  'delivery',
  true)
ON CONFLICT (id) DO NOTHING;

-- Owner and General Manager configure delivery for the whole business.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT role.id, p.id
FROM (VALUES ('role_owner'), ('role_general_manager')) role(id)
CROSS JOIN vendor_permissions p
WHERE p.code IN ('vendor.delivery.policy.read', 'vendor.delivery.policy.manage')
ON CONFLICT DO NOTHING;

-- Branch Manager configures its own branch. The branch bound comes from the
-- role assignment branch_id, exactly as every other branch-scoped capability
-- in this service - not from a separate permission code.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT 'role_branch_manager', id FROM vendor_permissions
WHERE code IN ('vendor.delivery.policy.read', 'vendor.delivery.policy.manage')
ON CONFLICT DO NOTHING;

-- Read-only roles that need to explain a delivery charge to a customer or
-- reconcile one, without being able to change it.
INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT role.id, p.id
FROM (VALUES ('role_crm_support'), ('role_finance'), ('role_analyst_auditor')) role(id)
CROSS JOIN vendor_permissions p
WHERE p.code = 'vendor.delivery.policy.read'
ON CONFLICT DO NOTHING;
