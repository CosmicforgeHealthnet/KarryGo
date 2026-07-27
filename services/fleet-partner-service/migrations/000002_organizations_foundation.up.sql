-- Foundational organization schema: organizations, branches, departments,
-- settings and append-only status history.
-- Local same-service relations use foreign keys with ON DELETE RESTRICT.
-- No foreign key references another microservice's database.

CREATE TABLE IF NOT EXISTS fleet_partner_organizations (
    id                     text PRIMARY KEY,
    status                 text NOT NULL DEFAULT 'PENDING_REVIEW'
                           CHECK (status IN ('PENDING_REVIEW', 'ACTIVE', 'SUSPENDED', 'ARCHIVED')),
    legal_name             text NOT NULL,
    trading_name           text,
    registration_number    text NOT NULL,
    country_code           text NOT NULL CHECK (char_length(country_code) = 2),
    timezone               text NOT NULL,
    default_currency       text NOT NULL CHECK (default_currency ~ '^[A-Z]{3}$'),
    operating_capabilities jsonb NOT NULL DEFAULT '[]'::jsonb,
    version                bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    archived_at            timestamptz,
    -- Registration numbers are unique within a country (confirmed business
    -- rule pending; country-scoped is the safe additive default).
    CONSTRAINT uq_fleet_partner_org_registration UNIQUE (country_code, registration_number)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_organizations_status
    ON fleet_partner_organizations (status);

CREATE TABLE IF NOT EXISTS fleet_partner_branches (
    id                 text PRIMARY KEY,
    organization_id    text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    code               text NOT NULL,
    name               text NOT NULL,
    type               text NOT NULL DEFAULT 'BRANCH'
                       CHECK (type IN ('BRANCH', 'HUB', 'DEPOT', 'WAREHOUSE', 'OFFICE')),
    address            text NOT NULL DEFAULT '',
    latitude           double precision,
    longitude          double precision,
    geofence_reference text,
    operating_hours    text,
    timezone           text NOT NULL,
    status             text NOT NULL DEFAULT 'ACTIVE'
                       CHECK (status IN ('ACTIVE', 'INACTIVE', 'ARCHIVED')),
    version            bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    archived_at        timestamptz,
    CONSTRAINT uq_fleet_partner_branch_code UNIQUE (organization_id, code)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_branches_org
    ON fleet_partner_branches (organization_id, status);

CREATE TABLE IF NOT EXISTS fleet_partner_departments (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id       text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    parent_id       text REFERENCES fleet_partner_departments (id) ON DELETE RESTRICT,
    cost_centre_id  text,
    code            text NOT NULL,
    name            text NOT NULL,
    status          text NOT NULL DEFAULT 'ACTIVE'
                    CHECK (status IN ('ACTIVE', 'ARCHIVED')),
    version         bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    archived_at     timestamptz,
    CONSTRAINT uq_fleet_partner_department_code UNIQUE (organization_id, code),
    CONSTRAINT chk_fleet_partner_department_no_self_parent CHECK (parent_id IS NULL OR parent_id <> id)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_departments_org
    ON fleet_partner_departments (organization_id, status);

CREATE TABLE IF NOT EXISTS fleet_partner_organization_settings (
    organization_id            text PRIMARY KEY REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    assignment_timeout_seconds integer NOT NULL DEFAULT 300
                               CHECK (assignment_timeout_seconds BETWEEN 30 AND 3600),
    approval_thresholds        jsonb NOT NULL DEFAULT '{}'::jsonb,
    provider_visibility        text NOT NULL DEFAULT 'ORGANIZATION'
                               CHECK (provider_visibility IN ('ORGANIZATION', 'BRANCH')),
    retention_settings         jsonb NOT NULL DEFAULT '{}'::jsonb,
    feature_flags              jsonb NOT NULL DEFAULT '{}'::jsonb,
    version                    bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at                 timestamptz NOT NULL DEFAULT now(),
    updated_at                 timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fleet_partner_organization_status_history (
    id                text PRIMARY KEY,
    organization_id   text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    previous_status   text NOT NULL,
    new_status        text NOT NULL,
    actor_type        text NOT NULL,
    actor_id          text NOT NULL,
    reason_code       text NOT NULL,
    reason_text       text,
    evidence_file_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    request_id        text NOT NULL,
    correlation_id    text NOT NULL,
    occurred_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_org_status_history
    ON fleet_partner_organization_status_history (organization_id, occurred_at DESC);

-- Status history is append-only (reuses the trigger function from 000001).
DROP TRIGGER IF EXISTS trg_fleet_partner_org_status_history_append_only ON fleet_partner_organization_status_history;
CREATE TRIGGER trg_fleet_partner_org_status_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_organization_status_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
