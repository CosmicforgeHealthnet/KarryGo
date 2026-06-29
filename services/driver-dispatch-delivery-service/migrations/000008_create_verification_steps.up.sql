CREATE TABLE IF NOT EXISTS verification_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    step TEXT NOT NULL,
    is_optional BOOL NOT NULL DEFAULT false,
    is_auto_confirmed BOOL NOT NULL DEFAULT false,
    confirm_method TEXT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    submitted_at TIMESTAMPTZ NULL,
    reviewed_at TIMESTAMPTZ NULL,
    reviewer_id UUID NULL,
    rejection_reason TEXT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_verification_steps_step CHECK (
        step IN ('identity', 'licence', 'vehicle', 'face', 'guarantor', 'emergency')
    ),
    CONSTRAINT chk_verification_steps_status CHECK (
        status IN ('pending', 'submitted', 'approved', 'rejected')
    ),
    CONSTRAINT chk_verification_steps_confirm_method CHECK (
        confirm_method IS NULL OR confirm_method IN ('manual', 'auto')
    ),
    UNIQUE (provider_id, step)
);

CREATE INDEX IF NOT EXISTS idx_vsteps_provider
    ON verification_steps (provider_id);

CREATE INDEX IF NOT EXISTS idx_vsteps_status
    ON verification_steps (status);
