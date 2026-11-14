-- Spec Sections 9, 37 and 38 (FLEET-9.12, FLEET-9.13, FLEET-8.15).
--
-- This EXTENDS the approval framework of 000006/000020 and the organization
-- structure of 000002. It creates no second approval system:
--
--   * fleet_partner_approval_policies gains the fields an organization-managed
--     policy needs (code, origin, description, authorship, idempotency). Rows
--     created on first use from the platform eligibility policy keep
--     origin = 'TEMPLATE' and behave exactly as before.
--   * fleet_partner_approval_policy_versions is the append-only history of
--     every published version of a policy, so the (policy_id, policy_version)
--     an approval request pins always resolves to the exact rules in force.
--   * approval requests record the amount (minor units + currency) that
--     selected a threshold policy.
--   * approval decisions record when they were cast under a delegation and on
--     whose authority.
--   * fleet_partner_approval_delegations (created in 000006, unused until
--     now) gains branch scope, reasons, revocation evidence and idempotency.
--   * teams, cost centres and operating locations join branches and
--     departments as organization structure; departments.cost_centre_id
--     becomes a real reference.

-- ---------------------------------------------------------------------------
-- Approval policies
-- ---------------------------------------------------------------------------

ALTER TABLE fleet_partner_approval_policies
    ADD COLUMN IF NOT EXISTS code text,
    ADD COLUMN IF NOT EXISTS origin text NOT NULL DEFAULT 'TEMPLATE',
    ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS created_by text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS updated_by text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS idempotency_key text,
    ADD COLUMN IF NOT EXISTS archived_at timestamptz;

ALTER TABLE fleet_partner_approval_policies
    ADD CONSTRAINT chk_fleet_partner_approval_policy_origin
        CHECK (origin IN ('TEMPLATE', 'ORGANIZATION')),
    -- An organization-managed policy is addressed by its code.
    ADD CONSTRAINT chk_fleet_partner_approval_policy_code
        CHECK (origin <> 'ORGANIZATION'
               OR (code IS NOT NULL AND code ~ '^[A-Z0-9][A-Z0-9_-]{0,63}$')),
    -- A branch-scoped policy names its branch; an organization-wide one does not.
    ADD CONSTRAINT chk_fleet_partner_approval_policy_scope
        CHECK ((scope_type = 'ORGANIZATION' AND scope_id IS NULL)
               OR (scope_type = 'BRANCH' AND scope_id IS NOT NULL)),
    -- Thresholds are whole minor units with an explicit currency, both or neither.
    ADD CONSTRAINT chk_fleet_partner_approval_policy_threshold
        CHECK ((threshold_amount_minor IS NULL) = (threshold_currency IS NULL)
               AND (threshold_amount_minor IS NULL OR threshold_amount_minor >= 0));

-- Codes are unique among the organization's current (ACTIVE) policies.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_approval_policy_code
    ON fleet_partner_approval_policies (organization_id, code)
    WHERE code IS NOT NULL AND status = 'ACTIVE';

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_approval_policy_idempotency
    ON fleet_partner_approval_policies (organization_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- Append-only version history. A request pins (policy_id, policy_version);
-- this table is what makes that pin mean a fixed set of rules.
CREATE TABLE IF NOT EXISTS fleet_partner_approval_policy_versions (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    policy_id       text NOT NULL REFERENCES fleet_partner_approval_policies (id) ON DELETE RESTRICT,
    version         bigint NOT NULL CHECK (version >= 1),
    status          text NOT NULL CHECK (status IN ('ACTIVE', 'ARCHIVED')),
    rules           jsonb NOT NULL,
    changed_by      text NOT NULL,
    change_reason   text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_fleet_partner_approval_policy_version UNIQUE (policy_id, version)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_approval_policy_versions_org
    ON fleet_partner_approval_policy_versions (organization_id, policy_id, version DESC);

DROP TRIGGER IF EXISTS trg_fleet_partner_approval_policy_versions_append_only
    ON fleet_partner_approval_policy_versions;
CREATE TRIGGER trg_fleet_partner_approval_policy_versions_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_approval_policy_versions
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Requests and decisions
-- ---------------------------------------------------------------------------

-- The amount that selected a threshold policy, read from the fingerprinted
-- payload, so it cannot differ from what the approvers signed.
ALTER TABLE fleet_partner_approval_requests
    ADD COLUMN IF NOT EXISTS amount_minor bigint,
    ADD COLUMN IF NOT EXISTS amount_currency text;

ALTER TABLE fleet_partner_approval_requests
    ADD CONSTRAINT chk_fleet_partner_approval_request_amount
        CHECK ((amount_minor IS NULL) = (amount_currency IS NULL)
               AND (amount_minor IS NULL OR amount_minor >= 0)
               AND (amount_currency IS NULL OR amount_currency ~ '^[A-Z]{3}$'));

-- A decision cast under a delegation records the delegation and the person
-- whose authority it exercised.
ALTER TABLE fleet_partner_approval_decisions
    ADD COLUMN IF NOT EXISTS delegation_id text,
    ADD COLUMN IF NOT EXISTS on_behalf_of text;

ALTER TABLE fleet_partner_approval_decisions
    ADD CONSTRAINT chk_fleet_partner_approval_decision_delegation
        CHECK ((delegation_id IS NULL) = (on_behalf_of IS NULL)
               AND (on_behalf_of IS NULL OR on_behalf_of <> approver_id));

-- One person's authority counts at most once per step, however it is exercised.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_decision_on_behalf_per_step
    ON fleet_partner_approval_decisions (approval_step_id, on_behalf_of)
    WHERE on_behalf_of IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Delegations
-- ---------------------------------------------------------------------------

ALTER TABLE fleet_partner_approval_delegations
    ADD COLUMN IF NOT EXISTS branch_id text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS reason text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS revoked_by text,
    ADD COLUMN IF NOT EXISTS revocation_reason text,
    ADD COLUMN IF NOT EXISTS idempotency_key text;

ALTER TABLE fleet_partner_approval_delegations
    ADD CONSTRAINT chk_fleet_partner_delegation_window CHECK (expires_at > effective_from),
    ADD CONSTRAINT chk_fleet_partner_delegation_revocation
        CHECK ((revoked_at IS NULL) = (revoked_by IS NULL));

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_delegation_idempotency
    ON fleet_partner_approval_delegations (organization_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_delegations_delegator
    ON fleet_partner_approval_delegations (organization_id, delegator_id, expires_at)
    WHERE revoked_at IS NULL;

-- Organization administrators manage approval policies alongside the
-- eligibility policies they already publish.
INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
VALUES ('rol_system_admin', 'approval_policies.manage')
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- Organization structure: cost centres, teams, operating locations
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS fleet_partner_cost_centres (
    id                   text PRIMARY KEY,
    organization_id      text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id            text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    code                 text NOT NULL CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{0,31}$'),
    name                 text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    budget_owner_user_id text REFERENCES fleet_partner_organization_users (id) ON DELETE RESTRICT,
    status               text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ARCHIVED')),
    version              bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    archived_at          timestamptz,
    CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_cost_centre_code
    ON fleet_partner_cost_centres (organization_id, code) WHERE status = 'ACTIVE';
CREATE INDEX IF NOT EXISTS idx_fleet_partner_cost_centres_org
    ON fleet_partner_cost_centres (organization_id, status);

-- Departments already carried a free-text cost_centre_id. It now references
-- the cost centre register. NOT VALID: rows written before the register
-- existed are left as they were; every new or changed value is checked.
ALTER TABLE fleet_partner_departments
    ADD CONSTRAINT fk_fleet_partner_department_cost_centre
        FOREIGN KEY (cost_centre_id) REFERENCES fleet_partner_cost_centres (id)
        ON DELETE RESTRICT NOT VALID;

CREATE TABLE IF NOT EXISTS fleet_partner_teams (
    id                 text PRIMARY KEY,
    organization_id    text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id          text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    department_id      text REFERENCES fleet_partner_departments (id) ON DELETE RESTRICT,
    code               text NOT NULL CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{0,31}$'),
    name               text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    supervisor_user_id text REFERENCES fleet_partner_organization_users (id) ON DELETE RESTRICT,
    status             text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ARCHIVED')),
    version            bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    archived_at        timestamptz,
    CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_team_code
    ON fleet_partner_teams (organization_id, code) WHERE status = 'ACTIVE';
CREATE INDEX IF NOT EXISTS idx_fleet_partner_teams_org
    ON fleet_partner_teams (organization_id, department_id, status);

-- Operating locations are the organization's OWN sites (yards, parking,
-- loading bays, service points). They are distinct from CRM customer
-- locations (fleet_partner_customer_locations), which are a customer's
-- pickup/delivery/billing sites.
CREATE TABLE IF NOT EXISTS fleet_partner_operating_locations (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id       text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    code            text NOT NULL CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{0,31}$'),
    name            text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    location_type   text NOT NULL CHECK (location_type IN ('OFFICE', 'DEPOT', 'YARD', 'PARKING',
                        'LOADING_BAY', 'PICKUP_POINT', 'DROPOFF_POINT', 'SERVICE_POINT',
                        'FUEL_STATION', 'OTHER')),
    address         jsonb NOT NULL DEFAULT '{}'::jsonb,
    latitude        double precision CHECK (latitude IS NULL OR latitude BETWEEN -90 AND 90),
    longitude       double precision CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180),
    operating_hours jsonb NOT NULL DEFAULT '{}'::jsonb,
    status          text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'INACTIVE', 'ARCHIVED')),
    version         bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    archived_at     timestamptz,
    CHECK ((latitude IS NULL) = (longitude IS NULL)),
    CHECK ((status = 'ARCHIVED') = (archived_at IS NOT NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_operating_location_code
    ON fleet_partner_operating_locations (organization_id, code) WHERE status <> 'ARCHIVED';
CREATE INDEX IF NOT EXISTS idx_fleet_partner_operating_locations_org
    ON fleet_partner_operating_locations (organization_id, location_type, status);
CREATE INDEX IF NOT EXISTS idx_fleet_partner_operating_locations_geo
    ON fleet_partner_operating_locations (latitude, longitude)
    WHERE latitude IS NOT NULL;
