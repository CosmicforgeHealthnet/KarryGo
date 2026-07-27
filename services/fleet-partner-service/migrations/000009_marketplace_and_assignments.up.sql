-- Sections 11–12: marketplace projections, organization offers, provider
-- assignments, provider responses and reassignment.
--
-- Every reference to an execution service record (request, booking, trip,
-- provider) is an OPAQUE identifier with source metadata — never a foreign
-- key. The execution service remains the source of truth; these tables hold
-- organization-facing projections and organization-owned decisions.

-- ---------------------------------------------------------------------------
-- Marketplace request projections
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS fleet_partner_marketplace_request_projections (
    id                      text PRIMARY KEY,
    organization_id         text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    source_service          text NOT NULL CHECK (source_service IN ('DISPATCH', 'TAXI', 'HAULING')),
    platform_request_id     text NOT NULL,
    platform_booking_id     text,
    platform_trip_id        text,
    service_type            text NOT NULL,
    projection_status       text NOT NULL DEFAULT 'OFFERED'
                            CHECK (projection_status IN (
                                'OFFERED', 'RESERVATION_PENDING', 'ORGANIZATION_ACCEPTED',
                                'ASSIGNMENT_PENDING', 'PROVIDER_ACCEPTED', 'EXECUTION_REQUESTED',
                                'EXECUTION_CREATED', 'IN_PROGRESS', 'COMPLETED',
                                'ORGANIZATION_DECLINED', 'EXPIRED', 'CANCELLED',
                                'RECONCILIATION_REQUIRED')),
    source_status           text NOT NULL DEFAULT '',
    -- Operational summaries only. Customer pricing is never projected here.
    pickup_summary          jsonb NOT NULL DEFAULT '{}'::jsonb,
    dropoff_summary         jsonb NOT NULL DEFAULT '{}'::jsonb,
    operational_requirements jsonb NOT NULL DEFAULT '{}'::jsonb,
    settlement_organization_id text,
    executing_provider_id   text,
    organization_membership_id text,
    acceptance_source       text CHECK (acceptance_source IS NULL OR acceptance_source IN (
                                'ORGANIZATION_DISPATCHER', 'PROVIDER_DIRECT',
                                'PLATFORM_RECOVERY', 'AUTOMATION_DRAFT')),
    assignment_id           text,
    offered_at              timestamptz NOT NULL DEFAULT now(),
    expires_at              timestamptz,
    source_version          bigint NOT NULL DEFAULT 0,
    last_event_id           text,
    last_source_event_at    timestamptz,
    last_reconciled_at      timestamptz,
    freshness_state         text NOT NULL DEFAULT 'CURRENT'
                            CHECK (freshness_state IN ('CURRENT', 'STALE', 'GAP_DETECTED', 'RECONCILING', 'UNKNOWN')),
    version                 bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_fleet_partner_request_projection
        UNIQUE (organization_id, source_service, platform_request_id)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_request_projections_org
    ON fleet_partner_marketplace_request_projections (organization_id, projection_status, offered_at DESC);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_request_projections_source
    ON fleet_partner_marketplace_request_projections (source_service, platform_request_id);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_request_projections_stale
    ON fleet_partner_marketplace_request_projections (freshness_state, last_source_event_at)
    WHERE freshness_state <> 'CURRENT';

-- ---------------------------------------------------------------------------
-- Organization offers
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS fleet_partner_organization_request_offers (
    id                    text PRIMARY KEY,
    organization_id       text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    request_projection_id text NOT NULL REFERENCES fleet_partner_marketplace_request_projections (id) ON DELETE RESTRICT,
    operating_mode        text NOT NULL CHECK (operating_mode IN ('CENTRALIZED', 'DELEGATED', 'HYBRID')),
    status                text NOT NULL DEFAULT 'OFFERED'
                          CHECK (status IN ('OFFERED', 'RESERVED', 'ACCEPTED', 'DECLINED', 'EXPIRED', 'CANCELLED')),
    offered_at            timestamptz NOT NULL DEFAULT now(),
    expires_at            timestamptz NOT NULL,
    reserved_at           timestamptz,
    accepted_at           timestamptz,
    declined_at           timestamptz,
    cancelled_at          timestamptz,
    resolved_actor_type   text,
    resolved_actor_id     text,
    source_reservation_id text,
    reason_code           text,
    reason_text           text,
    version               bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

-- Only one live offer per request projection.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_offer_current
    ON fleet_partner_organization_request_offers (request_projection_id)
    WHERE status IN ('OFFERED', 'RESERVED', 'ACCEPTED');

CREATE INDEX IF NOT EXISTS idx_fleet_partner_offers_expiry
    ON fleet_partner_organization_request_offers (expires_at)
    WHERE status = 'OFFERED';

-- ---------------------------------------------------------------------------
-- Provider offer recipients (delegated / hybrid fan-out)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS fleet_partner_provider_offer_recipients (
    id                text PRIMARY KEY,
    organization_id   text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    offer_id          text NOT NULL REFERENCES fleet_partner_organization_request_offers (id) ON DELETE RESTRICT,
    membership_id     text NOT NULL REFERENCES fleet_partner_organization_provider_memberships (id) ON DELETE RESTRICT,
    status            text NOT NULL DEFAULT 'PENDING'
                      CHECK (status IN ('PENDING', 'ACCEPTED', 'DECLINED', 'EXPIRED', 'CANCELLED', 'LOST_RACE')),
    delivery_status   text NOT NULL DEFAULT 'PENDING'
                      CHECK (delivery_status IN ('PENDING', 'ACCEPTED', 'FAILED', 'UNAVAILABLE')),
    delivered_at      timestamptz,
    viewed_at         timestamptz,
    responded_at      timestamptz,
    client_response_id text,
    reason_code       text,
    reason_text       text,
    expires_at        timestamptz NOT NULL,
    version           bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_fleet_partner_offer_recipient UNIQUE (offer_id, membership_id)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_recipients_membership
    ON fleet_partner_provider_offer_recipients (organization_id, membership_id, status);

-- ---------------------------------------------------------------------------
-- Organization acceptances (one per offer)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS fleet_partner_organization_acceptances (
    id                      text PRIMARY KEY,
    organization_id         text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    offer_id                text NOT NULL REFERENCES fleet_partner_organization_request_offers (id) ON DELETE RESTRICT,
    accepted_by_user_id     text,
    accepted_by_membership_id text,
    acceptance_source       text NOT NULL CHECK (acceptance_source IN (
                                'ORGANIZATION_DISPATCHER', 'PROVIDER_DIRECT',
                                'PLATFORM_RECOVERY', 'AUTOMATION_DRAFT')),
    source_reservation_id   text NOT NULL,
    source_idempotency_key  text NOT NULL,
    snapshot                jsonb NOT NULL DEFAULT '{}'::jsonb,
    request_id              text NOT NULL DEFAULT '',
    correlation_id          text NOT NULL DEFAULT '',
    accepted_at             timestamptz NOT NULL DEFAULT now(),
    -- Exactly one actor kind: a dispatcher acceptance or a provider-direct
    -- acceptance, never both and never neither.
    CONSTRAINT chk_fleet_partner_acceptance_actor CHECK (
        (accepted_by_user_id IS NOT NULL AND accepted_by_membership_id IS NULL) OR
        (accepted_by_user_id IS NULL AND accepted_by_membership_id IS NOT NULL)
    ),
    CONSTRAINT uq_fleet_partner_acceptance_offer UNIQUE (offer_id)
);

-- Acceptances are an immutable record of who committed the organization.
CREATE OR REPLACE FUNCTION fleet_partner_forbid_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'table % is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_fleet_partner_acceptances_append_only ON fleet_partner_organization_acceptances;
CREATE TRIGGER trg_fleet_partner_acceptances_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_organization_acceptances
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Provider assignments
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS fleet_partner_provider_assignments (
    id                     text PRIMARY KEY,
    organization_id        text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    request_projection_id  text NOT NULL REFERENCES fleet_partner_marketplace_request_projections (id) ON DELETE RESTRICT,
    offer_id               text REFERENCES fleet_partner_organization_request_offers (id) ON DELETE RESTRICT,
    membership_id          text NOT NULL REFERENCES fleet_partner_organization_provider_memberships (id) ON DELETE RESTRICT,
    -- Opaque provider identifier owned by the execution service.
    executing_provider_id  text NOT NULL,
    source_service         text NOT NULL CHECK (source_service IN ('DISPATCH', 'TAXI', 'HAULING')),
    asset_reference        text,
    assigned_by_type       text NOT NULL CHECK (assigned_by_type IN ('ORGANIZATION_USER', 'PROVIDER', 'SYSTEM')),
    assigned_by_id         text NOT NULL,
    status                 text NOT NULL DEFAULT 'CREATED'
                           CHECK (status IN (
                               'CREATED', 'PENDING_PROVIDER_RESPONSE', 'ACCEPTED', 'READY_FOR_EXECUTION',
                               'IN_PROGRESS', 'COMPLETED', 'DECLINED', 'EXPIRED', 'CANCELLED',
                               'EXECUTION_START_PENDING', 'RECONCILIATION_REQUIRED')),
    response_deadline      timestamptz NOT NULL,
    accepted_at            timestamptz,
    declined_at            timestamptz,
    expired_at             timestamptz,
    cancelled_at           timestamptz,
    ready_for_execution_at timestamptz,
    started_at             timestamptz,
    completed_at           timestamptz,
    root_assignment_id     text,
    replaces_assignment_id text,
    reassignment_attempt   integer NOT NULL DEFAULT 0 CHECK (reassignment_attempt >= 0),
    reassignment_reason    text,
    execution_start_status text NOT NULL DEFAULT 'NOT_REQUESTED'
                           CHECK (execution_start_status IN (
                               'NOT_REQUESTED', 'PENDING', 'SUBMITTED', 'CONFIRMED',
                               'REJECTED', 'UNKNOWN_OUTCOME', 'RECONCILIATION_REQUIRED', 'FAILED_PERMANENT')),
    source_execution_id    text,
    version                bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);

-- Only one live assignment per request. Reassignment resolves the previous
-- assignment first, so a replacement never races the original.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_assignment_current
    ON fleet_partner_provider_assignments (request_projection_id)
    WHERE status IN ('CREATED', 'PENDING_PROVIDER_RESPONSE', 'ACCEPTED',
                     'READY_FOR_EXECUTION', 'IN_PROGRESS', 'EXECUTION_START_PENDING');

CREATE INDEX IF NOT EXISTS idx_fleet_partner_assignments_org
    ON fleet_partner_provider_assignments (organization_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_assignments_membership
    ON fleet_partner_provider_assignments (membership_id, status);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_assignments_expiry
    ON fleet_partner_provider_assignments (response_deadline)
    WHERE status = 'PENDING_PROVIDER_RESPONSE';

CREATE INDEX IF NOT EXISTS idx_fleet_partner_assignments_execution_pending
    ON fleet_partner_provider_assignments (execution_start_status)
    WHERE execution_start_status IN ('PENDING', 'SUBMITTED', 'UNKNOWN_OUTCOME', 'RECONCILIATION_REQUIRED');

-- ---------------------------------------------------------------------------
-- Assignment responses — the immutable provider decision
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS fleet_partner_assignment_responses (
    id                 text PRIMARY KEY,
    organization_id    text NOT NULL,
    assignment_id      text NOT NULL REFERENCES fleet_partner_provider_assignments (id) ON DELETE RESTRICT,
    membership_id      text NOT NULL,
    response           text NOT NULL CHECK (response IN ('ACCEPTED', 'DECLINED')),
    reason_code        text,
    reason_text        text,
    safety_related     boolean NOT NULL DEFAULT false,
    client_response_id text NOT NULL,
    device_recorded_at timestamptz,
    occurred_at        timestamptz NOT NULL DEFAULT now(),
    request_id         text NOT NULL DEFAULT '',
    correlation_id     text NOT NULL DEFAULT '',
    -- Exactly one response per assignment: acceptance and decline are
    -- mutually exclusive and final.
    CONSTRAINT uq_fleet_partner_assignment_response UNIQUE (assignment_id),
    CONSTRAINT uq_fleet_partner_response_client_id UNIQUE (membership_id, client_response_id),
    -- A decline must always carry a reason.
    CONSTRAINT chk_fleet_partner_decline_reason CHECK (
        response <> 'DECLINED' OR reason_code IS NOT NULL
    )
);

DROP TRIGGER IF EXISTS trg_fleet_partner_assignment_responses_append_only ON fleet_partner_assignment_responses;
CREATE TRIGGER trg_fleet_partner_assignment_responses_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_assignment_responses
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE IF NOT EXISTS fleet_partner_assignment_response_evidence (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    response_id     text NOT NULL REFERENCES fleet_partner_assignment_responses (id) ON DELETE RESTRICT,
    -- Opaque media-file-service reference; no binary content is stored.
    file_id         text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_fleet_partner_response_evidence UNIQUE (response_id, file_id)
);

DROP TRIGGER IF EXISTS trg_fleet_partner_response_evidence_append_only ON fleet_partner_assignment_response_evidence;
CREATE TRIGGER trg_fleet_partner_response_evidence_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_assignment_response_evidence
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Assignment state history (append-only)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS fleet_partner_assignment_state_history (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    assignment_id   text NOT NULL,
    from_status     text NOT NULL,
    to_status       text NOT NULL,
    actor_type      text NOT NULL,
    actor_id        text NOT NULL,
    reason_code     text,
    metadata        jsonb NOT NULL DEFAULT '{}'::jsonb,
    request_id      text NOT NULL DEFAULT '',
    correlation_id  text NOT NULL DEFAULT '',
    occurred_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_assignment_history
    ON fleet_partner_assignment_state_history (organization_id, assignment_id, occurred_at DESC);

DROP TRIGGER IF EXISTS trg_fleet_partner_assignment_history_append_only ON fleet_partner_assignment_state_history;
CREATE TRIGGER trg_fleet_partner_assignment_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_assignment_state_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Capacity reservations (provider and asset)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS fleet_partner_assignment_reservations (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    assignment_id   text NOT NULL REFERENCES fleet_partner_provider_assignments (id) ON DELETE RESTRICT,
    resource_type   text NOT NULL CHECK (resource_type IN ('PROVIDER', 'ASSET')),
    resource_id     text NOT NULL,
    status          text NOT NULL DEFAULT 'HELD' CHECK (status IN ('HELD', 'RELEASED')),
    released_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- A provider or asset can be held by only one live assignment at a time.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_reservation_held
    ON fleet_partner_assignment_reservations (resource_type, resource_id)
    WHERE status = 'HELD';

CREATE INDEX IF NOT EXISTS idx_fleet_partner_reservations_assignment
    ON fleet_partner_assignment_reservations (assignment_id, status);

-- ---------------------------------------------------------------------------
-- Reassignment cases and decline follow-ups
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS fleet_partner_reassignment_cases (
    id                    text PRIMARY KEY,
    organization_id       text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    request_projection_id text NOT NULL REFERENCES fleet_partner_marketplace_request_projections (id) ON DELETE RESTRICT,
    origin_assignment_id  text NOT NULL,
    trigger               text NOT NULL CHECK (trigger IN ('DECLINED', 'EXPIRED', 'CANCELLED', 'ELIGIBILITY_LOST')),
    reason_code           text,
    -- Safety-related triggers require a human decision before any automatic
    -- reassignment may run.
    requires_human_review boolean NOT NULL DEFAULT false,
    status                text NOT NULL DEFAULT 'OPEN'
                          CHECK (status IN ('OPEN', 'IN_PROGRESS', 'RESOLVED', 'ABANDONED')),
    attempts              integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    excluded_membership_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    resolved_assignment_id text,
    version               bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_reassignment_open
    ON fleet_partner_reassignment_cases (organization_id, status, created_at);

CREATE TABLE IF NOT EXISTS fleet_partner_decline_followup_actions (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    response_id     text NOT NULL REFERENCES fleet_partner_assignment_responses (id) ON DELETE RESTRICT,
    membership_id   text NOT NULL,
    action_type     text NOT NULL,
    reason_code     text NOT NULL,
    safety_related  boolean NOT NULL DEFAULT false,
    payload         jsonb NOT NULL DEFAULT '{}'::jsonb,
    status          text NOT NULL DEFAULT 'PENDING'
                    CHECK (status IN ('PENDING', 'DISPATCHED', 'COMPLETED', 'FAILED', 'DEAD_LETTERED')),
    attempts        integer NOT NULL DEFAULT 0,
    last_error_code text,
    next_attempt_at timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_followups_due
    ON fleet_partner_decline_followup_actions (status, next_attempt_at)
    WHERE status IN ('PENDING', 'FAILED');

-- ---------------------------------------------------------------------------
-- Source command operations — every external write is durable and read-backable
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS fleet_partner_marketplace_command_operations (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL,
    operation_type      text NOT NULL CHECK (operation_type IN (
                            'RESERVE_REQUEST', 'RELEASE_RESERVATION', 'CONFIRM_PROVIDER',
                            'CREATE_EXECUTION', 'CANCEL_EXECUTION')),
    source_service      text NOT NULL CHECK (source_service IN ('DISPATCH', 'TAXI', 'HAULING')),
    source_request_id   text NOT NULL,
    assignment_id       text,
    idempotency_key     text NOT NULL,
    request_fingerprint text NOT NULL,
    status              text NOT NULL DEFAULT 'PENDING'
                        CHECK (status IN ('PENDING', 'SUBMITTED', 'CONFIRMED', 'REJECTED',
                                          'UNKNOWN_OUTCOME', 'RECONCILIATION_REQUIRED', 'FAILED_PERMANENT')),
    external_reference  text,
    attempts            integer NOT NULL DEFAULT 0,
    last_error_code     text,
    outcome_checked_at  timestamptz,
    next_attempt_at     timestamptz,
    version             bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    -- One operation per idempotency key: a replay reuses the original record
    -- rather than issuing a second external write.
    CONSTRAINT uq_fleet_partner_command_idempotency UNIQUE (operation_type, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_commands_due
    ON fleet_partner_marketplace_command_operations (status, next_attempt_at)
    WHERE status IN ('PENDING', 'SUBMITTED', 'UNKNOWN_OUTCOME', 'RECONCILIATION_REQUIRED');
