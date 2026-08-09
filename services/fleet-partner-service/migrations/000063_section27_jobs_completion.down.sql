DELETE FROM fleet_partner_role_permissions WHERE permission_code IN ('jobs.read','jobs.manage','jobs.retry','jobs.cancel');
DELETE FROM fleet_partner_permissions WHERE code IN ('jobs.read','jobs.manage','jobs.retry','jobs.cancel');

ALTER TABLE fleet_partner_job_attempts
    DROP COLUMN IF EXISTS next_attempt_at,
    DROP COLUMN IF EXISTS authoritative_readback_result,
    DROP COLUMN IF EXISTS external_response_reference,
    DROP COLUMN IF EXISTS external_request_reference,
    DROP COLUMN IF EXISTS error_code,
    DROP COLUMN IF EXISTS error_class,
    DROP COLUMN IF EXISTS heartbeat_at;

ALTER TABLE fleet_partner_job_attempts
    DROP CONSTRAINT IF EXISTS fleet_partner_job_attempts_status_check_v2,
    ADD CONSTRAINT fleet_partner_job_attempts_status_check
        CHECK (status IN ('RUNNING','SUCCEEDED','FAILED','DEAD_LETTERED','LEASE_LOST'));

DROP INDEX IF EXISTS idx_fleet_partner_jobs_dead_letter;
DROP INDEX IF EXISTS idx_fleet_partner_jobs_branch;
DROP INDEX IF EXISTS idx_fleet_partner_jobs_scheduled_occurrence;
DROP INDEX IF EXISTS idx_fleet_partner_jobs_active_idempotency;

ALTER TABLE fleet_partner_jobs
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS authoritative_readback_required,
    DROP COLUMN IF EXISTS external_outcome_status,
    DROP COLUMN IF EXISTS last_safe_error_message,
    DROP COLUMN IF EXISTS last_error_code,
    DROP COLUMN IF EXISTS last_error_class,
    DROP COLUMN IF EXISTS dead_lettered_at,
    DROP COLUMN IF EXISTS cancelled_at,
    DROP COLUMN IF EXISTS started_at,
    DROP COLUMN IF EXISTS next_attempt_at,
    DROP COLUMN IF EXISTS scheduled_for,
    DROP COLUMN IF EXISTS scheduled_task_id,
    DROP COLUMN IF EXISTS causation_id,
    DROP COLUMN IF EXISTS correlation_id,
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS payload_version,
    DROP COLUMN IF EXISTS branch_id;
