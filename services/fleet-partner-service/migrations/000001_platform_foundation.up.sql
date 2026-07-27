-- Platform foundation: transactional outbox, consumer inbox, durable jobs,
-- idempotency keys, append-only audit records and file references.
-- No table here references another microservice's database; external IDs are
-- stored as opaque text.

CREATE TABLE IF NOT EXISTS fleet_partner_outbox (
    id                  text PRIMARY KEY,
    event_type          text NOT NULL,
    schema_version      integer NOT NULL CHECK (schema_version >= 1),
    aggregate_type      text NOT NULL,
    aggregate_id        text NOT NULL,
    organization_id     text,
    payload             jsonb NOT NULL,
    headers             jsonb NOT NULL DEFAULT '{}'::jsonb,
    status              text NOT NULL DEFAULT 'PENDING'
                        CHECK (status IN ('PENDING', 'PUBLISHED', 'DEAD_LETTERED')),
    available_at        timestamptz NOT NULL DEFAULT now(),
    attempts            integer NOT NULL DEFAULT 0,
    lease_owner         text,
    lease_expires_at    timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    published_at        timestamptz,
    last_error_redacted text
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_outbox_pending
    ON fleet_partner_outbox (available_at)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_fleet_partner_outbox_organization
    ON fleet_partner_outbox (organization_id, created_at);

CREATE TABLE IF NOT EXISTS fleet_partner_inbox (
    consumer_name  text NOT NULL,
    event_id       text NOT NULL,
    event_type     text NOT NULL,
    schema_version integer NOT NULL CHECK (schema_version >= 1),
    payload_hash   text NOT NULL,
    received_at    timestamptz NOT NULL DEFAULT now(),
    processed_at   timestamptz,
    status         text NOT NULL DEFAULT 'PROCESSING'
                   CHECK (status IN ('PROCESSING', 'PROCESSED', 'FAILED')),
    error_code     text,
    PRIMARY KEY (consumer_name, event_id)
);

CREATE TABLE IF NOT EXISTS fleet_partner_jobs (
    id                  text PRIMARY KEY,
    organization_id     text,
    job_type            text NOT NULL,
    dedupe_key          text,
    payload             jsonb NOT NULL DEFAULT 'null'::jsonb,
    status              text NOT NULL DEFAULT 'PENDING'
                        CHECK (status IN ('PENDING', 'RETRY_SCHEDULED', 'CLAIMED', 'RUNNING',
                                          'SUCCEEDED', 'FAILED', 'DEAD_LETTERED', 'CANCELLED')),
    priority            integer NOT NULL DEFAULT 0,
    scheduled_at        timestamptz NOT NULL DEFAULT now(),
    attempts            integer NOT NULL DEFAULT 0,
    max_attempts        integer NOT NULL DEFAULT 5 CHECK (max_attempts >= 1),
    lease_owner         text,
    lease_expires_at    timestamptz,
    heartbeat_at        timestamptz,
    last_error_redacted text,
    created_at          timestamptz NOT NULL DEFAULT now(),
    completed_at        timestamptz
);

-- Dedupe is organization-scoped: the same dedupe key may be active in two
-- different organizations without colliding. COALESCE folds platform-scoped
-- jobs (NULL organization) into their own scope.
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_partner_jobs_active_dedupe
    ON fleet_partner_jobs (COALESCE(organization_id, ''), job_type, dedupe_key)
    WHERE dedupe_key IS NOT NULL AND status IN ('PENDING','RETRY_SCHEDULED','CLAIMED','RUNNING');

CREATE INDEX IF NOT EXISTS idx_fleet_partner_jobs_claimable
    ON fleet_partner_jobs (priority DESC, scheduled_at)
    WHERE status IN ('PENDING', 'RETRY_SCHEDULED');

CREATE INDEX IF NOT EXISTS idx_fleet_partner_jobs_organization
    ON fleet_partner_jobs (organization_id, created_at);

CREATE TABLE IF NOT EXISTS fleet_partner_idempotency_keys (
    organization_id     text NOT NULL,
    endpoint            text NOT NULL,
    idempotency_key     text NOT NULL,
    request_fingerprint text NOT NULL,
    status              text NOT NULL CHECK (status IN ('IN_PROGRESS', 'COMPLETED')),
    response_status     integer,
    response_body       bytea,
    created_at          timestamptz NOT NULL DEFAULT now(),
    expires_at          timestamptz NOT NULL,
    PRIMARY KEY (organization_id, endpoint, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_idempotency_expiry
    ON fleet_partner_idempotency_keys (expires_at);

CREATE TABLE IF NOT EXISTS fleet_partner_audit_records (
    id               text PRIMARY KEY,
    organization_id  text NOT NULL,
    branch_id        text,
    actor_type       text NOT NULL,
    actor_id         text NOT NULL,
    action           text NOT NULL,
    resource_type    text NOT NULL,
    resource_id      text NOT NULL,
    previous_summary jsonb,
    new_summary      jsonb,
    reason           text,
    request_id       text NOT NULL,
    correlation_id   text NOT NULL,
    network_meta     jsonb,
    occurred_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_audit_org_time
    ON fleet_partner_audit_records (organization_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_audit_resource
    ON fleet_partner_audit_records (organization_id, resource_type, resource_id);

-- Append-only enforcement: no UPDATE or DELETE on audit records.
CREATE OR REPLACE FUNCTION fleet_partner_forbid_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'table % is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_fleet_partner_audit_append_only ON fleet_partner_audit_records;
CREATE TRIGGER trg_fleet_partner_audit_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_audit_records
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- File references: metadata only. Binary data and permanent public URLs are
-- never stored; media-file-service owns the files.
CREATE TABLE IF NOT EXISTS fleet_partner_file_references (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    file_id         text NOT NULL,
    purpose         text NOT NULL,
    subject_type    text NOT NULL,
    subject_id      text NOT NULL,
    retention_class text NOT NULL DEFAULT 'STANDARD',
    source_service  text NOT NULL DEFAULT 'media-file-service',
    source_status   text NOT NULL DEFAULT 'UNSYNCED',
    synced_at       timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_file_refs_subject
    ON fleet_partner_file_references (organization_id, subject_type, subject_id);
