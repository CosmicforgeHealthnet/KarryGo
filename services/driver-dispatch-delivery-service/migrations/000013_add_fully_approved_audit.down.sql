-- Revert to pre-000013 constraints (removes fully_approved and 'all').

ALTER TABLE verification_audit
    DROP CONSTRAINT IF EXISTS chk_verification_audit_action;

ALTER TABLE verification_audit
    ADD CONSTRAINT chk_verification_audit_action CHECK (
        action IN (
            'submitted',
            'approved',
            'rejected',
            'resubmitted',
            'auto_confirmed',
            'suspended',
            'face_failed'
        )
    );

ALTER TABLE verification_audit
    DROP CONSTRAINT IF EXISTS chk_verification_audit_step;

ALTER TABLE verification_audit
    ADD CONSTRAINT chk_verification_audit_step CHECK (
        step IN ('identity', 'licence', 'vehicle', 'face', 'guarantor', 'emergency')
    );
