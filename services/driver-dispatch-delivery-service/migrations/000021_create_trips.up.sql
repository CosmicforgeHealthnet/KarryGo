CREATE TABLE trips (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id UUID NOT NULL UNIQUE,
    provider_id UUID NOT NULL REFERENCES providers(id),
    customer_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'assigned',
    pickup_address TEXT NOT NULL,
    pickup_lat NUMERIC(10,7) NOT NULL,
    pickup_lng NUMERIC(10,7) NOT NULL,
    dropoff_address TEXT NOT NULL,
    dropoff_lat NUMERIC(10,7) NOT NULL,
    dropoff_lng NUMERIC(10,7) NOT NULL,
    distance_km NUMERIC(8,2) NOT NULL DEFAULT 0,
    fare_amount BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'NGN',
    receiver_name TEXT NOT NULL,
    receiver_phone TEXT NOT NULL,
    package_desc TEXT NULL,
    package_weight NUMERIC(6,2) NULL,
    started_at TIMESTAMPTZ NULL,
    arrived_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    cancelled_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_trip_status CHECK (status IN (
        'assigned',
        'en_route_pickup',
        'arrived_pickup',
        'in_progress',
        'proof_submitted',
        'completed',
        'cancelled',
        'failed'
    ))
);

CREATE UNIQUE INDEX idx_trips_booking ON trips (booking_id);
CREATE INDEX idx_trips_provider ON trips (provider_id);
CREATE INDEX idx_trips_status ON trips (status);
CREATE INDEX idx_trips_provider_active ON trips (provider_id, status)
WHERE status NOT IN ('completed', 'cancelled', 'failed');
CREATE INDEX idx_trips_created_at ON trips (created_at DESC);
CREATE INDEX idx_trips_provider_created_at ON trips (provider_id, created_at DESC);
