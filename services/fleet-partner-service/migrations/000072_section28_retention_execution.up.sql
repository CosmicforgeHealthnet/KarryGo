-- Section 28: executable retention. internal/features/retentionpolicy
-- (migration 000068) already stores each organization's configured
-- retention_days per class, bounded by a platform minimum -- but nothing
-- ever ACTED on that configuration; it was pure configuration with no
-- execution behind it (disclosed as such in
-- docs/section28-security-privacy-audit.md's original "Privacy and
-- retention policy" section). This migration adds the durable-job-backed
-- (internal/jobs, Section 27's own framework -- no second job system) real
-- execution engine, scoped to a small, explicitly safe allowlist of record
-- classes -- see internal/platform/retentionexec's package doc for exactly
-- which, and why every immutable/evidentiary table this schema owns
-- (fleet_partner_audit_records, fleet_partner_job_attempts,
-- fleet_partner_scheduled_task_runs, and every financial/trip-execution/
-- safety/incident/compliance table) is categorically excluded, not merely
-- unimplemented.
--
-- fleet_partner_retention_runs is one INITIATED pass over one
-- (organization, retention class, mode) triple. fleet_partner_
-- retention_run_records is one IMMUTABLE outcome per candidate record a run
-- actually considered -- append-only in the same spirit as
-- fleet_partner_audit_records (never updated after insert; a record that
-- needs re-considering gets a NEW row in a later run, not an edited old
-- one), though it does not share that table's hash-chain machinery: these
-- rows record retention DECISIONS about other tables, not authorization
-- actions, and are themselves covered by the ordinary audit log
-- (retention.run.initiated / retention.record.processed entries) for
-- tamper-evidence of the DECISION to run retention at all.
CREATE TABLE IF NOT EXISTS fleet_partner_retention_runs (
    id                text PRIMARY KEY,
    organization_id   text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    retention_class   text NOT NULL,
    mode              text NOT NULL CHECK (mode IN ('DRY_RUN', 'EXECUTE')),
    status            text NOT NULL DEFAULT 'PENDING'
                      CHECK (status IN ('PENDING', 'RUNNING', 'COMPLETED', 'FAILED')),
    job_id            text,
    initiated_by      text NOT NULL,
    idempotency_key   text NOT NULL,
    records_considered   integer NOT NULL DEFAULT 0,
    records_archived     integer NOT NULL DEFAULT 0,
    records_anonymized   integer NOT NULL DEFAULT 0,
    records_skipped      integer NOT NULL DEFAULT 0,
    records_failed       integer NOT NULL DEFAULT 0,
    error_message     text,
    started_at        timestamptz,
    completed_at      timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_retention_runs_org
    ON fleet_partner_retention_runs (organization_id, retention_class, created_at DESC);

CREATE TABLE IF NOT EXISTS fleet_partner_retention_run_records (
    id                text PRIMARY KEY,
    run_id            text NOT NULL REFERENCES fleet_partner_retention_runs (id) ON DELETE RESTRICT,
    organization_id   text NOT NULL,
    record_type       text NOT NULL,
    record_id         text NOT NULL,
    outcome           text NOT NULL CHECK (outcome IN
                      ('ARCHIVED', 'ANONYMIZED', 'SKIPPED_MINIMUM_RETENTION', 'SKIPPED_LEGAL_HOLD',
                       'SKIPPED_IMMUTABLE', 'SKIPPED_ACTIVE_REFERENCE', 'FAILED')),
    detail            text NOT NULL DEFAULT '',
    processed_at      timestamptz NOT NULL DEFAULT now(),
    -- One outcome per (run, record) -- reprocessing the same run (a retried
    -- durable job after lease recovery) is idempotent: a record already
    -- decided in this run is skipped on replay, not decided twice.
    UNIQUE (run_id, record_type, record_id)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_retention_run_records_run
    ON fleet_partner_retention_run_records (run_id);

-- fleet_partner_forbid_mutation (migration 000001) intentionally does NOT
-- cover either table: fleet_partner_retention_runs' own status/counters are
-- legitimately updated in place as the run progresses (PENDING -> RUNNING
-- -> COMPLETED/FAILED, counters incrementing) -- it is a run's live
-- state, not a historical record of a completed action. Once a
-- fleet_partner_retention_run_records row is inserted, application code
-- never updates or deletes it (enforced by convention and the unique
-- constraint above, not a trigger): a mistake here is a business-logic bug
-- to fix in code, not a security boundary needing database-level
-- enforcement the way the audit log's tamper-resistance is -- retention
-- decisions are already independently visible through the audit log itself
-- (every run initiation and every EXECUTE-mode anonymize/archive action is
-- separately audited).

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('privacy.retention.execute', 'privacy', 'Initiate and preview retention execution runs', true)
ON CONFLICT (code) DO UPDATE SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT r.id, 'privacy.retention.execute' FROM fleet_partner_roles r
WHERE r.id IN ('rol_system_owner', 'rol_system_compliance')
ON CONFLICT DO NOTHING;
