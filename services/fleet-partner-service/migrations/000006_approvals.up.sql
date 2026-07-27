-- Generic approval framework for organization-controlled sensitive actions.
-- Decisions and state history are append-only; an approved action is bound to
-- a request fingerprint so the payload cannot be swapped after approval.

CREATE TABLE IF NOT EXISTS fleet_partner_approval_policies (
    id                   text PRIMARY KEY,
    organization_id      text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    name                 text NOT NULL,
    resource_type        text NOT NULL,
    action               text NOT NULL,
    scope_type           text NOT NULL DEFAULT 'ORGANIZATION'
                         CHECK (scope_type IN ('ORGANIZATION', 'BRANCH')),
    scope_id             text,
    conditions           jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- Money thresholds are integer minor units plus explicit currency; this is
    -- a threshold reference only, never a pricing formula.
    threshold_amount_minor bigint,
    threshold_currency   text CHECK (threshold_currency IS NULL OR threshold_currency ~ '^[A-Z]{3}$'),
    required_sequence    jsonb NOT NULL DEFAULT '[]'::jsonb,
    quorum               integer NOT NULL DEFAULT 1 CHECK (quorum >= 1),
    eligible_role_ids    jsonb NOT NULL DEFAULT '[]'::jsonb,
    requester_may_approve boolean NOT NULL DEFAULT false,
    expiry_seconds       integer NOT NULL DEFAULT 604800 CHECK (expiry_seconds > 0),
    step_up_required     boolean NOT NULL DEFAULT false,
    escalation_policy    jsonb NOT NULL DEFAULT '{}'::jsonb,
    status               text NOT NULL DEFAULT 'ACTIVE'
                         CHECK (status IN ('ACTIVE', 'ARCHIVED')),
    version              bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_approval_policies_match
    ON fleet_partner_approval_policies (organization_id, resource_type, action, status);

CREATE TABLE IF NOT EXISTS fleet_partner_approval_requests (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    policy_id           text NOT NULL REFERENCES fleet_partner_approval_policies (id) ON DELETE RESTRICT,
    resource_type       text NOT NULL,
    resource_id         text NOT NULL,
    action              text NOT NULL,
    requester_id        text NOT NULL,
    -- Hash of the normalized controlled action; execution re-checks it.
    request_fingerprint text NOT NULL,
    payload_snapshot    jsonb NOT NULL,
    status              text NOT NULL DEFAULT 'PENDING'
                        CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'EXPIRED', 'CANCELLED', 'EXECUTED')),
    current_step        integer NOT NULL DEFAULT 1 CHECK (current_step >= 1),
    expires_at          timestamptz NOT NULL,
    executed_at         timestamptz,
    version             bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    request_id          text NOT NULL DEFAULT '',
    correlation_id      text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_approval_requests_org
    ON fleet_partner_approval_requests (organization_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_approval_requests_expiry
    ON fleet_partner_approval_requests (expires_at)
    WHERE status = 'PENDING';

-- Only one open approval per (resource, action) so a second request cannot
-- race a first to execution.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_approval_open_resource
    ON fleet_partner_approval_requests (organization_id, resource_type, resource_id, action)
    WHERE status = 'PENDING';

CREATE TABLE IF NOT EXISTS fleet_partner_approval_steps (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL,
    approval_request_id text NOT NULL REFERENCES fleet_partner_approval_requests (id) ON DELETE RESTRICT,
    step_number         integer NOT NULL CHECK (step_number >= 1),
    required_quorum     integer NOT NULL DEFAULT 1 CHECK (required_quorum >= 1),
    eligible_role_ids   jsonb NOT NULL DEFAULT '[]'::jsonb,
    status              text NOT NULL DEFAULT 'PENDING'
                        CHECK (status IN ('PENDING', 'COMPLETED', 'REJECTED', 'SKIPPED')),
    completed_at        timestamptz,
    version             bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_fleet_partner_approval_step UNIQUE (approval_request_id, step_number)
);

-- Decisions are immutable.
CREATE TABLE IF NOT EXISTS fleet_partner_approval_decisions (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL,
    approval_request_id text NOT NULL REFERENCES fleet_partner_approval_requests (id) ON DELETE RESTRICT,
    approval_step_id    text NOT NULL REFERENCES fleet_partner_approval_steps (id) ON DELETE RESTRICT,
    approver_id         text NOT NULL,
    decision            text NOT NULL CHECK (decision IN ('APPROVED', 'REJECTED')),
    reason              text NOT NULL DEFAULT '',
    client_decision_id  text,
    step_up_reference   text,
    decided_at          timestamptz NOT NULL DEFAULT now()
);

-- One approver counts at most once per step.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_decision_per_step
    ON fleet_partner_approval_decisions (approval_step_id, approver_id);

-- Client-supplied decision IDs are idempotent per request.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_decision_client_id
    ON fleet_partner_approval_decisions (approval_request_id, client_decision_id)
    WHERE client_decision_id IS NOT NULL;

DROP TRIGGER IF EXISTS trg_fleet_partner_approval_decisions_append_only ON fleet_partner_approval_decisions;
CREATE TRIGGER trg_fleet_partner_approval_decisions_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_approval_decisions
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE IF NOT EXISTS fleet_partner_approval_delegations (
    id                   text PRIMARY KEY,
    organization_id      text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    delegator_id         text NOT NULL,
    delegate_id          text NOT NULL,
    resource_type        text,
    action               text,
    effective_from       timestamptz NOT NULL DEFAULT now(),
    expires_at           timestamptz NOT NULL,
    revoked_at           timestamptz,
    created_by           text NOT NULL,
    version              bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_fleet_partner_delegation_distinct CHECK (delegator_id <> delegate_id)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_delegations_live
    ON fleet_partner_approval_delegations (organization_id, delegate_id)
    WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS fleet_partner_approval_state_history (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL,
    approval_request_id text NOT NULL,
    from_status         text NOT NULL,
    to_status           text NOT NULL,
    actor_type          text NOT NULL,
    actor_id            text NOT NULL,
    reason_code         text NOT NULL DEFAULT '',
    request_id          text NOT NULL DEFAULT '',
    correlation_id      text NOT NULL DEFAULT '',
    occurred_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_approval_history
    ON fleet_partner_approval_state_history (organization_id, approval_request_id, occurred_at DESC);

DROP TRIGGER IF EXISTS trg_fleet_partner_approval_history_append_only ON fleet_partner_approval_state_history;
CREATE TRIGGER trg_fleet_partner_approval_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_approval_state_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
