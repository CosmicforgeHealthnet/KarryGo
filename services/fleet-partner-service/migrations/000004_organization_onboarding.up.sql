-- Organization onboarding: profile extension, authorized representatives,
-- onboarding stages, documents, verification requests and issues, wallet
-- provisioning and controlled closure.
--
-- External service records (files, verification cases, wallet accounts) are
-- referenced by opaque ID plus source metadata; there is never a database
-- foreign key into another service.

CREATE TABLE IF NOT EXISTS fleet_partner_organization_profiles (
    organization_id     text PRIMARY KEY REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    business_type       text,
    incorporation_date  date,
    tax_reference       text,
    registered_address  jsonb NOT NULL DEFAULT '{}'::jsonb,
    operating_address   jsonb NOT NULL DEFAULT '{}'::jsonb,
    business_email      text,
    business_phone      text,
    website             text,
    business_description text,
    version             bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fleet_partner_organization_authorized_representatives (
    id                          text PRIMARY KEY,
    organization_id             text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    full_name                   text NOT NULL,
    role_title                  text NOT NULL,
    email                       text NOT NULL,
    phone                       text NOT NULL,
    authority_type              text NOT NULL
                                CHECK (authority_type IN ('DIRECTOR', 'OFFICER', 'POWER_OF_ATTORNEY', 'OWNER', 'OTHER')),
    -- File and identity records stay in their owning services.
    authority_document_file_id  text,
    identity_reference          text,
    is_onboarding_owner         boolean NOT NULL DEFAULT false,
    status                      text NOT NULL DEFAULT 'ACTIVE'
                                CHECK (status IN ('ACTIVE', 'ENDED')),
    effective_from              timestamptz NOT NULL DEFAULT now(),
    effective_to                timestamptz,
    version                     bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_representatives_org
    ON fleet_partner_organization_authorized_representatives (organization_id, status);

CREATE TABLE IF NOT EXISTS fleet_partner_organization_onboarding_stages (
    organization_id text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    stage           text NOT NULL
                    CHECK (stage IN ('OWNER_CONTACT', 'BUSINESS_PROFILE', 'AUTHORIZED_REPRESENTATIVE',
                                     'OPERATING_CAPABILITIES', 'HEADQUARTERS', 'BUSINESS_DOCUMENTS',
                                     'VERIFICATION', 'ORGANIZATION_SETTINGS', 'OPERATING_TERRITORIES',
                                     'WALLET_PROVISIONING', 'ACTIVATION')),
    status          text NOT NULL DEFAULT 'NOT_STARTED'
                    CHECK (status IN ('NOT_STARTED', 'IN_PROGRESS', 'COMPLETE', 'BLOCKED', 'CHANGES_REQUIRED')),
    missing_fields  jsonb NOT NULL DEFAULT '[]'::jsonb,
    blocking_issues jsonb NOT NULL DEFAULT '[]'::jsonb,
    completed_at    timestamptz,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, stage)
);

CREATE TABLE IF NOT EXISTS fleet_partner_organization_capabilities (
    organization_id text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    capability      text NOT NULL,
    status          text NOT NULL DEFAULT 'REQUESTED'
                    CHECK (status IN ('REQUESTED', 'APPROVED', 'REJECTED', 'WITHDRAWN')),
    requires_review boolean NOT NULL DEFAULT true,
    version         bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, capability)
);

CREATE TABLE IF NOT EXISTS fleet_partner_organization_documents (
    id                     text PRIMARY KEY,
    organization_id        text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    document_type          text NOT NULL,
    -- Opaque media-file-service identifier. No binary content is stored here.
    file_id                text NOT NULL,
    verification_reference text,
    status                 text NOT NULL DEFAULT 'ATTACHED'
                           CHECK (status IN ('ATTACHED', 'READY', 'REJECTED', 'EXPIRED', 'REMOVED')),
    issued_at              timestamptz,
    expires_at             timestamptz,
    source_service         text NOT NULL DEFAULT 'media-file-service',
    source_version         bigint NOT NULL DEFAULT 0,
    last_event_id          text,
    synced_at              timestamptz,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_org_document_file
    ON fleet_partner_organization_documents (organization_id, file_id)
    WHERE status <> 'REMOVED';

CREATE INDEX IF NOT EXISTS idx_fleet_partner_org_documents
    ON fleet_partner_organization_documents (organization_id, document_type, status);

CREATE TABLE IF NOT EXISTS fleet_partner_organization_verification_requests (
    id                    text PRIMARY KEY,
    organization_id       text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    submission_version    integer NOT NULL CHECK (submission_version >= 1),
    -- Immutable snapshot of what was submitted.
    submission_snapshot   jsonb NOT NULL,
    status                text NOT NULL DEFAULT 'PENDING_DISPATCH'
                          CHECK (status IN ('PENDING_DISPATCH', 'SUBMITTED', 'UNDER_REVIEW', 'APPROVED',
                                            'CHANGES_REQUIRED', 'REJECTED', 'UNKNOWN_OUTCOME')),
    external_reference    text,
    source_service        text NOT NULL DEFAULT 'verification-compliance-service',
    source_version        bigint NOT NULL DEFAULT 0,
    last_event_id         text,
    synced_at             timestamptz,
    submitted_by          text NOT NULL,
    request_id            text NOT NULL DEFAULT '',
    correlation_id        text NOT NULL DEFAULT '',
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_fleet_partner_verification_submission UNIQUE (organization_id, submission_version)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_verification_org
    ON fleet_partner_organization_verification_requests (organization_id, created_at DESC);

-- Review issues are append-only: a later review never overwrites history.
CREATE TABLE IF NOT EXISTS fleet_partner_organization_verification_issues (
    id                      text PRIMARY KEY,
    organization_id         text NOT NULL,
    verification_request_id text NOT NULL REFERENCES fleet_partner_organization_verification_requests (id) ON DELETE RESTRICT,
    issue_code              text NOT NULL,
    field_reference         text,
    document_reference      text,
    remediation             text NOT NULL DEFAULT '',
    raised_at               timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_verification_issues
    ON fleet_partner_organization_verification_issues (organization_id, verification_request_id);

DROP TRIGGER IF EXISTS trg_fleet_partner_verification_issues_append_only ON fleet_partner_organization_verification_issues;
CREATE TRIGGER trg_fleet_partner_verification_issues_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_organization_verification_issues
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- Wallet provisioning projection. No balance is ever stored here; the ledger
-- belongs to payment-wallet-service.
CREATE TABLE IF NOT EXISTS fleet_partner_organization_wallet_provisioning (
    organization_id   text PRIMARY KEY REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    status            text NOT NULL DEFAULT 'NOT_REQUESTED'
                      CHECK (status IN ('NOT_REQUESTED', 'REQUESTED', 'PROVISIONING', 'ACTIVE',
                                        'FAILED_RETRYABLE', 'FAILED_PERMANENT', 'RECONCILIATION_REQUIRED')),
    wallet_account_id text,
    idempotency_key   text,
    attempts          integer NOT NULL DEFAULT 0,
    last_error_code   text,
    next_attempt_at   timestamptz,
    source_service    text NOT NULL DEFAULT 'payment-wallet-service',
    source_version    bigint NOT NULL DEFAULT 0,
    last_event_id     text,
    synced_at         timestamptz,
    version           bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fleet_partner_organization_closure_requests (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    status              text NOT NULL DEFAULT 'REQUESTED'
                        CHECK (status IN ('REQUESTED', 'ASSESSING', 'BLOCKED', 'APPROVAL_REQUIRED', 'APPROVED',
                                          'EXECUTING', 'COMPLETED', 'REJECTED', 'CANCELLED')),
    reason_code         text NOT NULL,
    reason_text         text,
    requested_by        text NOT NULL,
    approval_request_id text,
    assessed_at         timestamptz,
    completed_at        timestamptz,
    version             bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    request_id          text NOT NULL DEFAULT '',
    correlation_id      text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

-- At most one closure request may be open per organization.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_open_closure_request
    ON fleet_partner_organization_closure_requests (organization_id)
    WHERE status IN ('REQUESTED', 'ASSESSING', 'BLOCKED', 'APPROVAL_REQUIRED', 'APPROVED', 'EXECUTING');

CREATE TABLE IF NOT EXISTS fleet_partner_organization_closure_blockers (
    id                 text PRIMARY KEY,
    organization_id    text NOT NULL,
    closure_request_id text NOT NULL REFERENCES fleet_partner_organization_closure_requests (id) ON DELETE RESTRICT,
    blocker_code       text NOT NULL,
    owning_service     text NOT NULL,
    detail             text NOT NULL DEFAULT '',
    -- UNKNOWN means the authoritative service could not be reached; it blocks
    -- closure exactly like a confirmed blocker until reconciled.
    state              text NOT NULL DEFAULT 'BLOCKING'
                       CHECK (state IN ('BLOCKING', 'CLEARED', 'UNKNOWN')),
    checked_at         timestamptz NOT NULL DEFAULT now(),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_closure_blockers
    ON fleet_partner_organization_closure_blockers (organization_id, closure_request_id, state);
