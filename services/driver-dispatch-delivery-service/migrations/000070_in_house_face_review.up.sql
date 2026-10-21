-- First-party verification without third-party face matching.
-- All statuses are workflow states, never evidence of an automated biometric match.
ALTER TABLE face_checks
    DROP CONSTRAINT IF EXISTS chk_face_checks_result;

ALTER TABLE face_checks
    ADD CONSTRAINT chk_face_checks_result CHECK (
        result IS NULL OR result IN (
            'pass', 'fail', 'pending_review', 'manual_approved', 'manual_rejected'
        )
    );

CREATE INDEX IF NOT EXISTS idx_face_checks_review_queue
    ON face_checks(provider_id, checked_at DESC)
    WHERE provider_used = 'karryam_manual_review';
