CREATE TABLE IF NOT EXISTS dispatch_rider_otps (
    id UUID PRIMARY KEY,
    phone_number VARCHAR(30) NOT NULL,
    otp_code_hash TEXT NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    expires_at TIMESTAMP NOT NULL,
    verified BOOLEAN NOT NULL DEFAULT false,
    locked_until TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dispatch_rider_otps_phone_number
    ON dispatch_rider_otps (phone_number);

CREATE INDEX IF NOT EXISTS idx_dispatch_rider_otps_expires_at
    ON dispatch_rider_otps (expires_at);

CREATE INDEX IF NOT EXISTS idx_dispatch_rider_otps_verified
    ON dispatch_rider_otps (verified);

CREATE INDEX IF NOT EXISTS idx_dispatch_rider_otps_locked_until
    ON dispatch_rider_otps (locked_until);

CREATE TABLE IF NOT EXISTS dispatch_rider_identities (
    id UUID PRIMARY KEY,
    phone_number VARCHAR(30) NOT NULL UNIQUE,
    status VARCHAR(30) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_dispatch_rider_identities_status CHECK (status IN ('active', 'suspended', 'deleted'))
);

CREATE INDEX IF NOT EXISTS idx_dispatch_rider_identities_phone_number
    ON dispatch_rider_identities (phone_number);

CREATE INDEX IF NOT EXISTS idx_dispatch_rider_identities_status
    ON dispatch_rider_identities (status);

CREATE TABLE IF NOT EXISTS dispatch_rider_sessions (
    id UUID PRIMARY KEY,
    dispatch_rider_id UUID NOT NULL REFERENCES dispatch_rider_identities(id) ON DELETE CASCADE,
    phone_number VARCHAR(30) NOT NULL,
    refresh_token_hash TEXT NOT NULL,
    device_id VARCHAR(255) NULL,
    device_type VARCHAR(100) NULL,
    ip_address VARCHAR(100) NULL,
    user_agent TEXT NULL,
    expires_at TIMESTAMP NOT NULL,
    revoked_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dispatch_rider_sessions_dispatch_rider_id
    ON dispatch_rider_sessions (dispatch_rider_id);

CREATE INDEX IF NOT EXISTS idx_dispatch_rider_sessions_phone_number
    ON dispatch_rider_sessions (phone_number);

CREATE INDEX IF NOT EXISTS idx_dispatch_rider_sessions_refresh_token_hash
    ON dispatch_rider_sessions (refresh_token_hash);

CREATE INDEX IF NOT EXISTS idx_dispatch_rider_sessions_expires_at
    ON dispatch_rider_sessions (expires_at);

CREATE INDEX IF NOT EXISTS idx_dispatch_rider_sessions_revoked_at
    ON dispatch_rider_sessions (revoked_at);
