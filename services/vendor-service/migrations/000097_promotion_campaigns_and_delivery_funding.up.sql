-- Authoritative commerce promotions and immutable delivery-funding evidence.
-- CRM messaging campaigns remain independent; CheckoutQuote remains the
-- commercial calculator and Wallet remains the monetary authority.

CREATE TABLE commerce_promotion_campaigns (
    id text PRIMARY KEY,
    business_id text NOT NULL REFERENCES vendor_businesses(id) ON DELETE RESTRICT,
    name text NOT NULL,
    promotion_type text NOT NULL CHECK (promotion_type IN (
        'PERCENTAGE_DISCOUNT','FIXED_DISCOUNT','COUPON','BUNDLE','QUANTITY_BREAK',
        'FREE_DELIVERY','DELIVERY_PERCENTAGE_DISCOUNT','DELIVERY_FLAT_DISCOUNT')),
    status text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','SCHEDULED','ACTIVE','PAUSED','ENDED','ARCHIVED')),
    currency text NOT NULL,
    percentage_bps integer NOT NULL DEFAULT 0 CHECK (percentage_bps BETWEEN 0 AND 10000),
    fixed_amount_minor bigint NOT NULL DEFAULT 0 CHECK (fixed_amount_minor >= 0),
    max_discount_minor bigint CHECK (max_discount_minor IS NULL OR max_discount_minor > 0),
    minimum_order_minor bigint NOT NULL DEFAULT 0 CHECK (minimum_order_minor >= 0),
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    funding_sponsor text NOT NULL CHECK (funding_sponsor IN ('VENDOR','PLATFORM','COMBINED')),
    vendor_funding_bps integer NOT NULL DEFAULT 0 CHECK (vendor_funding_bps BETWEEN 0 AND 10000),
    platform_approval_ref text NOT NULL DEFAULT '',
    coupon_required boolean NOT NULL DEFAULT false,
    global_usage_limit bigint CHECK (global_usage_limit IS NULL OR global_usage_limit > 0),
    per_customer_usage_limit bigint CHECK (per_customer_usage_limit IS NULL OR per_customer_usage_limit > 0),
    combinability text NOT NULL DEFAULT 'EXCLUSIVE' CHECK (combinability IN ('EXCLUSIVE','STACKABLE')),
    priority integer NOT NULL DEFAULT 100,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by_staff_ref text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    CHECK (funding_sponsor = 'VENDOR' OR platform_approval_ref <> ''),
    UNIQUE (id, business_id)
);

ALTER TABLE vendor_delivery_policies
    ADD COLUMN hybrid_fallback_funding_sponsor text NOT NULL DEFAULT 'VENDOR'
        CHECK (hybrid_fallback_funding_sponsor IN ('VENDOR','PLATFORM','COMBINED')),
    ADD COLUMN hybrid_vendor_share_bps integer NOT NULL DEFAULT 10000
        CHECK (hybrid_vendor_share_bps BETWEEN 0 AND 10000),
    ADD COLUMN hybrid_platform_approval_ref text NOT NULL DEFAULT '',
    ADD CONSTRAINT vendor_delivery_hybrid_funding_authority_ck CHECK (
        (hybrid_fallback_funding_sponsor = 'VENDOR' AND hybrid_vendor_share_bps = 10000 AND hybrid_platform_approval_ref = '') OR
        (hybrid_fallback_funding_sponsor = 'PLATFORM' AND hybrid_vendor_share_bps = 0 AND hybrid_platform_approval_ref <> '') OR
        (hybrid_fallback_funding_sponsor = 'COMBINED' AND hybrid_vendor_share_bps BETWEEN 1 AND 9999 AND hybrid_platform_approval_ref <> '')
    );

CREATE TABLE commerce_promotion_scopes (
    id text PRIMARY KEY,
    campaign_id text NOT NULL,
    business_id text NOT NULL,
    scope_type text NOT NULL CHECK (scope_type IN ('BRANCH','CAPABILITY','CATEGORY','ITEM','VARIANT','CUSTOMER_SEGMENT')),
    scope_ref text NOT NULL,
    FOREIGN KEY (campaign_id, business_id) REFERENCES commerce_promotion_campaigns(id, business_id) ON DELETE CASCADE,
    UNIQUE (campaign_id, scope_type, scope_ref)
);

CREATE TABLE commerce_promotion_bundle_requirements (
    id text PRIMARY KEY,
    campaign_id text NOT NULL,
    business_id text NOT NULL,
    item_ref text NOT NULL,
    variant_ref text NOT NULL DEFAULT '',
    minimum_quantity bigint NOT NULL CHECK (minimum_quantity > 0),
    FOREIGN KEY (campaign_id, business_id) REFERENCES commerce_promotion_campaigns(id, business_id) ON DELETE CASCADE,
    UNIQUE (campaign_id, item_ref, variant_ref)
);

CREATE TABLE commerce_promotion_quantity_tiers (
    id text PRIMARY KEY,
    campaign_id text NOT NULL,
    business_id text NOT NULL,
    minimum_quantity bigint NOT NULL CHECK (minimum_quantity > 0),
    maximum_quantity bigint,
    percentage_bps integer NOT NULL DEFAULT 0 CHECK (percentage_bps BETWEEN 0 AND 10000),
    fixed_amount_minor bigint NOT NULL DEFAULT 0 CHECK (fixed_amount_minor >= 0),
    FOREIGN KEY (campaign_id, business_id) REFERENCES commerce_promotion_campaigns(id, business_id) ON DELETE CASCADE,
    CHECK (maximum_quantity IS NULL OR maximum_quantity >= minimum_quantity),
    CHECK ((percentage_bps > 0) <> (fixed_amount_minor > 0)),
    UNIQUE (campaign_id, minimum_quantity)
);

CREATE TABLE commerce_promotion_coupons (
    id text PRIMARY KEY,
    campaign_id text NOT NULL,
    business_id text NOT NULL,
    code_digest text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    revoked_at timestamptz,
    usage_limit bigint CHECK (usage_limit IS NULL OR usage_limit > 0),
    per_customer_usage_limit bigint CHECK (per_customer_usage_limit IS NULL OR per_customer_usage_limit > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (campaign_id, business_id) REFERENCES commerce_promotion_campaigns(id, business_id) ON DELETE CASCADE,
    UNIQUE (business_id, code_digest),
    UNIQUE (id, business_id)
);

CREATE TABLE commerce_promotion_redemptions (
    id text PRIMARY KEY,
    campaign_id text NOT NULL,
    coupon_id text,
    business_id text NOT NULL,
    customer_id text NOT NULL,
    quote_id text NOT NULL REFERENCES commerce_quotes(id) ON DELETE RESTRICT,
    order_id text,
    idempotency_key text NOT NULL,
    status text NOT NULL DEFAULT 'RESERVED' CHECK (status IN ('RESERVED','CONSUMED','RELEASED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (campaign_id, business_id) REFERENCES commerce_promotion_campaigns(id, business_id) ON DELETE RESTRICT,
    FOREIGN KEY (coupon_id, business_id) REFERENCES commerce_promotion_coupons(id, business_id) ON DELETE RESTRICT,
    UNIQUE (campaign_id, quote_id),
    UNIQUE (business_id, idempotency_key)
);

ALTER TABLE commerce_quote_adjustments
    ADD COLUMN business_id text NOT NULL DEFAULT '',
    ADD COLUMN branch_id text NOT NULL DEFAULT '',
    ADD COLUMN promotion_type text NOT NULL DEFAULT '',
    ADD COLUMN campaign_version bigint NOT NULL DEFAULT 0,
    ADD COLUMN eligibility_evidence jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN coupon_digest text NOT NULL DEFAULT '',
    ADD COLUMN platform_approval_ref text NOT NULL DEFAULT '',
    ADD COLUMN calculation_base_minor bigint NOT NULL DEFAULT 0,
    ADD COLUMN delivery_discount_minor bigint NOT NULL DEFAULT 0,
    ADD COLUMN stack_priority integer NOT NULL DEFAULT 0,
    ADD COLUMN applied_at timestamptz;

ALTER TABLE commerce_parent_orders
    ADD COLUMN promotion_snapshot jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN delivery_funding_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE commerce_vendor_orders
    ADD COLUMN promotion_snapshot jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN delivery_funding_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE commerce_delivery_funding_operations (
    id text PRIMARY KEY,
    parent_order_id text NOT NULL REFERENCES commerce_parent_orders(id) ON DELETE RESTRICT,
    vendor_order_id text NOT NULL REFERENCES commerce_vendor_orders(id) ON DELETE RESTRICT,
    business_id text NOT NULL,
    currency text NOT NULL,
    authoritative_delivery_minor bigint NOT NULL CHECK (authoritative_delivery_minor >= 0),
    customer_contribution_minor bigint NOT NULL CHECK (customer_contribution_minor >= 0),
    vendor_subsidy_minor bigint NOT NULL CHECK (vendor_subsidy_minor >= 0),
    platform_subsidy_minor bigint NOT NULL CHECK (platform_subsidy_minor >= 0),
    wallet_operation_ref text NOT NULL DEFAULT '',
    wallet_status text NOT NULL DEFAULT 'PENDING' CHECK (wallet_status IN ('PENDING','APPLIED','UNKNOWN','REVERSED','FAILED')),
    idempotency_key text NOT NULL,
    source_snapshot jsonb NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (customer_contribution_minor + vendor_subsidy_minor + platform_subsidy_minor = authoritative_delivery_minor),
    UNIQUE (vendor_order_id),
    UNIQUE (parent_order_id, idempotency_key)
);

CREATE INDEX commerce_promotion_active_lookup_idx
    ON commerce_promotion_campaigns(business_id, status, starts_at, ends_at, priority, id);
CREATE INDEX commerce_promotion_scope_lookup_idx
    ON commerce_promotion_scopes(business_id, scope_type, scope_ref, campaign_id);
CREATE INDEX commerce_promotion_coupon_lookup_idx
    ON commerce_promotion_coupons(business_id, code_digest) WHERE enabled AND revoked_at IS NULL;
CREATE INDEX commerce_promotion_redemption_campaign_idx
    ON commerce_promotion_redemptions(campaign_id, status, customer_id);
CREATE INDEX commerce_delivery_funding_order_idx
    ON commerce_delivery_funding_operations(parent_order_id, vendor_order_id);

INSERT INTO vendor_permissions(id, code, description, category, is_high_risk) VALUES
 ('perm_promotions_read', 'vendor.promotions.read', 'Read promotion campaigns and usage.', 'commerce', false),
 ('perm_promotions_manage', 'vendor.promotions.manage', 'Manage Vendor-funded promotion campaigns.', 'commerce', false)
ON CONFLICT (id) DO NOTHING;

INSERT INTO vendor_role_permissions(role_id, permission_id)
SELECT role.id, permission.id
FROM (VALUES ('role_owner'), ('role_general_manager'), ('role_branch_manager'), ('role_marketing')) role(id)
CROSS JOIN vendor_permissions permission
WHERE permission.code IN ('vendor.promotions.read','vendor.promotions.manage')
ON CONFLICT DO NOTHING;
