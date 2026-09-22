-- Section 13 closure: supply by product, demand buckets, mission outcomes by
-- product. Additive to 003; its tables keep serving existing readers.
--
-- A count the authority could not determine is NULL. NULL is UNKNOWN, and is
-- never read back as zero.

CREATE TABLE IF NOT EXISTS supply_product_snapshots (
    authority text NOT NULL,
    market_id text NOT NULL DEFAULT '',
    zone text NOT NULL,
    product_code text NOT NULL,
    online_count bigint NOT NULL CHECK (online_count >= 0),
    eligible_count bigint CHECK (eligible_count IS NULL OR (eligible_count >= 0 AND eligible_count <= online_count)),
    occupied_count bigint CHECK (occupied_count IS NULL OR (occupied_count >= 0 AND occupied_count <= online_count)),
    supply_state text NOT NULL CHECK (supply_state IN ('AVAILABLE','LIMITED','UNKNOWN')),
    policy_version text NOT NULL DEFAULT '',
    observed_at timestamptz NOT NULL,
    bucket_start timestamptz NOT NULL,
    source_version bigint NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (authority, market_id, zone, product_code)
);
CREATE INDEX IF NOT EXISTS idx_supply_product_snapshots_zone
    ON supply_product_snapshots (authority, zone);

-- One row per bucket, replaced only by a strictly newer observation.
CREATE TABLE IF NOT EXISTS demand_buckets (
    authority text NOT NULL,
    market_id text NOT NULL DEFAULT '',
    zone text NOT NULL,
    product_code text NOT NULL,
    bucket_start timestamptz NOT NULL,
    bucket_minutes integer NOT NULL CHECK (bucket_minutes > 0),
    demand_count bigint NOT NULL CHECK (demand_count >= 0),
    unmatched_count bigint NOT NULL CHECK (unmatched_count >= 0 AND unmatched_count <= demand_count),
    observed_at timestamptz NOT NULL,
    source_version bigint NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (authority, market_id, zone, product_code, bucket_start)
);
CREATE INDEX IF NOT EXISTS idx_demand_buckets_slot
    ON demand_buckets (authority, zone, product_code, bucket_start);

CREATE TABLE IF NOT EXISTS mission_outcomes_daily (
    authority text NOT NULL,
    product_code text NOT NULL,
    metric_date date NOT NULL,
    committed bigint NOT NULL DEFAULT 0,
    assigned bigint NOT NULL DEFAULT 0,
    completed bigint NOT NULL DEFAULT 0,
    failed bigint NOT NULL DEFAULT 0,
    cancelled bigint NOT NULL DEFAULT 0,
    recovered bigint NOT NULL DEFAULT 0,
    recovery_failed bigint NOT NULL DEFAULT 0,
    guarantee_breaches bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (authority, product_code, metric_date)
);
