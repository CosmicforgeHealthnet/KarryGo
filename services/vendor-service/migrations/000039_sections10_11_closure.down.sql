DROP TABLE IF EXISTS pastry_change_request_history;
ALTER TABLE pastry_change_requests
    DROP COLUMN IF EXISTS decided_at,
    DROP COLUMN IF EXISTS decided_by,
    DROP COLUMN IF EXISTS decision_note;
ALTER TABLE cloud_kitchen_occurrences
    DROP COLUMN IF EXISTS lease_owner,
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS attempt_count,
    DROP COLUMN IF EXISTS replay_count,
    DROP COLUMN IF EXISTS dead_lettered_at,
    DROP COLUMN IF EXISTS last_error,
    DROP COLUMN IF EXISTS production_job_id;
ALTER TABLE production_jobs DROP COLUMN IF EXISTS risk_flags;
