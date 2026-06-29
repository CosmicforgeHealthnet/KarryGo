CREATE TABLE IF NOT EXISTS availability_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    went_online_at TIMESTAMPTZ NOT NULL,
    went_offline_at TIMESTAMPTZ NULL,
    duration_minutes INT NULL,
    trips_in_session INT NOT NULL DEFAULT 0,
    forced_offline BOOL NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_availability_sessions_duration CHECK (
        duration_minutes IS NULL OR duration_minutes >= 0
    ),
    CONSTRAINT chk_availability_sessions_trips CHECK (
        trips_in_session >= 0
    )
);

CREATE INDEX IF NOT EXISTS idx_sessions_provider
    ON availability_sessions (provider_id);

CREATE INDEX IF NOT EXISTS idx_sessions_online_at
    ON availability_sessions (went_online_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_provider_open
    ON availability_sessions (provider_id)
    WHERE went_offline_at IS NULL;
