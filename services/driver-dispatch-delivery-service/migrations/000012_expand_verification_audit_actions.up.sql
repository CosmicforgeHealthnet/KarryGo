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
