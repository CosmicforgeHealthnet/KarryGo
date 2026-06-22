-- Truck providers
CREATE TABLE IF NOT EXISTS truck_providers (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone                TEXT UNIQUE,
    email                TEXT UNIQUE,
    first_name           TEXT NOT NULL DEFAULT '',
    last_name            TEXT NOT NULL DEFAULT '',
    profile_photo_url    TEXT,
    photo_asset_id       TEXT,
    status               TEXT NOT NULL DEFAULT 'active',
    onboarding_status    TEXT NOT NULL DEFAULT 'profile_required',
    rating               NUMERIC(3,2) DEFAULT 5.00,
    total_trips          INT NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Provider auth sessions
CREATE TABLE IF NOT EXISTS provider_sessions (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id          UUID NOT NULL REFERENCES truck_providers(id) ON DELETE CASCADE,
    refresh_token_hash   TEXT NOT NULL,
    device_id            TEXT NOT NULL DEFAULT '',
    user_agent           TEXT NOT NULL DEFAULT '',
    ip_address           TEXT NOT NULL DEFAULT '',
    expires_at           TIMESTAMPTZ NOT NULL,
    revoked_at           TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Trucks registered by providers
CREATE TABLE IF NOT EXISTS trucks (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id  UUID NOT NULL REFERENCES truck_providers(id) ON DELETE CASCADE,
    truck_type   TEXT NOT NULL,
    capacity_kg  INT NOT NULL,
    plate_number TEXT NOT NULL UNIQUE,
    year         INT,
    make         TEXT,
    model        TEXT,
    color        TEXT,
    status       TEXT NOT NULL DEFAULT 'active',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Haulage bookings
CREATE TABLE IF NOT EXISTS haulage_bookings (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id          UUID NOT NULL,
    provider_id          UUID REFERENCES truck_providers(id),
    truck_id             UUID REFERENCES trucks(id),

    pickup_address       TEXT NOT NULL,
    pickup_lat           DOUBLE PRECISION NOT NULL,
    pickup_lng           DOUBLE PRECISION NOT NULL,

    dropoff_address      TEXT NOT NULL,
    dropoff_lat          DOUBLE PRECISION NOT NULL,
    dropoff_lng          DOUBLE PRECISION NOT NULL,

    cargo_type           TEXT NOT NULL,
    cargo_weight_kg      INT NOT NULL,
    cargo_description    TEXT NOT NULL DEFAULT '',
    requires_helpers     BOOLEAN NOT NULL DEFAULT FALSE,
    helper_count         INT NOT NULL DEFAULT 0,

    distance_km          NUMERIC(8,2),
    fare_estimate_kobo   BIGINT,
    fare_final_kobo      BIGINT,

    payment_intent_id    TEXT,

    status               TEXT NOT NULL DEFAULT 'pending_match',
    cancel_reason        TEXT,
    cancelled_by         TEXT,

    matched_at           TIMESTAMPTZ,
    accepted_at          TIMESTAMPTZ,
    picked_up_at         TIMESTAMPTZ,
    delivered_at         TIMESTAMPTZ,
    completed_at         TIMESTAMPTZ,
    cancelled_at         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Booking audit trail
CREATE TABLE IF NOT EXISTS booking_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id  UUID NOT NULL REFERENCES haulage_bookings(id) ON DELETE CASCADE,
    event_type  TEXT NOT NULL,
    actor_type  TEXT NOT NULL,
    actor_id    TEXT NOT NULL,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_haulage_bookings_customer  ON haulage_bookings(customer_id);
CREATE INDEX IF NOT EXISTS idx_haulage_bookings_provider  ON haulage_bookings(provider_id);
CREATE INDEX IF NOT EXISTS idx_haulage_bookings_status    ON haulage_bookings(status);
CREATE INDEX IF NOT EXISTS idx_booking_events_booking     ON booking_events(booking_id);
CREATE INDEX IF NOT EXISTS idx_trucks_provider            ON trucks(provider_id);
CREATE INDEX IF NOT EXISTS idx_provider_sessions_provider ON provider_sessions(provider_id);
