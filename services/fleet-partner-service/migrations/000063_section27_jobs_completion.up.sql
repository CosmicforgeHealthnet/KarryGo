-- Section 27: complete the canonical durable-jobs schema additively.
--
-- fleet_partner_jobs (migration 000001) and fleet_partner_job_attempts
-- (migration 000047) already carry the core lease/heartbeat/attempt model
-- real workers across Sections 13-23 use today via internal/jobs.Queue and
-- internal/jobs.Worker. This migration adds the columns the Section 27
-- specification names that were not yet present -- branch scope, payload
-- versioning, correlation/causation, scheduled-task origin, richer
-- timestamps, structured error classification, and unknown-external-outcome
-- tracking -- without renaming or removing anything an existing caller
-- depends on. dedupe_key remains the job-level idempotency identity;
-- idempotency_key is added as an explicit alias column so a caller that
-- enqueues with a caller-supplied idempotency key (as opposed to a
-- worker-derived dedupe key) has a field named for what it is, without a
-- second, competing uniqueness mechanism -- the existing dedupe unique index
-- already covers both by column position once both are populated from the
-- same value at the call site.

ALTER TABLE fleet_partner_jobs
    ADD COLUMN IF NOT EXISTS branch_id text,
    ADD COLUMN IF NOT EXISTS payload_version integer NOT NULL DEFAULT 1 CHECK (payload_version >= 1),
    ADD COLUMN IF NOT EXISTS idempotency_key text,
    ADD COLUMN IF NOT EXISTS correlation_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS causation_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS scheduled_task_id text,
    ADD COLUMN IF NOT EXISTS scheduled_for timestamptz,
    ADD COLUMN IF NOT EXISTS next_attempt_at timestamptz,
    ADD COLUMN IF NOT EXISTS started_at timestamptz,
    ADD COLUMN IF NOT EXISTS cancelled_at timestamptz,
    ADD COLUMN IF NOT EXISTS dead_lettered_at timestamptz,
    ADD COLUMN IF NOT EXISTS last_error_class text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_error_code text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_safe_error_message text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS external_outcome_status text NOT NULL DEFAULT ''
        CHECK (external_outcome_status IN ('', 'UNKNOWN', 'CONFIRMED_SUCCEEDED', 'CONFIRMED_FAILED')),
    ADD COLUMN IF NOT EXISTS authoritative_readback_required boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1 CHECK (version >= 1);

-- The existing dedupe unique index only fires when dedupe_key is populated.
-- A caller-supplied idempotency_key deserves the identical guarantee under
-- its own name, scoped the same way (organization + job type).
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_partner_jobs_active_idempotency
    ON fleet_partner_jobs (COALESCE(organization_id, ''), job_type, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND status IN ('PENDING','RETRY_SCHEDULED','CLAIMED','RUNNING');

-- One durable job per scheduled-task occurrence: scheduled_task_id +
-- scheduled_for is the stable identity the scheduler's own idempotent
-- enqueue relies on so two scheduler instances can never create two jobs
-- for the same due occurrence.
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_partner_jobs_scheduled_occurrence
    ON fleet_partner_jobs (scheduled_task_id, scheduled_for)
    WHERE scheduled_task_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_jobs_branch
    ON fleet_partner_jobs (organization_id, branch_id)
    WHERE branch_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_jobs_dead_letter
    ON fleet_partner_jobs (job_type, dead_lettered_at DESC)
    WHERE status = 'DEAD_LETTERED';

-- job_attempts: widen the outcome vocabulary additively (the original
-- 000047 CHECK only covered RUNNING/SUCCEEDED/FAILED/DEAD_LETTERED/
-- LEASE_LOST) and add the remaining Section 27 attempt fields. The table
-- stays append-only (trg_fleet_job_attempts_append_only, unchanged).
DO $$
DECLARE
    old_check text;
BEGIN
    SELECT con.conname INTO old_check
    FROM pg_constraint con
    JOIN pg_class rel ON rel.oid = con.conrelid
    WHERE rel.relname = 'fleet_partner_job_attempts'
      AND con.contype = 'c'
      AND pg_get_constraintdef(con.oid) ILIKE '%status%'
      AND pg_get_constraintdef(con.oid) ILIKE '%SUCCEEDED%';
    IF old_check IS NOT NULL THEN
        EXECUTE 'ALTER TABLE fleet_partner_job_attempts DROP CONSTRAINT ' || quote_ident(old_check);
    END IF;
END $$;

ALTER TABLE fleet_partner_job_attempts
    ADD CONSTRAINT fleet_partner_job_attempts_status_check_v2
    CHECK (status IN ('RUNNING', 'SUCCEEDED', 'FAILED', 'DEAD_LETTERED', 'LEASE_LOST',
                       'TRANSIENT_FAILURE', 'PERMANENT_FAILURE', 'UNKNOWN_EXTERNAL_OUTCOME',
                       'CANCELLED', 'SHUTDOWN_RELEASED'));

ALTER TABLE fleet_partner_job_attempts
    ADD COLUMN IF NOT EXISTS heartbeat_at timestamptz,
    ADD COLUMN IF NOT EXISTS error_class text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS error_code text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS external_request_reference text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS external_response_reference text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS authoritative_readback_result text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS next_attempt_at timestamptz;

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('jobs.read', 'jobs', 'Read durable background jobs', true),
    ('jobs.manage', 'jobs', 'Manage durable background jobs', true),
    ('jobs.retry', 'jobs', 'Retry or replay a dead-lettered job', true),
    ('jobs.cancel', 'jobs', 'Cancel a pending or running job', true)
ON CONFLICT (code) DO UPDATE SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM fleet_partner_roles r CROSS JOIN fleet_partner_permissions p
WHERE r.id IN ('rol_system_owner', 'rol_system_admin', 'rol_system_operations') AND p.category = 'jobs'
ON CONFLICT DO NOTHING;
