CREATE TABLE IF NOT EXISTS provider_request_inbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    broadcast_id UUID NOT NULL REFERENCES request_broadcasts(id) ON DELETE CASCADE,
    booking_id UUID NOT NULL,
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at TIMESTAMPTZ NULL,
    fcm_sent BOOLEAN NOT NULL DEFAULT false,
    fcm_sent_at TIMESTAMPTZ NULL,
    CONSTRAINT chk_inbox_status CHECK (
        status IN ('pending','accepted','rejected','expired')
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_inbox_provider_booking ON provider_request_inbox (provider_id, booking_id);
CREATE INDEX IF NOT EXISTS idx_inbox_broadcast ON provider_request_inbox (broadcast_id);
CREATE INDEX IF NOT EXISTS idx_inbox_provider ON provider_request_inbox (provider_id);
CREATE INDEX IF NOT EXISTS idx_inbox_status ON provider_request_inbox (status);
CREATE INDEX IF NOT EXISTS idx_inbox_provider_status_received ON provider_request_inbox (provider_id, status, received_at DESC);
CREATE INDEX IF NOT EXISTS idx_inbox_booking ON provider_request_inbox (booking_id);

