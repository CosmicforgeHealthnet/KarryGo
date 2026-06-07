-- Expand verification_audit constraints to support provider-level gate actions.
--
-- fully_approved: inserted once when all required verification steps are approved.
-- 'all' step: used for provider-level audit rows that do not correspond to a single step.

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
            'face_failed',
            'fully_approved'
        )
    );

ALTER TABLE verification_audit
    DROP CONSTRAINT IF EXISTS chk_verification_audit_step;

ALTER TABLE verification_audit
    ADD CONSTRAINT chk_verification_audit_step CHECK (
        step IN ('identity', 'licence', 'vehicle', 'face', 'guarantor', 'emergency', 'all')
    );
