-- Fuel price reports from drivers, the daily fuel index built from them, and
-- the tariff changes the index proposed.
--
-- The index never changes a fare. When fuel moves past the fuel.adjustment
-- threshold, pricing drafts and submits a tariff version with adjusted per-km
-- rates; an approver must approve and activate it like any other change.

-- One report per driver, fuel type and day: the first report stands, so one
-- driver cannot drag a market's index by reporting repeatedly.
CREATE TABLE IF NOT EXISTS pricing_fuel_reports (
    id                    text PRIMARY KEY,
    consumer_service      text NOT NULL CHECK (consumer_service <> ''),
    provider_ref          text NOT NULL CHECK (provider_ref <> ''),
    fuel_type             text NOT NULL CHECK (fuel_type IN ('PETROL', 'DIESEL', 'GAS')),
    price_per_litre_kobo  bigint NOT NULL CHECK (price_per_litre_kobo BETWEEN 10000 AND 1000000),
    lat_e6                bigint NOT NULL,
    lng_e6                bigint NOT NULL,
    -- Null when the driver was outside every pricing market: kept, but it
    -- feeds no index.
    market_id             text REFERENCES pricing_markets (id),
    zone_id               text,
    report_day            date NOT NULL,
    reported_at           timestamptz NOT NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pricing_fuel_reports_one_per_day UNIQUE (consumer_service, provider_ref, fuel_type, report_day)
);

CREATE INDEX IF NOT EXISTS pricing_fuel_reports_market_day_idx
    ON pricing_fuel_reports (market_id, fuel_type, report_day) WHERE market_id IS NOT NULL;

DROP TRIGGER IF EXISTS pricing_fuel_reports_append_only ON pricing_fuel_reports;
CREATE TRIGGER pricing_fuel_reports_append_only
    BEFORE UPDATE OR DELETE ON pricing_fuel_reports
    FOR EACH ROW EXECUTE FUNCTION pricing_reject_mutation();

-- The index for a market, fuel and day: the median of the reports in a window
-- ending that day, after dropping outliers. Recomputed as reports arrive.
CREATE TABLE IF NOT EXISTS pricing_fuel_index (
    market_id      text NOT NULL REFERENCES pricing_markets (id),
    fuel_type      text NOT NULL CHECK (fuel_type IN ('PETROL', 'DIESEL', 'GAS')),
    index_day      date NOT NULL,
    median_kobo    bigint NOT NULL CHECK (median_kobo > 0),
    report_count   integer NOT NULL CHECK (report_count > 0),
    dropped_count  integer NOT NULL DEFAULT 0 CHECK (dropped_count >= 0),
    window_days    integer NOT NULL CHECK (window_days > 0),
    computed_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (market_id, fuel_type, index_day)
);

-- Every tariff change the index proposed, and what it was based on.
CREATE TABLE IF NOT EXISTS pricing_fuel_proposals (
    id                   text PRIMARY KEY,
    market_id            text NOT NULL REFERENCES pricing_markets (id),
    tariff_kind          text NOT NULL,
    fuel_type            text NOT NULL,
    index_day            date NOT NULL,
    fuel_price_kobo      bigint NOT NULL,
    baseline_kobo        bigint NOT NULL,
    change_bps           bigint NOT NULL,
    share_bps            bigint NOT NULL,
    report_count         integer NOT NULL,
    rules_version_id     text NOT NULL REFERENCES pricing_policy_versions (id),
    base_version_id      text NOT NULL REFERENCES pricing_policy_versions (id),
    proposed_version_id  text NOT NULL REFERENCES pricing_policy_versions (id),
    approval_request_id  text NOT NULL DEFAULT '',
    created_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pricing_fuel_proposals_once_a_day UNIQUE (market_id, tariff_kind, base_version_id, index_day)
);

CREATE INDEX IF NOT EXISTS pricing_fuel_proposals_market_kind_idx
    ON pricing_fuel_proposals (market_id, tariff_kind, created_at DESC);

DROP TRIGGER IF EXISTS pricing_fuel_proposals_append_only ON pricing_fuel_proposals;
CREATE TRIGGER pricing_fuel_proposals_append_only
    BEFORE UPDATE OR DELETE ON pricing_fuel_proposals
    FOR EACH ROW EXECUTE FUNCTION pricing_reject_mutation();

-- The fuel index proposes changes under its own name. It holds EDITOR only, so
-- a proposal still needs a human approver.
INSERT INTO pricing_operators (operator_id, display_name, roles)
VALUES ('system:fuel-index', 'Fuel index (automatic proposals)', '{EDITOR}')
ON CONFLICT DO NOTHING;
