CREATE TABLE IF NOT EXISTS analytics_commerce_events (
    event_id text PRIMARY KEY,
    event_type text NOT NULL,
    schema_version integer NOT NULL,
    source_service text NOT NULL,
    source_aggregate text NOT NULL,
    source_id text NOT NULL,
    source_version bigint NOT NULL,
    business_id text,
    vendor_id text,
    branch_id text,
    occurred_at timestamptz NOT NULL,
    payload jsonb NOT NULL,
    payload_hash text NOT NULL,
    status text NOT NULL DEFAULT 'PROCESSED',
    last_error_redacted text NOT NULL DEFAULT '',
    ingested_at timestamptz NOT NULL DEFAULT now(),
    CHECK (status IN ('PROCESSED','DUPLICATE','DEAD_LETTERED'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_analytics_commerce_events_source_version
    ON analytics_commerce_events(source_service, source_aggregate, source_id, source_version);
CREATE INDEX IF NOT EXISTS idx_analytics_commerce_events_business_time
    ON analytics_commerce_events(business_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_analytics_commerce_events_vendor_time
    ON analytics_commerce_events(vendor_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_analytics_commerce_events_type_time
    ON analytics_commerce_events(event_type, occurred_at);

CREATE TABLE IF NOT EXISTS analytics_vendor_daily_metrics (
    business_id text NOT NULL,
    vendor_id text,
    branch_id text,
    metric_date date NOT NULL,
    order_count bigint NOT NULL DEFAULT 0,
    completed_value_kobo bigint NOT NULL DEFAULT 0,
    accepted_count bigint NOT NULL DEFAULT 0,
    rejected_count bigint NOT NULL DEFAULT 0,
    cancelled_vendor_count bigint NOT NULL DEFAULT 0,
    cancelled_customer_count bigint NOT NULL DEFAULT 0,
    refund_count bigint NOT NULL DEFAULT 0,
    preparation_seconds bigint NOT NULL DEFAULT 0,
    preparation_count bigint NOT NULL DEFAULT 0,
    picking_seconds bigint NOT NULL DEFAULT 0,
    picking_count bigint NOT NULL DEFAULT 0,
    own_rider_count bigint NOT NULL DEFAULT 0,
    platform_count bigint NOT NULL DEFAULT 0,
    own_rider_failures bigint NOT NULL DEFAULT 0,
    platform_failures bigint NOT NULL DEFAULT 0,
    own_rider_on_time bigint NOT NULL DEFAULT 0,
    platform_on_time bigint NOT NULL DEFAULT 0,
    quality_reported bigint NOT NULL DEFAULT 0,
    quality_confirmed bigint NOT NULL DEFAULT 0,
    availability_events bigint NOT NULL DEFAULT 0,
    substitution_accuracy_events bigint NOT NULL DEFAULT 0,
    stockout_events bigint NOT NULL DEFAULT 0,
    lost_opportunity_estimate_kobo bigint NOT NULL DEFAULT 0,
    demand_events bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, branch_id, metric_date)
);

CREATE TABLE IF NOT EXISTS analytics_vendor_dimension_metrics (
    business_id text NOT NULL,
    branch_id text NOT NULL DEFAULT '',
    dimension_type text NOT NULL,
    dimension_id text NOT NULL,
    metric_date date NOT NULL,
    orders_count bigint NOT NULL DEFAULT 0,
    units_count bigint NOT NULL DEFAULT 0,
    value_kobo bigint NOT NULL DEFAULT 0,
    demand_events bigint NOT NULL DEFAULT 0,
    stockout_demand_events bigint NOT NULL DEFAULT 0,
    sample_size bigint NOT NULL DEFAULT 0,
    nearby_median_kobo bigint,
    nearby_min_kobo bigint,
    nearby_max_kobo bigint,
    own_price_kobo bigint,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, branch_id, dimension_type, dimension_id, metric_date)
);

CREATE TABLE IF NOT EXISTS analytics_vendor_supplier_metrics (
    business_id text NOT NULL,
    supplier_id text NOT NULL,
    item_id text NOT NULL DEFAULT '',
    metric_date date NOT NULL,
    deliveries_count bigint NOT NULL DEFAULT 0,
    on_time_count bigint NOT NULL DEFAULT 0,
    ordered_quantity bigint NOT NULL DEFAULT 0,
    accepted_quantity bigint NOT NULL DEFAULT 0,
    damaged_quantity bigint NOT NULL DEFAULT 0,
    total_purchase_cost_kobo bigint NOT NULL DEFAULT 0,
    unit_cost_samples bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, supplier_id, item_id, metric_date)
);

CREATE TABLE IF NOT EXISTS analytics_vendor_capacity_metrics (
    business_id text NOT NULL,
    branch_id text NOT NULL DEFAULT '',
    production_type text NOT NULL,
    metric_date date NOT NULL,
    available_units bigint NOT NULL DEFAULT 0,
    used_units bigint NOT NULL DEFAULT 0,
    samples bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, branch_id, production_type, metric_date)
);

CREATE TABLE IF NOT EXISTS analytics_vendor_rfq_metrics (
    business_id text NOT NULL,
    branch_id text NOT NULL DEFAULT '',
    metric_date date NOT NULL,
    received bigint NOT NULL DEFAULT 0,
    quotes_submitted bigint NOT NULL DEFAULT 0,
    accepted bigint NOT NULL DEFAULT 0,
    rejected bigint NOT NULL DEFAULT 0,
    expired bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, branch_id, metric_date)
);

CREATE TABLE IF NOT EXISTS analytics_vendor_crm_cohorts (
    business_id text NOT NULL,
    branch_id text NOT NULL DEFAULT '',
    cohort text NOT NULL,
    metric_date date NOT NULL,
    customers bigint NOT NULL DEFAULT 0,
    total_lifetime_kobo bigint NOT NULL DEFAULT 0,
    policy_version text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, branch_id, cohort, metric_date, policy_version)
);

CREATE TABLE IF NOT EXISTS analytics_vendor_reliability_scores (
    id text PRIMARY KEY,
    business_id text NOT NULL,
    branch_id text NOT NULL DEFAULT '',
    window_start timestamptz NOT NULL,
    window_end timestamptz NOT NULL,
    policy_version text NOT NULL,
    score integer,
    status text NOT NULL,
    component_values jsonb NOT NULL,
    component_weights jsonb NOT NULL,
    input_event_max_occurred_at timestamptz,
    sample_size bigint NOT NULL,
    calculated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (status IN ('CALCULATED','INSUFFICIENT_DATA'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_analytics_vendor_reliability_unique_window
    ON analytics_vendor_reliability_scores(business_id, branch_id, window_start, window_end, policy_version);

CREATE TABLE IF NOT EXISTS analytics_order_guarantee_policy_models (
    policy_version text PRIMARY KEY,
    status text NOT NULL,
    policy jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (status = 'FUTURE_DISABLED')
);

INSERT INTO analytics_order_guarantee_policy_models(policy_version, status, policy)
VALUES (
    'order_guarantee_future_disabled_v1',
    'FUTURE_DISABLED',
    '{"launchDependency":false,"customerFacing":false,"compensationEnabled":false,"walletMutationEnabled":false}'::jsonb
)
ON CONFLICT (policy_version) DO NOTHING;
