-- Sections 6-7 foundation: persisted operating mode, first-class
-- compensation plans, completed-work projection, deterministic compensation
-- resolution and immutable settlement instructions.
-- SECTION30-CLEANUP-RELEASE: additive section7 foundation; down migration drops only the new, unreleased tables/columns introduced here.

ALTER TABLE fleet_partner_organization_settings
    ADD COLUMN IF NOT EXISTS operating_mode text NOT NULL DEFAULT 'CENTRALIZED'
        CHECK (operating_mode IN ('CENTRALIZED', 'DELEGATED', 'HYBRID')),
    ADD COLUMN IF NOT EXISTS financial_visibility_policy jsonb NOT NULL DEFAULT '{}'::jsonb;

UPDATE fleet_partner_organization_settings
SET operating_mode = 'CENTRALIZED'
WHERE operating_mode IS NULL OR operating_mode = '';

CREATE TABLE IF NOT EXISTS fleet_partner_compensation_plans (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    name            text NOT NULL,
    description     text NOT NULL DEFAULT '',
    currency        text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    status          text NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','ACTIVE','INACTIVE','ARCHIVED')),
    version         bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    effective_from  timestamptz NOT NULL,
    effective_to    timestamptz,
    created_by      text NOT NULL,
    approved_by     text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE INDEX IF NOT EXISTS idx_fleet_comp_plans_org_status
    ON fleet_partner_compensation_plans(organization_id, status, currency, effective_from);

CREATE TABLE IF NOT EXISTS fleet_partner_compensation_plan_versions (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    plan_id         text NOT NULL REFERENCES fleet_partner_compensation_plans(id) ON DELETE RESTRICT,
    plan_version    bigint NOT NULL CHECK (plan_version >= 1),
    snapshot        jsonb NOT NULL,
    actor_reference text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(plan_id, plan_version)
);

CREATE TRIGGER trg_fleet_comp_plan_versions_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_compensation_plan_versions
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE IF NOT EXISTS fleet_partner_compensation_plan_rules (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    plan_id         text NOT NULL REFERENCES fleet_partner_compensation_plans(id) ON DELETE RESTRICT,
    plan_version    bigint NOT NULL CHECK (plan_version >= 1),
    rule_type       text NOT NULL CHECK(rule_type IN('SALARY','PER_JOB','PER_SHIFT','COMMISSION','ALLOWANCE','OVERTIME','BONUS','DEDUCTION')),
    applicable_roles jsonb NOT NULL DEFAULT '[]'::jsonb,
    applicable_services jsonb NOT NULL DEFAULT '[]'::jsonb,
    calculation_configuration jsonb NOT NULL,
    status          text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN('DRAFT','ACTIVE','INACTIVE','ARCHIVED')),
    effective_from  timestamptz NOT NULL,
    effective_to    timestamptz,
    version         bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (effective_to IS NULL OR effective_to > effective_from),
    CHECK (rule_type <> 'COMMISSION' OR ((calculation_configuration ? 'rateBps') AND ((calculation_configuration->>'rateBps')::int BETWEEN 0 AND 10000) AND (calculation_configuration ? 'basis'))),
    CHECK (rule_type NOT IN ('SALARY','PER_JOB','PER_SHIFT') OR ((calculation_configuration ? 'amountMinor') AND (calculation_configuration ? 'currency'))),
    CHECK (rule_type <> 'SALARY' OR ((calculation_configuration ? 'cadence') AND calculation_configuration->>'cadence' IN ('DAILY','WEEKLY','MONTHLY')))
);

CREATE INDEX IF NOT EXISTS idx_fleet_comp_plan_rules_effective
    ON fleet_partner_compensation_plan_rules(organization_id, plan_id, plan_version, status, effective_from, effective_to);

CREATE TABLE IF NOT EXISTS fleet_partner_membership_compensation_assignments (
    id              text PRIMARY KEY,
    membership_id   text NOT NULL REFERENCES fleet_partner_organization_provider_memberships(id) ON DELETE RESTRICT,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    compensation_plan_id text NOT NULL REFERENCES fleet_partner_compensation_plans(id) ON DELETE RESTRICT,
    compensation_plan_version bigint NOT NULL CHECK (compensation_plan_version >= 1),
    service_context text NOT NULL DEFAULT '*' CHECK (service_context IN ('*','TAXI','DISPATCH','HAULING')),
    currency        text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    effective_from  timestamptz NOT NULL,
    effective_to    timestamptz,
    status          text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN('DRAFT','ACTIVE','INACTIVE','ARCHIVED')),
    assigned_by     text NOT NULL,
    approved_by     text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE INDEX IF NOT EXISTS idx_fleet_member_comp_assignment_resolve
    ON fleet_partner_membership_compensation_assignments(membership_id, organization_id, status, service_context, currency, effective_from, effective_to);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_member_comp_assignment_open
    ON fleet_partner_membership_compensation_assignments(membership_id, organization_id, service_context, currency)
    WHERE status = 'ACTIVE' AND effective_to IS NULL;

CREATE TABLE IF NOT EXISTS fleet_partner_work_sessions (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    membership_id   text NOT NULL REFERENCES fleet_partner_organization_provider_memberships(id) ON DELETE RESTRICT,
    source_service  text NOT NULL DEFAULT '' CHECK (source_service IN ('','TAXI','DISPATCH','HAULING')),
    started_at      timestamptz NOT NULL,
    ended_at        timestamptz,
    status          text NOT NULL DEFAULT 'SCHEDULED' CHECK(status IN('SCHEDULED','ACTIVE','COMPLETED','APPROVED','REJECTED','CANCELLED')),
    approved_at     timestamptz,
    approved_by     text NOT NULL DEFAULT '',
    version         bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (ended_at IS NULL OR ended_at >= started_at)
);

CREATE INDEX IF NOT EXISTS idx_fleet_work_sessions_member
    ON fleet_partner_work_sessions(organization_id, membership_id, status, started_at);

CREATE TABLE IF NOT EXISTS fleet_partner_completed_work_records (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    membership_id   text NOT NULL REFERENCES fleet_partner_organization_provider_memberships(id) ON DELETE RESTRICT,
    provider_id     text NOT NULL,
    source_service  text NOT NULL CHECK (source_service IN ('TAXI','DISPATCH','HAULING','FLEET','VENDOR')),
    source_job_id   text NOT NULL,
    source_assignment_id text NOT NULL DEFAULT '',
    work_type       text NOT NULL DEFAULT 'MANAGED_WORK',
    completed_at    timestamptz NOT NULL,
    currency        text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    eligible_basis_minor bigint NOT NULL CHECK (eligible_basis_minor >= 0),
    source_version  bigint NOT NULL CHECK (source_version >= 1),
    ingestion_status text NOT NULL DEFAULT 'RECORDED' CHECK (ingestion_status IN ('RECORDED','DUPLICATE','RECONCILIATION_REQUIRED','INVALID')),
    request_fingerprint text NOT NULL,
    idempotency_key text NOT NULL,
    correlation_id  text NOT NULL DEFAULT '',
    version         bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(source_service, source_job_id, membership_id, source_version),
    UNIQUE(organization_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_fleet_completed_work_reconcile
    ON fleet_partner_completed_work_records(organization_id, ingestion_status, created_at);

CREATE TABLE IF NOT EXISTS fleet_partner_compensation_resolutions (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    completed_work_id text NOT NULL REFERENCES fleet_partner_completed_work_records(id) ON DELETE RESTRICT,
    membership_id   text NOT NULL,
    compensation_plan_id text NOT NULL,
    compensation_plan_version bigint NOT NULL,
    matched_rules   jsonb NOT NULL DEFAULT '[]'::jsonb,
    provider_share_minor bigint NOT NULL CHECK (provider_share_minor >= 0),
    organization_share_minor bigint NOT NULL CHECK (organization_share_minor >= 0),
    currency        text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    calculation_basis jsonb NOT NULL DEFAULT '{}'::jsonb,
    visibility_policy_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    resolved_at     timestamptz NOT NULL DEFAULT now(),
    resolution_version bigint NOT NULL DEFAULT 1 CHECK (resolution_version >= 1),
    status          text NOT NULL DEFAULT 'RESOLVED' CHECK(status IN('RESOLVED','RECONCILIATION_REQUIRED')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(completed_work_id, resolution_version)
);

CREATE TABLE IF NOT EXISTS fleet_partner_managed_work_settlement_instructions (
    instruction_id  text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    membership_id   text NOT NULL,
    provider_id     text NOT NULL,
    source_service  text NOT NULL CHECK (source_service IN ('TAXI','DISPATCH','HAULING','FLEET','VENDOR')),
    source_job_id   text NOT NULL,
    source_assignment_id text NOT NULL DEFAULT '',
    compensation_plan_id text NOT NULL,
    compensation_plan_version bigint NOT NULL,
    compensation_resolution_version bigint NOT NULL,
    gross_basis_minor bigint NOT NULL CHECK (gross_basis_minor >= 0),
    provider_share_minor bigint NOT NULL CHECK (provider_share_minor >= 0),
    organization_share_minor bigint NOT NULL CHECK (organization_share_minor >= 0),
    platform_share_minor bigint,
    currency        text NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    visibility_policy_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    status          text NOT NULL DEFAULT 'CREATED' CHECK(status IN('CREATED','PENDING_WALLET','RECONCILIATION_REQUIRED','SUPERSEDED','CANCELLED')),
    idempotency_key text NOT NULL,
    request_fingerprint text NOT NULL,
    version         bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(source_service, source_job_id, membership_id, compensation_resolution_version),
    UNIQUE(organization_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_fleet_settlement_instruction_status
    ON fleet_partner_managed_work_settlement_instructions(organization_id, status, created_at);

CREATE TABLE IF NOT EXISTS fleet_partner_compensation_adjustments (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    membership_id   text NOT NULL REFERENCES fleet_partner_organization_provider_memberships(id) ON DELETE RESTRICT,
    adjustment_type text NOT NULL CHECK(adjustment_type IN('ALLOWANCE','OVERTIME','BONUS','DEDUCTION')),
    reason          text NOT NULL,
    amount_minor    bigint NOT NULL CHECK(amount_minor >= 0),
    currency        text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
    source          text NOT NULL,
    payroll_period_id text NOT NULL DEFAULT '',
    status          text NOT NULL DEFAULT 'PENDING_APPROVAL' CHECK(status IN('PENDING_APPROVAL','APPROVED','REJECTED','POSTED','REVERSED')),
    approved_by     text NOT NULL DEFAULT '',
    approved_at     timestamptz,
    audit_data      jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_comp_adjustments_member
    ON fleet_partner_compensation_adjustments(organization_id, membership_id, status, created_at);

CREATE TABLE IF NOT EXISTS fleet_partner_reconciliation_items (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
    item_type       text NOT NULL CHECK(item_type IN('COMPLETED_WORK','COMPENSATION_RESOLUTION','SETTLEMENT_INSTRUCTION')),
    item_id         text NOT NULL,
    reason_code     text NOT NULL CHECK(reason_code IN('COMPENSATION_NOT_RESOLVED','SETTLEMENT_NOT_CREATED','DUPLICATE_SOURCE_EVENT','STALE_ASSIGNMENT_REFERENCE','INVALID_MEMBERSHIP_BINDING','COMPENSATION_PLAN_MISSING','DOWNSTREAM_SETTLEMENT_PENDING')),
    status          text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','IN_PROGRESS','RESOLVED','FAILED_PERMANENT')),
    last_error_code text NOT NULL DEFAULT '',
    correlation_id  text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(item_type, item_id, reason_code)
);

INSERT INTO fleet_partner_permissions(code, category, description, is_active) VALUES
('fleet_partner_foundation.read','section7','Read Fleet partner operating, compensation and settlement foundation records',true),
('fleet_partner_foundation.manage','section7','Manage Fleet partner operating mode and compensation foundation records',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category, description=excluded.description, is_active=true;
