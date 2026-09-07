-- Section 30 final gap closure: versioned, hash-only OTP and opaque QR
-- credentials for the existing Hauling handover lifecycle.
CREATE TABLE IF NOT EXISTS hauling_handover_challenges (
    challenge_ref UUID PRIMARY KEY,
    booking_id UUID NOT NULL REFERENCES haulage_bookings(id) ON DELETE CASCADE,
    stage TEXT NOT NULL CHECK (stage IN ('pickup', 'delivery')),
    version BIGINT NOT NULL CHECK (version > 0),
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'CONSUMED', 'SUPERSEDED', 'EXPIRED', 'LOCKED', 'INVALIDATED')),
    otp_hash TEXT NOT NULL CHECK (otp_hash <> ''),
    qr_token_hash TEXT NOT NULL CHECK (qr_token_hash <> ''),
    assigned_provider_id UUID NOT NULL,
    owner_id UUID NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    attempt_count INT NOT NULL DEFAULT 0 CHECK (attempt_count BETWEEN 0 AND 5),
    resend_count INT NOT NULL DEFAULT 0 CHECK (resend_count BETWEEN 0 AND 5),
    last_sent_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    consumed_at TIMESTAMPTZ NULL,
    consumed_method TEXT NULL CHECK (consumed_method IS NULL OR consumed_method IN ('OTP', 'QR')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (booking_id, stage, version)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_hauling_handover_active_stage
    ON hauling_handover_challenges(booking_id, stage)
    WHERE status = 'ACTIVE';

CREATE INDEX IF NOT EXISTS ix_hauling_handover_booking_history
    ON hauling_handover_challenges(booking_id, stage, version DESC);

-- New application code never writes these legacy plaintext columns. The
-- constraint makes that invariant database-enforced after deployment. Existing
-- keyed legacy hashes remain available for the bounded drain window.
UPDATE haulage_bookings SET pickup_code = '', delivery_code = ''
WHERE pickup_code <> '' OR delivery_code <> '';

ALTER TABLE haulage_bookings
    ADD CONSTRAINT haulage_bookings_no_plaintext_handover
    CHECK (pickup_code = '' AND delivery_code = '');

COMMENT ON TABLE hauling_handover_challenges IS
    'Existing Hauling handover authority: hash-only alternative OTP/QR credentials sharing one versioned, single-use lifecycle.';

