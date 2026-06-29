CREATE TABLE IF NOT EXISTS verification_audit (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    step TEXT NOT NULL,
    action TEXT NOT NULL,
    from_status TEXT NOT NULL,
    to_status TEXT NOT NULL,
    performed_by UUID NULL,
    notes TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_verification_audit_step CHECK (
        step IN ('identity', 'licence', 'vehicle', 'face', 'guarantor', 'emergency')
    ),
    CONSTRAINT chk_verification_audit_action CHECK (
        action IN ('approved', 'rejected', 'resubmitted', 'auto_confirmed', 'suspended')
    ),
    CONSTRAINT chk_verification_audit_from_status CHECK (
        from_status IN ('pending', 'submitted', 'approved', 'rejected')
    ),
    CONSTRAINT chk_verification_audit_to_status CHECK (
        to_status IN ('pending', 'submitted', 'approved', 'rejected')
    )
);

CREATE INDEX IF NOT EXISTS idx_vaudit_provider
    ON verification_audit (provider_id);
