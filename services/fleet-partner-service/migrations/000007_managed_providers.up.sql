-- Organization-managed provider accounts.
--
-- Provider identity, authentication and platform status remain owned by
-- driver-dispatch-delivery-service / driver-taxi-service /
-- driver-hauling-service. provider_id here is an OPAQUE external reference —
-- deliberately not a foreign key — carried with source metadata.

CREATE TABLE IF NOT EXISTS fleet_partner_organization_provider_memberships (
    id                      text PRIMARY KEY,
    organization_id         text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    provider_service        text NOT NULL
                            CHECK (provider_service IN ('driver-dispatch-delivery-service',
                                                        'driver-taxi-service',
                                                        'driver-hauling-service')),
    service_category        text NOT NULL CHECK (service_category IN ('DISPATCH', 'TAXI', 'HAULING')),
    -- Opaque identifier owned by the provider service; NULL until resolved or
    -- sponsored provisioning completes.
    provider_id             text,
    provider_mode           text NOT NULL DEFAULT 'ORGANIZATION_MANAGED'
                            CHECK (provider_mode IN ('ORGANIZATION_MANAGED')),
    onboarding_source       text NOT NULL
                            CHECK (onboarding_source IN ('ORGANIZATION_SPONSORED', 'EXISTING_PROVIDER_LINKED')),
    relationship_type       text NOT NULL DEFAULT 'EMPLOYEE'
                            CHECK (relationship_type IN ('EMPLOYEE', 'CONTRACTOR', 'PARTNER')),
    employment_type         text CHECK (employment_type IS NULL OR employment_type IN ('FULL_TIME', 'PART_TIME', 'CASUAL')),
    employee_number         text,
    home_branch_id          text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    status                  text NOT NULL DEFAULT 'INVITED'
                            CHECK (status IN ('INVITED', 'PENDING_PROVIDER_ACCEPTANCE', 'PENDING_SETUP', 'ACTIVE',
                                              'DECLINED', 'EXPIRED', 'CANCELLED', 'SUSPENDED', 'ENDING', 'REMOVED')),
    association_accepted_at timestamptz,
    joined_at               timestamptz,
    suspended_at            timestamptz,
    ending_at               timestamptz,
    removed_at              timestamptz,
    source_version          bigint NOT NULL DEFAULT 0,
    last_event_id           text,
    synced_at               timestamptz,
    version                 bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

-- One live settlement organization per provider per service category.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_provider_live_category
    ON fleet_partner_organization_provider_memberships (provider_service, provider_id)
    WHERE provider_id IS NOT NULL
      AND status IN ('PENDING_PROVIDER_ACCEPTANCE', 'PENDING_SETUP', 'ACTIVE', 'SUSPENDED', 'ENDING');

CREATE INDEX IF NOT EXISTS idx_fleet_partner_provider_memberships_org
    ON fleet_partner_organization_provider_memberships (organization_id, status);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_provider_memberships_branch
    ON fleet_partner_organization_provider_memberships (organization_id, home_branch_id);

CREATE TABLE IF NOT EXISTS fleet_partner_provider_invitations (
    id                     text PRIMARY KEY,
    organization_id        text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    membership_id          text REFERENCES fleet_partner_organization_provider_memberships (id) ON DELETE RESTRICT,
    provider_service       text NOT NULL,
    legal_name             text NOT NULL,
    email_normalized       text NOT NULL,
    phone_e164             text NOT NULL,
    existing_provider_id   text,
    -- Only the hash of the single-use invitation token is stored.
    invitation_token_hash  text NOT NULL,
    status                 text NOT NULL DEFAULT 'PENDING'
                           CHECK (status IN ('PENDING', 'ACCEPTED', 'DECLINED', 'EXPIRED', 'CANCELLED')),
    expires_at             timestamptz NOT NULL,
    delivery_status        text NOT NULL DEFAULT 'PENDING'
                           CHECK (delivery_status IN ('PENDING', 'ACCEPTED', 'FAILED', 'UNAVAILABLE')),
    notification_reference text,
    attempts               integer NOT NULL DEFAULT 0,
    created_by             text NOT NULL,
    accepted_at            timestamptz,
    declined_at            timestamptz,
    consumed_at            timestamptz,
    version                bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_provider_invitation_token
    ON fleet_partner_provider_invitations (invitation_token_hash);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_provider_invitations_expiry
    ON fleet_partner_provider_invitations (expires_at)
    WHERE status = 'PENDING';

CREATE TABLE IF NOT EXISTS fleet_partner_provider_capabilities (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    membership_id   text NOT NULL REFERENCES fleet_partner_organization_provider_memberships (id) ON DELETE RESTRICT,
    service_type    text NOT NULL,
    vehicle_classes jsonb NOT NULL DEFAULT '[]'::jsonb,
    operating_zones jsonb NOT NULL DEFAULT '[]'::jsonb,
    qualifications  jsonb NOT NULL DEFAULT '[]'::jsonb,
    valid_from      timestamptz NOT NULL DEFAULT now(),
    valid_until     timestamptz,
    status          text NOT NULL DEFAULT 'ACTIVE'
                    CHECK (status IN ('ACTIVE', 'SUSPENDED', 'EXPIRED')),
    version         bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_provider_capabilities
    ON fleet_partner_provider_capabilities (organization_id, membership_id, status);

CREATE TABLE IF NOT EXISTS fleet_partner_provider_policies (
    membership_id           text PRIMARY KEY REFERENCES fleet_partner_organization_provider_memberships (id) ON DELETE RESTRICT,
    organization_id         text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    assignment_mode         text NOT NULL DEFAULT 'OFFER'
                            CHECK (assignment_mode IN ('OFFER', 'BROADCAST', 'DIRECTED_OFFER')),
    -- Platform invariant: a managed provider always retains the right to
    -- decline work. No organization setting may switch this off.
    provider_decline_required boolean NOT NULL DEFAULT true
                            CHECK (provider_decline_required = true),
    visibility_policy       text NOT NULL DEFAULT 'ORGANIZATION'
                            CHECK (visibility_policy IN ('ORGANIZATION', 'BRANCH')),
    shift_restrictions      jsonb NOT NULL DEFAULT '{}'::jsonb,
    zone_restrictions       jsonb NOT NULL DEFAULT '{}'::jsonb,
    workload_restrictions   jsonb NOT NULL DEFAULT '{}'::jsonb,
    version                 bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fleet_partner_provider_membership_history (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    membership_id   text NOT NULL,
    from_status     text NOT NULL,
    to_status       text NOT NULL,
    actor_type      text NOT NULL,
    actor_id        text NOT NULL,
    reason_code     text NOT NULL DEFAULT '',
    reason_text     text,
    request_id      text NOT NULL DEFAULT '',
    correlation_id  text NOT NULL DEFAULT '',
    occurred_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_provider_history
    ON fleet_partner_provider_membership_history (organization_id, membership_id, occurred_at DESC);

DROP TRIGGER IF EXISTS trg_fleet_partner_provider_history_append_only ON fleet_partner_provider_membership_history;
CREATE TRIGGER trg_fleet_partner_provider_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_provider_membership_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- Multi-step provisioning saga. A partially provisioned provider is never
-- treated as active.
CREATE TABLE IF NOT EXISTS fleet_partner_provider_provisioning_sagas (
    id                         text PRIMARY KEY,
    organization_id            text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    membership_id              text NOT NULL REFERENCES fleet_partner_organization_provider_memberships (id) ON DELETE RESTRICT,
    provider_service           text NOT NULL,
    provider_resolution_status text NOT NULL DEFAULT 'PENDING'
                               CHECK (provider_resolution_status IN ('PENDING', 'RESOLVED_EXISTING', 'NOT_FOUND',
                                                                     'CONFLICT', 'UNAVAILABLE', 'UNKNOWN_OUTCOME')),
    provider_identity_status   text NOT NULL DEFAULT 'PENDING'
                               CHECK (provider_identity_status IN ('PENDING', 'REQUESTED', 'PROVISIONED',
                                                                   'FAILED', 'UNAVAILABLE', 'UNKNOWN_OUTCOME')),
    association_status         text NOT NULL DEFAULT 'PENDING'
                               CHECK (association_status IN ('PENDING', 'REQUESTED', 'AWAITING_PROVIDER',
                                                             'ACCEPTED', 'DECLINED', 'FAILED', 'UNAVAILABLE', 'UNKNOWN_OUTCOME')),
    subwallet_status           text NOT NULL DEFAULT 'PENDING'
                               CHECK (subwallet_status IN ('PENDING', 'REQUESTED', 'ACTIVE', 'FAILED',
                                                           'UNAVAILABLE', 'UNKNOWN_OUTCOME')),
    notification_status        text NOT NULL DEFAULT 'PENDING'
                               CHECK (notification_status IN ('PENDING', 'ACCEPTED', 'FAILED', 'UNAVAILABLE')),
    overall_status             text NOT NULL DEFAULT 'IN_PROGRESS'
                               CHECK (overall_status IN ('IN_PROGRESS', 'COMPLETED', 'BLOCKED',
                                                         'RECONCILIATION_REQUIRED', 'FAILED_PERMANENT')),
    idempotency_key            text NOT NULL,
    attempts                   integer NOT NULL DEFAULT 0,
    last_error_code            text,
    next_attempt_at            timestamptz,
    version                    bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at                 timestamptz NOT NULL DEFAULT now(),
    updated_at                 timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_provisioning_membership
    ON fleet_partner_provider_provisioning_sagas (membership_id);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_provisioning_due
    ON fleet_partner_provider_provisioning_sagas (next_attempt_at)
    WHERE overall_status IN ('IN_PROGRESS', 'RECONCILIATION_REQUIRED');

-- Sub-wallet link projection. No balance is ever stored here.
CREATE TABLE IF NOT EXISTS fleet_partner_provider_subwallet_links (
    provider_membership_id text PRIMARY KEY REFERENCES fleet_partner_organization_provider_memberships (id) ON DELETE RESTRICT,
    organization_id        text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    wallet_account_id      text,
    status                 text NOT NULL DEFAULT 'NOT_REQUESTED'
                           CHECK (status IN ('NOT_REQUESTED', 'REQUESTED', 'ACTIVE', 'FAILED_RETRYABLE',
                                             'FAILED_PERMANENT', 'RECONCILIATION_REQUIRED', 'DISABLED')),
    idempotency_key        text,
    source_service         text NOT NULL DEFAULT 'payment-wallet-service',
    source_version         bigint NOT NULL DEFAULT 0,
    last_event_id          text,
    synced_at              timestamptz,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_subwallet_links_org
    ON fleet_partner_provider_subwallet_links (organization_id, status);
