CREATE TABLE IF NOT EXISTS provider_availability (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'offline',
    verified_to_go_online BOOL NOT NULL DEFAULT false,
    session_start TIMESTAMPTZ NULL,
    last_changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_provider_availability_status CHECK (
        status IN ('online', 'offline', 'busy')
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_avail_provider
    ON provider_availability (provider_id);

CREATE INDEX IF NOT EXISTS idx_avail_status
    ON provider_availability (status);
