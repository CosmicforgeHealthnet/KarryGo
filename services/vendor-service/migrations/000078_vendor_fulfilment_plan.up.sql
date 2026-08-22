-- Section 17C: the candidate fulfilment plan.
--
-- Turns validated Section 17B vendor/branch groups into an authoritative
-- candidate plan: which execution authority serves each group, what each
-- delivery actually costs, and who funded it.
--
-- Explicitly NOT here:
--   * the final immutable quote and its approval/lock (17D)
--   * mission creation, provider assignment or any trip state (17F)
--   * payment, Wallet, escrow or any money movement
--
-- Four rules this schema enforces structurally, because each one is easy to
-- violate in code and expensive to discover later:
--
--   1. NO FAKE FEE. A group that is not serviceable may carry no fee, and a
--      PLATFORM_LOGISTICS mission may only carry a fee that came with an
--      authority quote reference. Vendor Service never prices a platform
--      delivery, so a platform mission with a fee and no quote id is a bug the
--      database refuses to store.
--
--   2. FUNDING ADDS UP. customer + vendor-funded + platform-funded always
--      equals the gross. "Who actually paid for this delivery" stays
--      answerable without re-deriving it from business rules afterwards.
--
--   3. NOTHING IS SILENTLY DROPPED. Every checkout group in the plan has a row
--      here, serviceable or not, and an unserviceable one must name a reason.
--
--   4. A MISSION NAMES ITS GROUPS. Consolidation is recorded as a real
--      relationship, so a consolidated delivery fee can always be traced back
--      to exactly the vendor groups it covers.

CREATE TABLE commerce_fulfilment_plans (
    id text PRIMARY KEY,

    checkout_session_id text NOT NULL
        REFERENCES commerce_checkout_sessions(id) ON DELETE CASCADE,

    customer_id text NOT NULL,

    -- The exact checkout session revision this plan was computed against, so a
    -- plan can never be silently applied to a re-validated checkout.
    checkout_validation_revision bigint NOT NULL
        CHECK (checkout_validation_revision > 0),

    status text NOT NULL DEFAULT 'PLANNING'
        CHECK (status IN (
            'PLANNING',
            'PLANNED',
            'UNSERVICEABLE',
            'EXPIRED',
            'SUPERSEDED',
            'FAILED'
        )),

    -- SINGLE is the honest answer while the optimizer produces one plan. The
    -- three optimizer modes are only ever stored when they actually differ;
    -- see docs — three identical options labelled differently would be a lie
    -- told three times.
    mode text NOT NULL DEFAULT 'SINGLE'
        CHECK (mode IN ('SINGLE', 'FASTEST', 'BALANCED', 'SAVER')),

    plan_revision bigint NOT NULL DEFAULT 1 CHECK (plan_revision > 0),

    currency text NOT NULL DEFAULT ''
        CHECK (currency = '' OR currency ~ '^[A-Z]{3}$'),

    -- Delivery money only. Merchandise totals stay on the checkout session.
    delivery_gross_minor          bigint NOT NULL DEFAULT 0 CHECK (delivery_gross_minor >= 0),
    delivery_customer_minor       bigint NOT NULL DEFAULT 0 CHECK (delivery_customer_minor >= 0),
    delivery_vendor_funded_minor  bigint NOT NULL DEFAULT 0 CHECK (delivery_vendor_funded_minor >= 0),
    delivery_platform_funded_minor bigint NOT NULL DEFAULT 0 CHECK (delivery_platform_funded_minor >= 0),

    failure_code text NOT NULL DEFAULT '',

    -- The plan expires no later than the earliest quote inside it. A plan that
    -- outlived its own quotes would be a fee nobody stands behind.
    expires_at timestamptz NOT NULL,

    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    planned_at timestamptz,

    CONSTRAINT commerce_fulfilment_plans_funding_chk
        CHECK (
            delivery_customer_minor
            + delivery_vendor_funded_minor
            + delivery_platform_funded_minor
            = delivery_gross_minor
        ),

    CONSTRAINT commerce_fulfilment_plans_planned_chk
        CHECK (
            status <> 'PLANNED'
            OR (planned_at IS NOT NULL AND currency <> '' AND failure_code = '')
        ),

    CONSTRAINT commerce_fulfilment_plans_failure_chk
        CHECK (
            status NOT IN ('UNSERVICEABLE', 'FAILED')
            OR failure_code <> ''
        ),

    -- Customer isolation enforced by row shape, not only by query predicates.
    UNIQUE (id, customer_id)
);

-- One live plan per checkout session. A second planning attempt must supersede
-- the first rather than race it.
CREATE UNIQUE INDEX commerce_fulfilment_plans_one_live_idx
    ON commerce_fulfilment_plans(checkout_session_id)
    WHERE status IN ('PLANNING', 'PLANNED');

CREATE INDEX commerce_fulfilment_plans_customer_idx
    ON commerce_fulfilment_plans(customer_id, created_at DESC, id);

CREATE INDEX commerce_fulfilment_plans_sweep_idx
    ON commerce_fulfilment_plans(expires_at, id)
    WHERE status IN ('PLANNING', 'PLANNED');


-- One quoted execution unit.
--
-- This is the COMMERCIAL unit: one authoritative quote, one fee. A later
-- section may split it into several physical execution missions without
-- destroying that commitment, which is why nothing here assumes one mission
-- equals one trip.
CREATE TABLE commerce_fulfilment_missions (
    id text PRIMARY KEY,

    plan_id text NOT NULL
        REFERENCES commerce_fulfilment_plans(id) ON DELETE CASCADE,

    customer_id text NOT NULL,

    authority text NOT NULL
        CHECK (authority IN ('PLATFORM_LOGISTICS', 'VENDOR_OWN_DELIVERY')),

    -- DISPATCH / HAULING for platform logistics; VENDOR_OWN_DELIVERY for the
    -- vendor's own fleet. TAXI is deliberately absent: Section 17C works
    -- without it and no taxi route is production-ready.
    service text NOT NULL
        CHECK (service IN ('DISPATCH', 'HAULING', 'VENDOR_OWN_DELIVERY')),

    vehicle_class text NOT NULL DEFAULT ''
        CHECK (vehicle_class IN ('', 'MOTORCYCLE', 'VAN', 'TRUCK')),

    -- The pickup this mission is quoted from. For a consolidated mission it is
    -- the shared pickup point the constituent branches actually share.
    pickup_business_id text NOT NULL,
    pickup_branch_id   text NOT NULL,
    pickup_latitude    double precision NOT NULL
        CHECK (pickup_latitude BETWEEN -90 AND 90),
    pickup_longitude   double precision NOT NULL
        CHECK (pickup_longitude BETWEEN -180 AND 180),

    dropoff_latitude  double precision NOT NULL
        CHECK (dropoff_latitude BETWEEN -90 AND 90),
    dropoff_longitude double precision NOT NULL
        CHECK (dropoff_longitude BETWEEN -180 AND 180),

    consolidated boolean NOT NULL DEFAULT false,

    -- Authority quote evidence. Every one of these comes from the logistics
    -- service's own answer; Vendor Service invents none of them.
    authority_quote_id        text NOT NULL DEFAULT '',
    authority_quote_reference text NOT NULL DEFAULT '',
    authority_quote_revision  bigint NOT NULL DEFAULT 0
        CHECK (authority_quote_revision >= 0),
    authority_quote_expires_at timestamptz,
    authority_pickup_ref      text NOT NULL DEFAULT '',
    authority_dropoff_ref     text NOT NULL DEFAULT '',

    distance_km       double precision NOT NULL DEFAULT 0 CHECK (distance_km >= 0),
    estimated_minutes integer NOT NULL DEFAULT 0 CHECK (estimated_minutes >= 0),

    currency text NOT NULL DEFAULT ''
        CHECK (currency = '' OR currency ~ '^[A-Z]{3}$'),

    gross_fee_minor           bigint NOT NULL DEFAULT 0 CHECK (gross_fee_minor >= 0),
    customer_fee_minor        bigint NOT NULL DEFAULT 0 CHECK (customer_fee_minor >= 0),
    vendor_funded_minor       bigint NOT NULL DEFAULT 0 CHECK (vendor_funded_minor >= 0),
    platform_funded_minor     bigint NOT NULL DEFAULT 0 CHECK (platform_funded_minor >= 0),

    base_minor      bigint NOT NULL DEFAULT 0 CHECK (base_minor >= 0),
    distance_minor  bigint NOT NULL DEFAULT 0 CHECK (distance_minor >= 0),
    surcharge_minor bigint NOT NULL DEFAULT 0 CHECK (surcharge_minor >= 0),

    -- Derived from the authority, never configured: vendor-owned delivery pays
    -- the vendor, platform delivery pays the executing provider.
    fee_beneficiary text NOT NULL
        CHECK (fee_beneficiary IN ('VENDOR', 'PLATFORM_PROVIDER')),

    total_weight_grams bigint NOT NULL DEFAULT 0 CHECK (total_weight_grams >= 0),
    package_count      bigint NOT NULL DEFAULT 0 CHECK (package_count >= 0),

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT commerce_fulfilment_missions_funding_chk
        CHECK (
            customer_fee_minor + vendor_funded_minor + platform_funded_minor
            = gross_fee_minor
        ),

    -- Rule 1, stated where it cannot be bypassed. A platform mission charging
    -- money must name the authority quote that priced it.
    CONSTRAINT commerce_fulfilment_missions_platform_quote_chk
        CHECK (
            authority <> 'PLATFORM_LOGISTICS'
            OR (
                length(btrim(authority_quote_id)) > 0
                AND authority_quote_expires_at IS NOT NULL
                AND vehicle_class <> ''
                AND currency <> ''
            )
        ),

    -- The converse: vendor-owned delivery is priced by the vendor's own
    -- policy and must NOT claim a platform authority quote.
    CONSTRAINT commerce_fulfilment_missions_vendor_own_chk
        CHECK (
            authority <> 'VENDOR_OWN_DELIVERY'
            OR (
                service = 'VENDOR_OWN_DELIVERY'
                AND authority_quote_id = ''
                AND fee_beneficiary = 'VENDOR'
            )
        ),

    CONSTRAINT commerce_fulfilment_missions_platform_beneficiary_chk
        CHECK (
            authority <> 'PLATFORM_LOGISTICS'
            OR fee_beneficiary = 'PLATFORM_PROVIDER'
        ),

    UNIQUE (id, plan_id),
    UNIQUE (id, customer_id)
);

CREATE INDEX commerce_fulfilment_missions_plan_idx
    ON commerce_fulfilment_missions(plan_id, id);


-- One vendor/branch group of the plan, with its own serviceability outcome.
--
-- Every Section 17B group gets a row, serviceable or not. That is what makes
-- "do not silently drop the group" structural rather than a code convention.
CREATE TABLE commerce_fulfilment_plan_groups (
    id text PRIMARY KEY,

    plan_id text NOT NULL
        REFERENCES commerce_fulfilment_plans(id) ON DELETE CASCADE,

    customer_id text NOT NULL,

    checkout_group_id text NOT NULL,

    business_id text NOT NULL,
    branch_id   text NOT NULL,

    serviceable boolean NOT NULL DEFAULT false,

    -- SERVICEABLE, or the explicit reason this group cannot be delivered.
    reason_code text NOT NULL,

    authority text NOT NULL DEFAULT 'NONE'
        CHECK (authority IN ('NONE', 'PLATFORM_LOGISTICS', 'VENDOR_OWN_DELIVERY')),

    -- The quoted mission serving this group. NULL for an unserviceable group.
    mission_id text,

    -- Aggregated logistics evidence, kept so a plan can be explained without
    -- re-reading a catalogue that may since have changed.
    vehicle_class text NOT NULL DEFAULT ''
        CHECK (vehicle_class IN ('', 'MOTORCYCLE', 'VAN', 'TRUCK')),

    package_count      bigint NOT NULL DEFAULT 0 CHECK (package_count >= 0),
    total_weight_grams bigint NOT NULL DEFAULT 0 CHECK (total_weight_grams >= 0),
    total_volume_ml    bigint NOT NULL DEFAULT 0 CHECK (total_volume_ml >= 0),
    longest_dimension_mm integer NOT NULL DEFAULT 0 CHECK (longest_dimension_mm >= 0),

    weight_evidence text NOT NULL DEFAULT 'ABSENT'
        CHECK (weight_evidence IN ('COMPLETE', 'PARTIAL', 'ABSENT')),

    urgency text NOT NULL DEFAULT 'STANDARD'
        CHECK (urgency IN ('STANDARD', 'IMMEDIATE')),

    capabilities text[] NOT NULL DEFAULT ARRAY[]::text[],

    -- This group's share of its mission's fee. For a single-group mission it
    -- is the whole fee; for a consolidated one it is this group's attributed
    -- share, and the shares always sum back to the mission's own totals.
    gross_fee_minor       bigint NOT NULL DEFAULT 0 CHECK (gross_fee_minor >= 0),
    customer_fee_minor    bigint NOT NULL DEFAULT 0 CHECK (customer_fee_minor >= 0),
    vendor_funded_minor   bigint NOT NULL DEFAULT 0 CHECK (vendor_funded_minor >= 0),
    platform_funded_minor bigint NOT NULL DEFAULT 0 CHECK (platform_funded_minor >= 0),

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT commerce_fulfilment_plan_groups_reason_chk
        CHECK (length(btrim(reason_code)) > 0),

    CONSTRAINT commerce_fulfilment_plan_groups_funding_chk
        CHECK (
            customer_fee_minor + vendor_funded_minor + platform_funded_minor
            = gross_fee_minor
        ),

    -- Rule 1 again, at the group level: a group nobody will deliver carries no
    -- charge and names no mission.
    CONSTRAINT commerce_fulfilment_plan_groups_refusal_chk
        CHECK (
            serviceable
            OR (
                gross_fee_minor = 0
                AND mission_id IS NULL
                AND authority = 'NONE'
                AND reason_code <> 'SERVICEABLE'
            )
        ),

    CONSTRAINT commerce_fulfilment_plan_groups_serviceable_chk
        CHECK (
            NOT serviceable
            OR (mission_id IS NOT NULL AND authority <> 'NONE')
        ),

    -- The mission must belong to the same plan. Without the composite key a
    -- group could name a mission from someone else's plan.
    CONSTRAINT commerce_fulfilment_plan_groups_mission_scope_fk
        FOREIGN KEY (mission_id, plan_id)
        REFERENCES commerce_fulfilment_missions(id, plan_id) ON DELETE CASCADE,

    UNIQUE (plan_id, checkout_group_id)
);

CREATE INDEX commerce_fulfilment_plan_groups_plan_idx
    ON commerce_fulfilment_plan_groups(plan_id, id);

CREATE INDEX commerce_fulfilment_plan_groups_mission_idx
    ON commerce_fulfilment_plan_groups(mission_id, id)
    WHERE mission_id IS NOT NULL;


-- Append-only planning evidence.
CREATE TABLE commerce_fulfilment_plan_events (
    id text PRIMARY KEY,

    plan_id text NOT NULL
        REFERENCES commerce_fulfilment_plans(id) ON DELETE CASCADE,

    customer_id text NOT NULL,

    action text NOT NULL
        CHECK (action IN (
            'commerce.fulfilment.planning_started',
            'commerce.fulfilment.group_requirement_resolved',
            'commerce.fulfilment.vendor_own_selected',
            'commerce.fulfilment.quote_requested',
            'commerce.fulfilment.quote_received',
            'commerce.fulfilment.quote_refused',
            'commerce.fulfilment.quote_unavailable',
            'commerce.fulfilment.group_unserviceable',
            'commerce.fulfilment.consolidated',
            'commerce.fulfilment.planned',
            'commerce.fulfilment.failed',
            'commerce.fulfilment.expired',
            'commerce.fulfilment.superseded'
        )),

    group_id   text,
    mission_id text,

    detail text NOT NULL DEFAULT '',

    plan_version bigint NOT NULL CHECK (plan_version > 0),

    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX commerce_fulfilment_plan_events_plan_idx
    ON commerce_fulfilment_plan_events(plan_id, occurred_at, id);


COMMENT ON TABLE commerce_fulfilment_plans IS
'Section 17C candidate fulfilment plan. It is planning evidence, not a quote, not an authorization and not an order. 17D locks the customer-protected total.';

COMMENT ON COLUMN commerce_fulfilment_missions.authority_quote_id IS
'The logistics authority own quote identity. A PLATFORM_LOGISTICS mission may not carry a fee without one - Vendor Service never prices a platform delivery.';

COMMENT ON COLUMN commerce_fulfilment_missions.consolidated IS
'True when this quoted mission covers more than one vendor group. A later section may split it into several execution missions without changing the commercial commitment recorded here.';

COMMENT ON TABLE commerce_fulfilment_plan_groups IS
'One Section 17B checkout group in the plan. Every group gets a row whether or not it can be delivered, so an unserviceable group is reported with a reason rather than silently dropped.';


-- Permission to read a fulfilment plan from the vendor side. Planning itself
-- is customer-driven and has no staff permission; this is for a vendor
-- explaining a delivery charge to a customer, or reconciling one.
INSERT INTO vendor_permissions (id, code, description, category, is_high_risk) VALUES
 ('perm_fulfilment_plan_read',
  'vendor.fulfilment.plan.read',
  'Read fulfilment plans and delivery quotes covering the vendor own orders.',
  'fulfilment',
  false)
ON CONFLICT (id) DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT role.id, p.id
FROM (VALUES
    ('role_owner'), ('role_general_manager'), ('role_branch_manager'),
    ('role_crm_support'), ('role_finance'), ('role_analyst_auditor')
) role(id)
CROSS JOIN vendor_permissions p
WHERE p.code = 'vendor.fulfilment.plan.read'
ON CONFLICT DO NOTHING;
