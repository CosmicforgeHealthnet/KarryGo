-- Distributed matching audit trail (addendum §30) and active-trip telemetry
-- (addendum §8). Both are append-only / audit tables read by the matching and
-- telemetry-ingestion features, not by anything built in phases A0-A4.

CREATE TABLE IF NOT EXISTS taxi_match_attempts (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id       UUID NOT NULL REFERENCES taxi_bookings(id) ON DELETE CASCADE,
    matcher_instance TEXT NOT NULL,
    started_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at         TIMESTAMPTZ,
    outcome          TEXT   -- matched | unmatched | superseded
);

CREATE TABLE IF NOT EXISTS taxi_match_candidates (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id   UUID NOT NULL REFERENCES taxi_match_attempts(id) ON DELETE CASCADE,
    provider_id  UUID NOT NULL REFERENCES taxi_providers(id),
    eta_seconds  INT,
    distance_km  NUMERIC(8,2),
    score        NUMERIC(10,4),
    rank         INT,
    offer_status TEXT NOT NULL DEFAULT 'scored',  -- scored|offered|accepted|rejected|timed_out
    offered_at   TIMESTAMPTZ,
    resolved_at  TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS taxi_trip_location_points (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id     UUID NOT NULL REFERENCES taxi_bookings(id) ON DELETE CASCADE,
    provider_id    UUID NOT NULL REFERENCES taxi_providers(id),
    sequence       BIGINT NOT NULL,
    lat            DOUBLE PRECISION NOT NULL,
    lng            DOUBLE PRECISION NOT NULL,
    accuracy_m     DOUBLE PRECISION,
    speed_mps      DOUBLE PRECISION,
    heading_deg    DOUBLE PRECISION,
    recorded_at    TIMESTAMPTZ NOT NULL,
    received_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source         TEXT NOT NULL DEFAULT 'gps',   -- gps|fused|provider
    integrity_flags TEXT[] NOT NULL DEFAULT '{}', -- spoof_suspected|impossible_jump|stale|duplicate|out_of_order
    client_event_id TEXT,
    UNIQUE (booking_id, provider_id, sequence)
);

CREATE INDEX IF NOT EXISTS idx_match_attempts_booking   ON taxi_match_attempts(booking_id);
CREATE INDEX IF NOT EXISTS idx_match_candidates_attempt ON taxi_match_candidates(attempt_id);
CREATE INDEX IF NOT EXISTS idx_trip_points_booking      ON taxi_trip_location_points(booking_id, sequence);
