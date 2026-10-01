-- Pricing geography: where a rate applies.
--
-- Four layers, most specific wins when a policy is resolved:
--
--   country  ->  state  ->  market  ->  zone
--   Nigeria      Lagos     NG-LAG     NG-LAG-ISLAND
--
-- This is deliberately SEPARATE from each service's operating zones. An
-- operating zone answers "may we serve here and match a driver", which is an
-- operational question owned by the booking service. A pricing zone answers
-- "what does it cost here", which is a commercial question owned here. Taxi's
-- existing zones map onto pricing zones through pricing_zone_mappings, so the
-- two can differ without either service lying to the other.

CREATE TABLE IF NOT EXISTS pricing_countries (
    code        text PRIMARY KEY CHECK (code ~ '^[A-Z]{2}$'),
    name        text NOT NULL CHECK (name <> ''),
    currency    char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS pricing_states (
    id           text PRIMARY KEY,
    country_code text NOT NULL REFERENCES pricing_countries (code),
    code         text NOT NULL CHECK (code <> ''),
    name         text NOT NULL CHECK (name <> ''),
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (country_code, code)
);

-- A market is a coherent commercial area, such as Lagos Metropolitan. Its code
-- intentionally matches the market codes the booking services already use
-- (taxi's markets.code, e.g. NG-LAG), so operators do not have to hold two
-- vocabularies in their heads.
CREATE TABLE IF NOT EXISTS pricing_markets (
    id           text PRIMARY KEY,
    country_code text NOT NULL REFERENCES pricing_countries (code),
    state_id     text REFERENCES pricing_states (id),
    code         text NOT NULL UNIQUE CHECK (code <> ''),
    name         text NOT NULL CHECK (name <> ''),
    timezone     text NOT NULL DEFAULT 'Africa/Lagos',
    status       text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS pricing_zones (
    id         text PRIMARY KEY,
    market_id  text NOT NULL REFERENCES pricing_markets (id),
    code       text NOT NULL CHECK (code <> ''),
    name       text NOT NULL CHECK (name <> ''),
    status     text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (market_id, code)
);

-- A zone's shape is versioned and immutable, because a quote records which zone
-- version priced it. Redrawing a boundary must not silently change what a past
-- quote claims about where the trip was.
--
-- The shape is JSON so a circle today can become a polygon later without a
-- schema change. Coordinates are stored as microdegree INTEGERS, matching the
-- canonical encoding: a float boundary would make the same zone hash differently
-- on different machines.
CREATE TABLE IF NOT EXISTS pricing_zone_versions (
    id          text PRIMARY KEY,
    zone_id     text NOT NULL REFERENCES pricing_zones (id),
    version_no  int NOT NULL CHECK (version_no > 0),
    shape       jsonb NOT NULL,
    shape_hash  text NOT NULL CHECK (shape_hash ~ '^[0-9a-f]{64}$'),
    created_by  text NOT NULL CHECK (created_by <> ''),
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (zone_id, version_no),
    CONSTRAINT pricing_zone_versions_known_shape CHECK (shape ? 'type')
);

DROP TRIGGER IF EXISTS pricing_zone_versions_append_only ON pricing_zone_versions;
CREATE TRIGGER pricing_zone_versions_append_only
    BEFORE UPDATE OR DELETE ON pricing_zone_versions
    FOR EACH ROW EXECUTE FUNCTION pricing_reject_mutation();

-- Maps a consuming service's own zone identifier onto a pricing zone.
--
-- valid_from exists so a remapping is a new row rather than an edit: a quote
-- issued last month must still resolve the mapping that applied then.
CREATE TABLE IF NOT EXISTS pricing_zone_mappings (
    id               text PRIMARY KEY,
    consumer_service text NOT NULL CHECK (consumer_service <> ''),
    consumer_zone_id text NOT NULL CHECK (consumer_zone_id <> ''),
    pricing_zone_id  text NOT NULL REFERENCES pricing_zones (id),
    valid_from       timestamptz NOT NULL DEFAULT now(),
    created_by       text NOT NULL CHECK (created_by <> ''),
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (consumer_service, consumer_zone_id, valid_from)
);

CREATE INDEX IF NOT EXISTS pricing_zone_mappings_lookup_idx
    ON pricing_zone_mappings (consumer_service, consumer_zone_id, valid_from DESC);

CREATE INDEX IF NOT EXISTS pricing_zones_market_idx ON pricing_zones (market_id);
CREATE INDEX IF NOT EXISTS pricing_markets_country_idx ON pricing_markets (country_code);
