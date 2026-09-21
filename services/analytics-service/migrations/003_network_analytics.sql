-- Section 13: network analytics.
--
-- Everything here is a READ MODEL over facts the owning services already
-- decided. Analytics never writes back, and nothing reads these tables to
-- make a decision it could not make without them: every consumer has a
-- neutral answer when analytics is absent (shared/go/networkanalytics).
--
-- No location, contact, free text or money is stored. The ingest minimizes
-- every payload before it reaches the event store, and none of the read
-- models has a column that could hold one.

-- The deduplicated, minimized event store.
CREATE TABLE IF NOT EXISTS analytics_network_events (
    event_id text PRIMARY KEY,
    event_name text NOT NULL,
    kind text NOT NULL DEFAULT '',
    authority text NOT NULL DEFAULT '',
    source_service text NOT NULL,
    source_entity_type text NOT NULL,
    source_entity_id text NOT NULL,
    source_version bigint NOT NULL CHECK (source_version >= 1),
    occurred_at timestamptz NOT NULL,
    correlation_id text NOT NULL DEFAULT '',
    payload jsonb NOT NULL,
    payload_hash text NOT NULL,
    -- PROCESSED: projected. STALE: an older version of something already
    -- seen, stored for audit but not projected. IGNORED: a name analytics
    -- does not project; kept so adding a projection later can replay it.
    status text NOT NULL CHECK (status IN ('PROCESSED','STALE','IGNORED')),
    ingested_at timestamptz NOT NULL DEFAULT now()
);

-- One fact, one row: a producer that re-publishes the same transition under
-- a new event id is still the same fact.
CREATE UNIQUE INDEX IF NOT EXISTS uq_analytics_network_event_identity
    ON analytics_network_events (source_service, source_entity_type, source_entity_id, event_name, source_version);
CREATE INDEX IF NOT EXISTS idx_analytics_network_events_kind_time
    ON analytics_network_events (kind, occurred_at);

-- The newest version seen per entity and event name. An event at or below it
-- is a replay or a regression and is not projected again.
CREATE TABLE IF NOT EXISTS analytics_network_entity_versions (
    source_service text NOT NULL,
    source_entity_type text NOT NULL,
    source_entity_id text NOT NULL,
    event_name text NOT NULL,
    last_version bigint NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source_service, source_entity_type, source_entity_id, event_name)
);

-- Recovery cases still open, so a resolution can be timed against its start.
CREATE TABLE IF NOT EXISTS analytics_network_open_recoveries (
    authority text NOT NULL,
    case_ref text NOT NULL,
    started_at timestamptz NOT NULL,
    PRIMARY KEY (authority, case_ref)
);

CREATE TABLE IF NOT EXISTS vendor_branch_readiness_stats (
    business_id text NOT NULL DEFAULT '',
    branch_id text NOT NULL,
    metric_date date NOT NULL,
    ready_count bigint NOT NULL DEFAULT 0,
    on_time_count bigint NOT NULL DEFAULT 0,
    late_count bigint NOT NULL DEFAULT 0,
    lateness_seconds_total bigint NOT NULL DEFAULT 0,
    prep_seconds_total bigint NOT NULL DEFAULT 0,
    prep_count bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (branch_id, metric_date)
);

CREATE TABLE IF NOT EXISTS provider_quality_snapshots (
    authority text NOT NULL,
    provider_id text NOT NULL,
    metric_date date NOT NULL,
    completed bigint NOT NULL DEFAULT 0,
    failed bigint NOT NULL DEFAULT 0,
    cancelled bigint NOT NULL DEFAULT 0,
    arrivals bigint NOT NULL DEFAULT 0,
    declines bigint NOT NULL DEFAULT 0,
    recoveries_as_original bigint NOT NULL DEFAULT 0,
    recoveries_as_replacement bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (authority, provider_id, metric_date)
);

-- Latest reported supply per zone. A snapshot is current state, so it is
-- replaced only by a strictly newer one.
CREATE TABLE IF NOT EXISTS zone_supply_snapshots (
    authority text NOT NULL,
    zone text NOT NULL,
    available_providers bigint NOT NULL CHECK (available_providers >= 0),
    busy_providers bigint NOT NULL CHECK (busy_providers >= 0),
    observed_at timestamptz NOT NULL,
    source_version bigint NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (authority, zone)
);

-- The demand projection.
CREATE TABLE IF NOT EXISTS zone_demand_daily (
    authority text NOT NULL,
    zone text NOT NULL,
    metric_date date NOT NULL,
    requests bigint NOT NULL DEFAULT 0,
    unmatched bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (authority, zone, metric_date)
);

CREATE TABLE IF NOT EXISTS mission_success_daily (
    authority text NOT NULL,
    metric_date date NOT NULL,
    completed bigint NOT NULL DEFAULT 0,
    failed bigint NOT NULL DEFAULT 0,
    cancelled bigint NOT NULL DEFAULT 0,
    guarantee_breaches bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (authority, metric_date)
);

CREATE TABLE IF NOT EXISTS recovery_performance (
    authority text NOT NULL,
    metric_date date NOT NULL,
    started bigint NOT NULL DEFAULT 0,
    replaced bigint NOT NULL DEFAULT 0,
    resolved bigint NOT NULL DEFAULT 0,
    failed bigint NOT NULL DEFAULT 0,
    resolution_seconds_total bigint NOT NULL DEFAULT 0,
    resolution_count bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (authority, metric_date)
);

CREATE TABLE IF NOT EXISTS capability_mismatch_metrics (
    authority text NOT NULL,
    metric_date date NOT NULL,
    reason_code text NOT NULL,
    mismatches bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (authority, metric_date, reason_code)
);
