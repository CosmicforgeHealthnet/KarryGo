CREATE TABLE IF NOT EXISTS request_broadcasts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id UUID NOT NULL UNIQUE,
    service_type TEXT NOT NULL DEFAULT 'dispatch',
    broadcast_radius_km NUMERIC(6,2) NOT NULL,
    attempt_number SMALLINT NOT NULL DEFAULT 1,
    providers_notified INT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'broadcasting',
    broadcast_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_by_provider_id UUID NULL REFERENCES providers(id),
    booking_payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_broadcast_status CHECK (
        status IN ('broadcasting','accepted','expired','cancelled','no_provider_found')
    ),
    CONSTRAINT chk_broadcast_attempt CHECK (attempt_number > 0),
    CONSTRAINT chk_broadcast_radius CHECK (broadcast_radius_km > 0),
    CONSTRAINT chk_broadcast_providers_notified CHECK (providers_notified >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_broadcasts_booking ON request_broadcasts (booking_id);
CREATE INDEX IF NOT EXISTS idx_broadcasts_status ON request_broadcasts (status);
CREATE INDEX IF NOT EXISTS idx_broadcasts_expires ON request_broadcasts (expires_at);
CREATE INDEX IF NOT EXISTS idx_broadcasts_accepted_provider
    ON request_broadcasts (accepted_by_provider_id)
    WHERE accepted_by_provider_id IS NOT NULL;

