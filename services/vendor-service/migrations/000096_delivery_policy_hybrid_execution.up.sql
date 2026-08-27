-- Delivery policy and smart-hybrid execution evidence.
-- This extends the existing Section 17C authorities; it does not create a
-- second pricing, checkout, Dispatch, or Hauling engine.

ALTER TABLE vendor_delivery_policies
    ADD COLUMN delivery_mode text NOT NULL DEFAULT 'VENDOR'
        CHECK (delivery_mode IN ('PLATFORM','VENDOR','HYBRID')),
    ADD COLUMN fee_rule text NOT NULL DEFAULT 'DISTANCE_TIER'
        CHECK (fee_rule IN ('FLAT','ZONE_BASED','DISTANCE_TIER')),
    ADD COLUMN operating_timezone text NOT NULL DEFAULT 'UTC',
    ADD COLUMN max_package_count bigint
        CHECK (max_package_count IS NULL OR max_package_count > 0),
    ADD COLUMN max_volume_ml bigint
        CHECK (max_volume_ml IS NULL OR max_volume_ml > 0),
    ADD COLUMN rider_availability text NOT NULL DEFAULT 'UNKNOWN'
        CHECK (rider_availability IN ('AVAILABLE','UNAVAILABLE','UNKNOWN')),
    ADD COLUMN service_zones jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN distance_tiers jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN operating_periods jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN supported_vehicle_classes jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN fallback_triggers jsonb NOT NULL DEFAULT '[]'::jsonb;

INSERT INTO vendor_permissions (id, code, description, category, is_high_risk) VALUES
 ('perm_delivery_operate', 'vendor.delivery.operate',
  'Operate vendor-owned delivery execution state for an authorised branch.',
  'delivery', false)
ON CONFLICT (id) DO NOTHING;

INSERT INTO vendor_role_permissions (role_id, permission_id)
SELECT role.id, p.id
FROM (VALUES ('role_owner'), ('role_general_manager'), ('role_branch_manager')) role(id)
CROSS JOIN vendor_permissions p
WHERE p.code = 'vendor.delivery.operate'
ON CONFLICT DO NOTHING;

ALTER TABLE commerce_fulfilment_plans
    ADD COLUMN delivery_policy_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN customer_delivery_price_guarantee_minor bigint NOT NULL DEFAULT 0
        CHECK (customer_delivery_price_guarantee_minor >= 0),
    ADD COLUMN delivery_price_guarantee_currency text NOT NULL DEFAULT '';

ALTER TABLE commerce_fulfilment_missions
    ADD COLUMN delivery_mode text NOT NULL DEFAULT 'PLATFORM'
        CHECK (delivery_mode IN ('PLATFORM','VENDOR','HYBRID')),
    ADD COLUMN fallback_trigger text NOT NULL DEFAULT '',
    ADD COLUMN policy_version bigint NOT NULL DEFAULT 0 CHECK (policy_version >= 0),
    ADD COLUMN customer_price_guarantee_minor bigint NOT NULL DEFAULT 0
        CHECK (customer_price_guarantee_minor >= 0),
    ADD COLUMN custody_state text NOT NULL DEFAULT 'NOT_TRANSFERRED'
        CHECK (custody_state IN ('NOT_TRANSFERRED','IN_VENDOR_CUSTODY','IN_PLATFORM_CUSTODY','RETURN_REQUIRED','RETURNED'));

ALTER TABLE commerce_fulfilment_plan_groups
    ADD COLUMN delivery_mode text NOT NULL DEFAULT 'PLATFORM'
        CHECK (delivery_mode IN ('PLATFORM','VENDOR','HYBRID')),
    ADD COLUMN fallback_trigger text NOT NULL DEFAULT '',
    ADD COLUMN customer_price_guarantee_minor bigint NOT NULL DEFAULT 0
        CHECK (customer_price_guarantee_minor >= 0);

CREATE TABLE commerce_delivery_execution (
    id text PRIMARY KEY,
    parent_order_id text NOT NULL
        REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    vendor_order_id text NOT NULL
        REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    parcel_ref text NOT NULL,
    owner text NOT NULL CHECK (owner IN ('VENDOR','PLATFORM','NONE')),
    state text NOT NULL CHECK (state IN ('PLANNED','ASSIGNED','READY_FOR_PICKUP','CUSTODY_TRANSFERRED','OUT_FOR_DELIVERY','DELIVERED','FAILED','RETURN_REQUIRED','RETURNED')),
    fallback_trigger text NOT NULL DEFAULT '',
    idempotency_key text NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (vendor_order_id),
    UNIQUE (parent_order_id, idempotency_key)
);

CREATE TABLE commerce_delivery_custody_events (
    id text PRIMARY KEY,
    execution_id text NOT NULL REFERENCES commerce_delivery_execution(id) ON DELETE RESTRICT,
    from_owner text NOT NULL,
    to_owner text NOT NULL,
    parcel_ref text NOT NULL,
    proof_ref text NOT NULL DEFAULT '',
    otp_or_qr_ref text NOT NULL DEFAULT '',
    reason_code text NOT NULL,
    actor_ref text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX commerce_delivery_execution_parent_idx
    ON commerce_delivery_execution(parent_order_id, updated_at DESC);
CREATE INDEX commerce_delivery_custody_events_execution_idx
    ON commerce_delivery_custody_events(execution_id, created_at);
