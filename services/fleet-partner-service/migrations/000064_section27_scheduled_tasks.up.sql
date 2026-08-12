-- Section 27: scheduled-task infrastructure. A scheduled task describes WHEN
-- and WHAT to enqueue; it never executes domain work itself -- the scheduler
-- creates a durable fleet_partner_jobs row (via the existing
-- scheduled_task_id + scheduled_for uniqueness added in migration 000063),
-- and the existing jobs.Worker performs the work under its own lease.

CREATE TABLE IF NOT EXISTS fleet_partner_scheduled_tasks (
    id                   text PRIMARY KEY,
    organization_id      text,
    branch_id            text,
    task_type            text NOT NULL,
    job_type             text NOT NULL,
    schedule_type        text NOT NULL CHECK (schedule_type IN ('CRON', 'INTERVAL', 'ONCE')),
    -- CRON: a standard 5-field cron expression. INTERVAL: an integer number
    -- of seconds. ONCE: an RFC3339 timestamp in the task's own timezone.
    schedule_expression  text NOT NULL,
    timezone             text NOT NULL DEFAULT 'UTC',
    payload              jsonb NOT NULL DEFAULT '{}'::jsonb,
    payload_version      integer NOT NULL DEFAULT 1 CHECK (payload_version >= 1),
    enabled              boolean NOT NULL DEFAULT true,
    next_run_at          timestamptz,
    last_scheduled_at    timestamptz,
    last_started_at      timestamptz,
    last_completed_at    timestamptz,
    last_result          text NOT NULL DEFAULT '',
    failure_count        integer NOT NULL DEFAULT 0,
    concurrency_policy   text NOT NULL DEFAULT 'FORBID'
                         CHECK (concurrency_policy IN ('ALLOW', 'FORBID', 'REPLACE')),
    misfire_policy       text NOT NULL DEFAULT 'SKIP'
                         CHECK (misfire_policy IN ('SKIP', 'RUN_ONCE', 'CATCH_UP_LIMITED')),
    max_catch_up         integer NOT NULL DEFAULT 1 CHECK (max_catch_up >= 1),
    max_attempts         integer NOT NULL DEFAULT 5 CHECK (max_attempts >= 1),
    priority              integer NOT NULL DEFAULT 0,
    lease_owner          text,
    lease_expires_at     timestamptz,
    created_by           text NOT NULL DEFAULT '',
    updated_by           text NOT NULL DEFAULT '',
    version              bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    archived_at          timestamptz
);

CREATE INDEX IF NOT EXISTS idx_fleet_scheduled_tasks_due
    ON fleet_partner_scheduled_tasks (next_run_at)
    WHERE enabled = true AND archived_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_scheduled_tasks_organization
    ON fleet_partner_scheduled_tasks (organization_id, created_at DESC);

-- One durable, append-only record per scheduler decision (enqueued,
-- skipped-forbid, skipped-misfire, failed-to-enqueue) -- the schedule
-- history the specification asks be preserved, independent of the jobs the
-- scheduler goes on to create.
CREATE TABLE IF NOT EXISTS fleet_partner_scheduled_task_runs (
    id                text PRIMARY KEY,
    scheduled_task_id text NOT NULL REFERENCES fleet_partner_scheduled_tasks (id) ON DELETE RESTRICT,
    organization_id   text,
    scheduled_for     timestamptz NOT NULL,
    outcome           text NOT NULL
                      CHECK (outcome IN ('ENQUEUED', 'SKIPPED_FORBID', 'SKIPPED_MISFIRE',
                                         'REPLACED_PRIOR', 'ENQUEUE_FAILED')),
    job_id            text,
    safe_error_code   text NOT NULL DEFAULT '',
    occurred_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_scheduled_task_runs_task
    ON fleet_partner_scheduled_task_runs (scheduled_task_id, occurred_at DESC);

CREATE TRIGGER trg_fleet_scheduled_task_runs_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_scheduled_task_runs
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('scheduled_tasks.read', 'scheduled_tasks', 'Read scheduled background tasks', true),
    ('scheduled_tasks.manage', 'scheduled_tasks', 'Create, update, enable or disable scheduled tasks', true),
    ('scheduled_tasks.run', 'scheduled_tasks', 'Manually run a scheduled task now', true),
    ('worker_operations.read', 'worker_operations', 'Read worker/job operational state', true),
    ('worker_operations.manage', 'worker_operations', 'Manage worker/job operational controls', true)
ON CONFLICT (code) DO UPDATE SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM fleet_partner_roles r CROSS JOIN fleet_partner_permissions p
WHERE r.id IN ('rol_system_owner', 'rol_system_admin', 'rol_system_operations')
  AND p.category IN ('scheduled_tasks', 'worker_operations')
ON CONFLICT DO NOTHING;
