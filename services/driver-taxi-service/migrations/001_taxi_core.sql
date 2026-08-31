-- Core taxi schema: markets/zones, providers with 6-dimension compliance
-- status, vehicles, operational-state history, and bookings over ordered
-- stops rather than flat pickup/dropoff columns.
--
-- Rewritten (not extended) from the original 001-004. This service has never
-- been deployed, so this is the one point where rewriting migration history
-- is correct rather than a violation of "migrations are append-only". Every
-- change after this file must be additive.

-- ─── Markets and operating zones (addendum §29) ───────────────────────────
CREATE TABLE IF NOT EXISTS markets (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    country       TEXT NOT NULL,
    name          TEXT NOT NULL,
    code          TEXT NOT NULL UNIQUE,   -- e.g. 'NG-LAG'
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- A zone's coverage is modeled as a simple circle (center_lat/lng +
-- match_radius_km) rather than a polygon: no PostGIS is installed and none is
-- needed at this scale (per the booking-creation design note in the build
-- plan). match_radius_km serves double duty deliberately -- it is both "how
-- far this zone's coverage circle extends" (used to resolve which zone a
-- pickup point belongs to) and "how far the matcher searches for a driver
-- within it" (used later by matching). Keeping one column for both avoids two
-- radii that could be configured to disagree with no clear meaning for the
-- gap between them.
CREATE TABLE IF NOT EXISTS operating_zones (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- ON DELETE CASCADE deliberately: a zone is a pure child configuration
    -- record of its market with no independent significance. This is safe
    -- even though taxi_bookings references operating_zones, because that
    -- reference (below) has NO explicit ON DELETE -- Postgres's default
    -- RESTRICT -- so a zone that any real booking still points at cannot
    -- actually be deleted; the cascade attempt fails closed at that deeper
    -- constraint instead of silently orphaning booking history.
    market_id             UUID NOT NULL REFERENCES markets(id) ON DELETE CASCADE,
    name                  TEXT NOT NULL,
    code                  TEXT NOT NULL,
    center_lat            DOUBLE PRECISION NOT NULL,
    center_lng            DOUBLE PRECISION NOT NULL,
    supported_classes     TEXT[] NOT NULL DEFAULT '{}',   -- e.g. {economy,comfort,xl}
    match_radius_km       NUMERIC(6,2) NOT NULL DEFAULT 10,
    is_airport_zone       BOOLEAN NOT NULL DEFAULT false,
    compliance_profile    TEXT NOT NULL DEFAULT 'standard',
    version               INT NOT NULL DEFAULT 1,
    status                TEXT NOT NULL DEFAULT 'active',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (market_id, code)
);

-- ─── Providers: 6 status dimensions, no single field represents compliance ─
CREATE TABLE IF NOT EXISTS taxi_providers (
    id                            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone                         TEXT UNIQUE,
    email                         TEXT UNIQUE,
    first_name                    TEXT NOT NULL DEFAULT '',
    last_name                     TEXT NOT NULL DEFAULT '',
    profile_photo_url             TEXT,
    photo_asset_id                TEXT,

    -- Addendum §11.1: six independent status dimensions. No single generic
    -- status field is expected to represent all of them.
    account_status                TEXT NOT NULL DEFAULT 'active',              -- active | suspended
    onboarding_status             TEXT NOT NULL DEFAULT 'profile_required',    -- profile_required | complete
    identity_verification_status  TEXT NOT NULL DEFAULT 'not_started',         -- not_started | pending | verified | failed
    vehicle_operational_status    TEXT NOT NULL DEFAULT 'none',                -- none | active | inactive
    vehicle_verification_status   TEXT NOT NULL DEFAULT 'not_started',         -- not_started | pending | verified | failed
    document_compliance_status    TEXT NOT NULL DEFAULT 'not_started',         -- not_started | pending | compliant | expired

    rating                        NUMERIC(3,2) DEFAULT 5.00,
    total_trips                   INT  NOT NULL DEFAULT 0,

    location_state                 TEXT NOT NULL DEFAULT '',
    location_city                  TEXT NOT NULL DEFAULT '',
    operation_mode                 TEXT NOT NULL DEFAULT '',
    service_type                   TEXT NOT NULL DEFAULT '',
    language                       TEXT NOT NULL DEFAULT '',
    gov_id_url                     TEXT NOT NULL DEFAULT '',
    driver_license_url             TEXT NOT NULL DEFAULT '',
    vehicle_reg_url                TEXT NOT NULL DEFAULT '',
    driver_license_number          TEXT NOT NULL DEFAULT '',
    license_expiry_date            TEXT NOT NULL DEFAULT '',
    guarantor_name                 TEXT NOT NULL DEFAULT '',
    guarantor_phone                TEXT NOT NULL DEFAULT '',
    emergency_contact_name         TEXT NOT NULL DEFAULT '',
    emergency_contact_phone        TEXT NOT NULL DEFAULT '',
    emergency_contact_relationship TEXT NOT NULL DEFAULT '',

    created_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS provider_sessions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id        UUID NOT NULL REFERENCES taxi_providers(id) ON DELETE CASCADE,
    refresh_token_hash TEXT NOT NULL,
    device_id          TEXT NOT NULL DEFAULT '',
    user_agent         TEXT NOT NULL DEFAULT '',
    ip_address         TEXT NOT NULL DEFAULT '',
    expires_at         TIMESTAMPTZ NOT NULL,
    revoked_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS vehicles (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id   UUID NOT NULL REFERENCES taxi_providers(id) ON DELETE CASCADE,
    vehicle_class TEXT NOT NULL,
    plate_number  TEXT NOT NULL UNIQUE,
    seats         INT  NOT NULL DEFAULT 4,
    year          INT,
    make          TEXT,
    model         TEXT,
    color         TEXT,
    status        TEXT NOT NULL DEFAULT 'active',    -- active | inactive
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── Provider operational state (addendum §5) ──────────────────────────────
-- Current state row per provider, plus an append-only history table. This is
-- the machine that replaces the boolean online/offline built in phase A4.
CREATE TABLE IF NOT EXISTS provider_operational_state (
    provider_id    UUID PRIMARY KEY REFERENCES taxi_providers(id) ON DELETE CASCADE,
    state          TEXT NOT NULL DEFAULT 'OFFLINE',
    -- OFFLINE | AVAILABLE | OFFERED | RESERVED | EN_ROUTE_PICKUP | AT_PICKUP
    -- | ON_TRIP | PAUSED | SAFETY_HOLD | COMPLIANCE_HOLD
    reason         TEXT NOT NULL DEFAULT '',
    booking_id     UUID,
    effective_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    version        BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS provider_operational_state_history (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id  UUID NOT NULL REFERENCES taxi_providers(id) ON DELETE CASCADE,
    from_state   TEXT NOT NULL,
    to_state     TEXT NOT NULL,
    reason       TEXT NOT NULL DEFAULT '',
    booking_id   UUID,
    changed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── Bookings over ordered stops (addendum §15) ────────────────────────────
-- No pickup_address/pickup_lat/... columns here: every location lives in
-- taxi_booking_stops. A STANDARD booking always has exactly one PICKUP and
-- one DROPOFF stop, seeded at creation; a HOURLY booking may add WAYPOINTs.
CREATE TABLE IF NOT EXISTS taxi_bookings (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id              UUID NOT NULL,
    provider_id              UUID REFERENCES taxi_providers(id),
    vehicle_id               UUID REFERENCES vehicles(id),

    -- Deliberately NO ON DELETE here (Postgres default RESTRICT): a booking is
    -- real financial/audit history and a market or zone must never be
    -- deletable, nor a booking row silently vanish, while any booking still
    -- references it. See the comment on operating_zones.market_id above for
    -- how this protects the cascade from that table too.
    market_id                UUID NOT NULL REFERENCES markets(id),
    operating_zone_id        UUID NOT NULL REFERENCES operating_zones(id),

    booking_mode              TEXT NOT NULL DEFAULT 'standard',   -- standard | hourly
    vehicle_class              TEXT NOT NULL DEFAULT 'economy',
    passenger_count            INT  NOT NULL DEFAULT 1,
    ride_notes                 TEXT NOT NULL DEFAULT '',
    reserved_duration_minutes  INT,          -- hourly mode only

    -- Fare is a SNAPSHOT from PricingClient, never computed here.
    fare_estimate_kobo        BIGINT,
    fare_final_kobo           BIGINT,
    pricing_quote_reference   TEXT NOT NULL DEFAULT '',   -- opaque id from the pricing engine
    pricing_version           TEXT NOT NULL DEFAULT '',   -- rate-table version the quote used
    distance_km               NUMERIC(8,2),
    duration_minutes          INT,
    actual_distance_km        NUMERIC(8,2),

    payment_method            TEXT NOT NULL DEFAULT 'wallet',
    payment_status            TEXT NOT NULL DEFAULT 'unpaid',
    payment_intent_id         TEXT,

    status                     TEXT NOT NULL DEFAULT 'pending_match',
    pickup_verification_status TEXT NOT NULL DEFAULT 'not_required',
    -- not_required | pending | verified

    scheduled_at               TIMESTAMPTZ,        -- NULL = dispatch immediately
    dispatch_start_at          TIMESTAMPTZ,
    reservation_status         TEXT NOT NULL DEFAULT 'none',   -- none|reserved|backup_dispatched
    reserved_provider_id       UUID REFERENCES taxi_providers(id),
    backup_dispatch_at         TIMESTAMPTZ,
    provider_confirmed_at      TIMESTAMPTZ,
    customer_ready_window      INT,   -- minutes, optional

    cancel_reason              TEXT,
    cancelled_by                TEXT,

    matched_at                  TIMESTAMPTZ,
    accepted_at                  TIMESTAMPTZ,
    arrived_at                   TIMESTAMPTZ,
    started_at                   TIMESTAMPTZ,
    completed_at                 TIMESTAMPTZ,
    cancelled_at                 TIMESTAMPTZ,
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One active booking per provider, enforced by the database, not application
-- code (addendum §5.1). A second concurrent accept fails here identically
-- across every service instance.
CREATE UNIQUE INDEX IF NOT EXISTS uq_taxi_bookings_one_active_per_provider
  ON taxi_bookings(provider_id)
  WHERE status IN ('awaiting_acceptance','accepted','en_route_pickup',
                    'arrived_at_pickup','pickup_verification_pending','in_progress');

CREATE TABLE IF NOT EXISTS taxi_booking_stops (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id   UUID NOT NULL REFERENCES taxi_bookings(id) ON DELETE CASCADE,
    sequence     INT NOT NULL,
    type         TEXT NOT NULL,     -- PICKUP | WAYPOINT | DROPOFF
    address      TEXT NOT NULL DEFAULT '',
    lat          DOUBLE PRECISION NOT NULL,
    lng          DOUBLE PRECISION NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',   -- pending|arrived|completed|skipped
    arrived_at   TIMESTAMPTZ,
    departed_at  TIMESTAMPTZ,
    wait_seconds INT,
    notes        TEXT NOT NULL DEFAULT '',
    UNIQUE (booking_id, sequence)
);

CREATE TABLE IF NOT EXISTS booking_events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id UUID NOT NULL REFERENCES taxi_bookings(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id   TEXT NOT NULL,
    metadata   JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS booking_reviews (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id        UUID NOT NULL REFERENCES taxi_bookings(id) ON DELETE CASCADE,
    direction         TEXT NOT NULL,   -- 'customer_to_provider' | 'provider_to_customer'
    customer_id       UUID NOT NULL,
    provider_id       UUID NOT NULL REFERENCES taxi_providers(id),
    rating            INT  NOT NULL CHECK (rating BETWEEN 1 AND 5),
    review_text       TEXT NOT NULL DEFAULT '',
    tags              TEXT[] NOT NULL DEFAULT '{}',
    recommends_driver BOOLEAN,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (booking_id, direction)
);

CREATE INDEX IF NOT EXISTS idx_taxi_bookings_customer   ON taxi_bookings(customer_id);
CREATE INDEX IF NOT EXISTS idx_taxi_bookings_provider   ON taxi_bookings(provider_id);
CREATE INDEX IF NOT EXISTS idx_taxi_bookings_status     ON taxi_bookings(status);
CREATE INDEX IF NOT EXISTS idx_taxi_bookings_market     ON taxi_bookings(market_id, operating_zone_id);
CREATE INDEX IF NOT EXISTS idx_booking_stops_booking    ON taxi_booking_stops(booking_id);
CREATE INDEX IF NOT EXISTS idx_booking_events_booking   ON booking_events(booking_id);
CREATE INDEX IF NOT EXISTS idx_vehicles_provider        ON vehicles(provider_id);
CREATE INDEX IF NOT EXISTS idx_provider_sessions_prov   ON provider_sessions(provider_id);
CREATE INDEX IF NOT EXISTS idx_op_state_history_provider ON provider_operational_state_history(provider_id);
