DROP INDEX IF EXISTS idx_face_checks_review_queue;

-- Rollback only after human-review state has been removed or migrated.
UPDATE face_checks SET result = NULL
WHERE result IN ('pending_review', 'manual_approved', 'manual_rejected');

ALTER TABLE face_checks
    DROP CONSTRAINT IF EXISTS chk_face_checks_result;

ALTER TABLE face_checks
    ADD CONSTRAINT chk_face_checks_result CHECK (
        result IS NULL OR result IN ('pass', 'fail')
    );
