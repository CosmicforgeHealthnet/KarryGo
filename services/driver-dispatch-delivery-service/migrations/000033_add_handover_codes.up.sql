-- Handover verification codes: pickup_code* is the rider's own pickup
-- credential, generated when the rider marks arrived-at-pickup and verified
-- by the customer before the rider can start the trip. delivery_code* is the
-- customer's delivery credential, generated when the rider starts the trip
-- and verified by the rider before proof-of-delivery is accepted.
ALTER TABLE trips
    ADD COLUMN IF NOT EXISTS pickup_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pickup_code_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pickup_code_expires_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS pickup_code_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS pickup_code_resend_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS pickup_code_last_sent_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS pickup_code_verified_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS delivery_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS delivery_code_hash TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS delivery_code_expires_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS delivery_code_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS delivery_code_resend_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS delivery_code_last_sent_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS delivery_code_verified_at TIMESTAMPTZ NULL;
