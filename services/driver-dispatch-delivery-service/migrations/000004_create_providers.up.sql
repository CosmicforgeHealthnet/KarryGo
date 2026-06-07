CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS providers (
    id UUID PRIMARY KEY,
    phone TEXT NOT NULL UNIQUE,
    full_name TEXT NULL,
    email TEXT NULL,
    state TEXT NULL,
    city TEXT NULL,
    country TEXT NOT NULL DEFAULT 'NG',
    profile_photo_url TEXT NULL,
    operation_type TEXT NULL,
    verification_status TEXT NOT NULL DEFAULT 'unverified',
    avg_rating NUMERIC(3,2) NOT NULL DEFAULT 0.00,
    total_trips INT NOT NULL DEFAULT 0,
    is_active BOOL NOT NULL DEFAULT true,
    onboarding_complete BOOL NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_providers_operation_type CHECK (
        operation_type IS NULL OR operation_type IN ('individual', 'fleet')
    ),
    CONSTRAINT chk_providers_verification_status CHECK (
        verification_status IN ('unverified', 'pending_review', 'verified', 'suspended', 'rejected')
    )
);

CREATE INDEX IF NOT EXISTS idx_providers_status
    ON providers (verification_status);
