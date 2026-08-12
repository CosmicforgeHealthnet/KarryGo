-- Section 14: organization CRM, sales, quotations, contracts, SLAs and credit
-- policy.
--
-- Every table here is organization owned. A CRM customer is a commercial
-- relationship record; it is not a marketplace identity, holds no credentials,
-- and its optional marketplace_customer_id is an opaque pointer that is never
-- resolved by reaching into another service's database.
--
-- Section 15 (service catalogue, private orders, shipments, packages) is
-- explicitly NOT created here. The forward-looking columns
-- (future_service_catalog_item_id, conversion intents) are opaque and inert.

-- ---------------------------------------------------------------------------
-- Company customers
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS fleet_partner_company_customers (
    id                      text PRIMARY KEY,
    organization_id         text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id               text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    customer_type           text NOT NULL
                            CHECK (customer_type IN ('INDIVIDUAL', 'BUSINESS', 'GOVERNMENT',
                                                     'NONPROFIT', 'INTERNAL_BUSINESS_UNIT', 'OTHER')),
    legal_name              text NOT NULL CHECK (length(btrim(legal_name)) > 0),
    display_name            text NOT NULL CHECK (length(btrim(display_name)) > 0),
    -- Case-folded, whitespace-collapsed name used only for organization-scoped
    -- duplicate detection. It is never shown and never used for matching across
    -- organizations.
    normalized_name         text NOT NULL,
    duplicate_fingerprint   text NOT NULL,
    registration_number     text,
    tax_reference           text,
    -- Opaque pointer into customer-service. Never joined, never resolved by SQL.
    marketplace_customer_id text,
    status                  text NOT NULL DEFAULT 'PROSPECT'
                            CHECK (status IN ('PROSPECT', 'ACTIVE', 'ON_HOLD', 'SUSPENDED',
                                              'INACTIVE', 'ARCHIVED')),
    risk_status             text NOT NULL DEFAULT 'NOT_ASSESSED'
                            CHECK (risk_status IN ('NOT_ASSESSED', 'LOW', 'MEDIUM', 'HIGH', 'RESTRICTED')),
    account_owner_user_id   text,
    default_currency        char(3) NOT NULL CHECK (default_currency ~ '^[A-Z]{3}$'),
    payment_terms_reference text,
    credit_profile_id       text,
    version                 bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by              text NOT NULL,
    updated_by              text NOT NULL DEFAULT '',
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    archived_at             timestamptz
);

-- Uniqueness is organization scoped. The same legal entity may legitimately be
-- a customer of two different fleet operators, and treating that as a conflict
-- would make one tenant's data a constraint on another's.
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_crm_customers_duplicate
    ON fleet_partner_company_customers (organization_id, duplicate_fingerprint)
    WHERE archived_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_crm_customers_cursor
    ON fleet_partner_company_customers (organization_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_crm_customers_status
    ON fleet_partner_company_customers (organization_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_crm_customers_branch
    ON fleet_partner_company_customers (organization_id, branch_id, created_at DESC)
    WHERE branch_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_fleet_crm_customers_name
    ON fleet_partner_company_customers (organization_id, normalized_name);

-- ---------------------------------------------------------------------------
-- Customer contacts
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS fleet_partner_customer_contacts (
    id                      text PRIMARY KEY,
    organization_id         text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    customer_id             text NOT NULL
                            REFERENCES fleet_partner_company_customers (id) ON DELETE RESTRICT,
    full_name               text NOT NULL CHECK (length(btrim(full_name)) > 0),
    role_title              text NOT NULL DEFAULT '',
    normalized_email        text,
    normalized_phone        text,
    preferred_channel       text NOT NULL DEFAULT 'EMAIL'
                            CHECK (preferred_channel IN ('EMAIL', 'PHONE', 'SMS', 'WHATSAPP', 'IN_APP', 'NONE')),
    contact_purpose         text NOT NULL DEFAULT 'GENERAL',
    is_primary              boolean NOT NULL DEFAULT false,
    notification_preferences jsonb NOT NULL DEFAULT '{}'::jsonb,
    consent_status          text NOT NULL DEFAULT 'UNKNOWN'
                            CHECK (consent_status IN ('UNKNOWN', 'GRANTED', 'WITHDRAWN', 'NOT_REQUIRED')),
    consent_recorded_at     timestamptz,
    status                  text NOT NULL DEFAULT 'ACTIVE'
                            CHECK (status IN ('ACTIVE', 'INACTIVE', 'ARCHIVED')),
    version                 bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by              text NOT NULL,
    updated_by              text NOT NULL DEFAULT '',
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    archived_at             timestamptz
);

-- Exactly one primary contact per customer and purpose. Enforcing it here
-- rather than in the handler is what makes two concurrent "make this primary"
-- requests resolve to one winner instead of two primaries.
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_crm_contacts_primary
    ON fleet_partner_customer_contacts (organization_id, customer_id, contact_purpose)
    WHERE is_primary AND archived_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_crm_contacts_customer
    ON fleet_partner_customer_contacts (organization_id, customer_id, created_at DESC, id DESC);

-- ---------------------------------------------------------------------------
-- Customer locations
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS fleet_partner_customer_locations (
    id                      text PRIMARY KEY,
    organization_id         text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    customer_id             text NOT NULL
                            REFERENCES fleet_partner_company_customers (id) ON DELETE RESTRICT,
    location_type           text NOT NULL
                            CHECK (location_type IN ('PICKUP', 'DELIVERY', 'BILLING', 'WAREHOUSE', 'OFFICE',
                                                     'DEPOT', 'SITE', 'PORT', 'TERMINAL', 'OTHER')),
    label                   text NOT NULL CHECK (length(btrim(label)) > 0),
    address_snapshot        jsonb NOT NULL DEFAULT '{}'::jsonb,
    latitude                numeric(9, 6) CHECK (latitude IS NULL OR (latitude >= -90 AND latitude <= 90)),
    longitude               numeric(9, 6) CHECK (longitude IS NULL OR (longitude >= -180 AND longitude <= 180)),
    geofence_reference      text,
    operating_hours         jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- Access and safety instructions are gated behind their own permission and
    -- are never returned on a masked read.
    access_instructions     text NOT NULL DEFAULT '',
    safety_instructions     text NOT NULL DEFAULT '',
    contact_reference       text REFERENCES fleet_partner_customer_contacts (id) ON DELETE RESTRICT,
    timezone                text NOT NULL,
    status                  text NOT NULL DEFAULT 'ACTIVE'
                            CHECK (status IN ('ACTIVE', 'INACTIVE', 'ARCHIVED')),
    version                 bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by              text NOT NULL,
    updated_by              text NOT NULL DEFAULT '',
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    archived_at             timestamptz,
    -- A coordinate is a pair or it is nothing; half a coordinate routes a
    -- vehicle to the equator.
    CHECK ((latitude IS NULL) = (longitude IS NULL))
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_locations_customer
    ON fleet_partner_customer_locations (organization_id, customer_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_crm_locations_type
    ON fleet_partner_customer_locations (organization_id, customer_id, location_type);

-- ---------------------------------------------------------------------------
-- Sales opportunities
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS fleet_partner_sales_opportunities (
    id                          text PRIMARY KEY,
    organization_id             text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id                   text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    customer_id                 text NOT NULL
                                REFERENCES fleet_partner_company_customers (id) ON DELETE RESTRICT,
    title                       text NOT NULL CHECK (length(btrim(title)) > 0),
    description                 text,
    stage                       text NOT NULL DEFAULT 'LEAD'
                                CHECK (stage IN ('LEAD', 'QUALIFIED', 'NEEDS_ANALYSIS', 'PROPOSAL',
                                                 'NEGOTIATION', 'WON', 'LOST', 'ON_HOLD')),
    expected_value_amount_minor bigint CHECK (expected_value_amount_minor IS NULL OR expected_value_amount_minor >= 0),
    expected_value_currency     char(3) CHECK (expected_value_currency IS NULL OR expected_value_currency ~ '^[A-Z]{3}$'),
    -- Basis points, not a float. A probability stored as 0.1 + 0.2 is a
    -- forecast that does not add up.
    probability_basis_points    integer NOT NULL DEFAULT 0
                                CHECK (probability_basis_points BETWEEN 0 AND 10000),
    expected_close_date         date,
    service_capabilities        jsonb NOT NULL DEFAULT '[]'::jsonb,
    account_owner_user_id       text,
    next_action                 text,
    next_action_at              timestamptz,
    loss_reason                 text,
    version                     bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by                  text NOT NULL,
    updated_by                  text NOT NULL DEFAULT '',
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),
    closed_at                   timestamptz,
    CHECK ((expected_value_amount_minor IS NULL) = (expected_value_currency IS NULL)),
    -- A lost opportunity without a reason teaches nobody anything.
    CHECK (stage <> 'LOST' OR (loss_reason IS NOT NULL AND length(btrim(loss_reason)) > 0))
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_opportunities_cursor
    ON fleet_partner_sales_opportunities (organization_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_crm_opportunities_stage
    ON fleet_partner_sales_opportunities (organization_id, stage, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_crm_opportunities_customer
    ON fleet_partner_sales_opportunities (organization_id, customer_id, created_at DESC);

CREATE TABLE IF NOT EXISTS fleet_partner_sales_opportunity_history (
    id               text PRIMARY KEY,
    organization_id  text NOT NULL,
    opportunity_id   text NOT NULL
                     REFERENCES fleet_partner_sales_opportunities (id) ON DELETE RESTRICT,
    previous_stage   text NOT NULL,
    new_stage        text NOT NULL,
    reason_code      text NOT NULL DEFAULT '',
    actor_type       text NOT NULL,
    actor_id         text NOT NULL,
    correlation_id   text NOT NULL DEFAULT '',
    occurred_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_opportunity_history
    ON fleet_partner_sales_opportunity_history (opportunity_id, occurred_at);

CREATE TRIGGER trg_fleet_crm_opportunity_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_sales_opportunity_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Quotations
-- ---------------------------------------------------------------------------
--
-- Pricing is manually supplied. There is no rule table, no rate card and no
-- formula: a fleet operator's commercial terms are theirs, and a hidden
-- calculation would silently change what a customer was quoted.

CREATE TABLE IF NOT EXISTS fleet_partner_quotations (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id           text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    customer_id         text NOT NULL REFERENCES fleet_partner_company_customers (id) ON DELETE RESTRICT,
    opportunity_id      text REFERENCES fleet_partner_sales_opportunities (id) ON DELETE RESTRICT,
    quotation_number    text NOT NULL,
    currency            char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    valid_from          timestamptz NOT NULL,
    valid_until         timestamptz NOT NULL,
    service_scope       jsonb NOT NULL DEFAULT '{}'::jsonb,
    terms               jsonb NOT NULL DEFAULT '{}'::jsonb,
    status              text NOT NULL DEFAULT 'DRAFT'
                        CHECK (status IN ('DRAFT', 'AWAITING_APPROVAL', 'APPROVED', 'ISSUED', 'VIEWED',
                                          'ACCEPTED', 'REJECTED', 'EXPIRED', 'CANCELLED', 'SUPERSEDED')),
    current_version     integer NOT NULL DEFAULT 1 CHECK (current_version >= 1),
    issued_version_id   text,
    accepted_version_id text,
    approval_request_id text,
    superseded_by_id    text REFERENCES fleet_partner_quotations (id) ON DELETE RESTRICT,
    total_amount_minor  bigint NOT NULL DEFAULT 0 CHECK (total_amount_minor >= 0),
    version             bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by          text NOT NULL,
    updated_by          text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    CHECK (valid_until > valid_from)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_crm_quotations_number
    ON fleet_partner_quotations (organization_id, quotation_number);
CREATE INDEX IF NOT EXISTS idx_fleet_crm_quotations_cursor
    ON fleet_partner_quotations (organization_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_crm_quotations_status
    ON fleet_partner_quotations (organization_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_crm_quotations_customer
    ON fleet_partner_quotations (organization_id, customer_id, created_at DESC);
-- Expiry worker lookup.
CREATE INDEX IF NOT EXISTS idx_fleet_crm_quotations_expiry
    ON fleet_partner_quotations (valid_until)
    WHERE status IN ('ISSUED', 'VIEWED');

CREATE TABLE IF NOT EXISTS fleet_partner_quotation_versions (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL,
    quotation_id        text NOT NULL REFERENCES fleet_partner_quotations (id) ON DELETE RESTRICT,
    version_number      integer NOT NULL CHECK (version_number >= 1),
    customer_snapshot   jsonb NOT NULL,
    contact_snapshot    jsonb NOT NULL DEFAULT '{}'::jsonb,
    currency            char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    line_items_snapshot jsonb NOT NULL,
    service_scope_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    terms_snapshot      jsonb NOT NULL DEFAULT '{}'::jsonb,
    total_amount_minor  bigint NOT NULL CHECK (total_amount_minor >= 0),
    valid_from          timestamptz NOT NULL,
    valid_until         timestamptz NOT NULL,
    document_references jsonb NOT NULL DEFAULT '[]'::jsonb,
    approval_request_id text,
    issued_by           text NOT NULL,
    issued_at           timestamptz NOT NULL DEFAULT now(),
    content_fingerprint text NOT NULL,
    UNIQUE (quotation_id, version_number)
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_quotation_versions_quotation
    ON fleet_partner_quotation_versions (organization_id, quotation_id, version_number DESC);

-- An issued version is a promise made to a customer. It is append-only at the
-- database level so no later edit, migration or repair script can restate what
-- was offered.
CREATE TRIGGER trg_fleet_crm_quotation_versions_immutable
    BEFORE UPDATE OR DELETE ON fleet_partner_quotation_versions
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE IF NOT EXISTS fleet_partner_quotation_line_items (
    id                        text PRIMARY KEY,
    organization_id           text NOT NULL,
    quotation_id              text NOT NULL REFERENCES fleet_partner_quotations (id) ON DELETE RESTRICT,
    description               text NOT NULL CHECK (length(btrim(description)) > 0),
    -- Decimal, never float: a quantity of 0.1 tonnes must stay 0.1.
    quantity                  numeric(18, 6) NOT NULL CHECK (quantity > 0),
    unit_type                 text NOT NULL,
    unit_amount_minor         bigint NOT NULL CHECK (unit_amount_minor >= 0),
    line_amount_minor         bigint NOT NULL CHECK (line_amount_minor >= 0),
    currency                  char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    external_pricing_reference text,
    manual_reason             text NOT NULL DEFAULT '',
    sequence                  integer NOT NULL CHECK (sequence >= 0),
    version                   bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now(),
    UNIQUE (quotation_id, sequence)
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_quotation_lines
    ON fleet_partner_quotation_line_items (organization_id, quotation_id, sequence);

CREATE TABLE IF NOT EXISTS fleet_partner_quotation_acceptances (
    id                   text PRIMARY KEY,
    organization_id      text NOT NULL,
    quotation_id         text NOT NULL REFERENCES fleet_partner_quotations (id) ON DELETE RESTRICT,
    accepted_version_id  text NOT NULL
                         REFERENCES fleet_partner_quotation_versions (id) ON DELETE RESTRICT,
    actor_type           text NOT NULL
                         CHECK (actor_type IN ('CUSTOMER', 'ORGANIZATION_USER', 'EXTERNAL_INTEGRATION', 'SYSTEM')),
    actor_reference      text NOT NULL DEFAULT '',
    acceptance_method    text NOT NULL
                         CHECK (acceptance_method IN ('SECURE_CUSTOMER_TOKEN', 'AUTHORIZED_COMPANY_USER',
                                                      'APPROVED_EXTERNAL_INTEGRATION',
                                                      'DOCUMENTED_OFFLINE_ACCEPTANCE')),
    accepted_at          timestamptz NOT NULL DEFAULT now(),
    -- Hashes, not values. An acceptance record is evidence, not a tracking log.
    ip_hash              text,
    user_agent_hash      text,
    evidence_document_reference text,
    idempotency_key      text NOT NULL,
    request_fingerprint  text NOT NULL,
    correlation_id       text NOT NULL DEFAULT '',
    created_at           timestamptz NOT NULL DEFAULT now()
);

-- One acceptance per quotation. A second acceptance is either a replay (which
-- returns the first) or a conflict (which is refused).
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_crm_acceptance_quotation
    ON fleet_partner_quotation_acceptances (quotation_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_crm_acceptance_idempotency
    ON fleet_partner_quotation_acceptances (organization_id, idempotency_key);

CREATE TRIGGER trg_fleet_crm_acceptances_immutable
    BEFORE UPDATE OR DELETE ON fleet_partner_quotation_acceptances
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- Conversion intent. This is NOT an order.
--
-- It records that an accepted quotation is eligible to become private work
-- once Section 15 exists. It mints no order ID, reserves no capacity, creates
-- no trip and moves no money.
CREATE TABLE IF NOT EXISTS fleet_partner_quotation_conversion_intents (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    quotation_id        text NOT NULL REFERENCES fleet_partner_quotations (id) ON DELETE RESTRICT,
    accepted_version_id text NOT NULL
                        REFERENCES fleet_partner_quotation_versions (id) ON DELETE RESTRICT,
    acceptance_id       text NOT NULL
                        REFERENCES fleet_partner_quotation_acceptances (id) ON DELETE RESTRICT,
    conversion_type     text NOT NULL DEFAULT 'FUTURE_PRIVATE_ORDER'
                        CHECK (conversion_type = 'FUTURE_PRIVATE_ORDER'),
    status              text NOT NULL DEFAULT 'PENDING_SECTION_15'
                        CHECK (status = 'PENDING_SECTION_15'),
    idempotency_key     text NOT NULL,
    fingerprint         text NOT NULL,
    correlation_id      text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_crm_conversion_intent_version
    ON fleet_partner_quotation_conversion_intents (accepted_version_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_crm_conversion_intent_idempotency
    ON fleet_partner_quotation_conversion_intents (organization_id, idempotency_key);

-- ---------------------------------------------------------------------------
-- Contracts
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS fleet_partner_contracts (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    branch_id           text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,
    customer_id         text NOT NULL REFERENCES fleet_partner_company_customers (id) ON DELETE RESTRICT,
    quotation_id        text REFERENCES fleet_partner_quotations (id) ON DELETE RESTRICT,
    contract_number     text NOT NULL,
    effective_from      timestamptz NOT NULL,
    effective_to        timestamptz,
    renewal_terms       jsonb NOT NULL DEFAULT '{}'::jsonb,
    service_scope       jsonb NOT NULL DEFAULT '{}'::jsonb,
    commercial_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    termination_terms   jsonb NOT NULL DEFAULT '{}'::jsonb,
    status              text NOT NULL DEFAULT 'DRAFT'
                        CHECK (status IN ('DRAFT', 'AWAITING_APPROVAL', 'APPROVED', 'ACTIVE', 'SUSPENDED',
                                          'EXPIRED', 'TERMINATED', 'RENEWAL_PENDING', 'SUPERSEDED')),
    current_version     integer NOT NULL DEFAULT 1 CHECK (current_version >= 1),
    active_version_id   text,
    approval_request_id text,
    supersedes_id       text REFERENCES fleet_partner_contracts (id) ON DELETE RESTRICT,
    version             bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by          text NOT NULL,
    updated_by          text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    terminated_at       timestamptz,
    CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_crm_contracts_number
    ON fleet_partner_contracts (organization_id, contract_number);
CREATE INDEX IF NOT EXISTS idx_fleet_crm_contracts_cursor
    ON fleet_partner_contracts (organization_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_crm_contracts_status
    ON fleet_partner_contracts (organization_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_crm_contracts_customer
    ON fleet_partner_contracts (organization_id, customer_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_crm_contracts_expiry
    ON fleet_partner_contracts (effective_to)
    WHERE status = 'ACTIVE' AND effective_to IS NOT NULL;

CREATE TABLE IF NOT EXISTS fleet_partner_contract_versions (
    id                     text PRIMARY KEY,
    organization_id        text NOT NULL,
    contract_id            text NOT NULL REFERENCES fleet_partner_contracts (id) ON DELETE RESTRICT,
    version_number         integer NOT NULL CHECK (version_number >= 1),
    customer_snapshot      jsonb NOT NULL,
    service_scope_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    commercial_snapshot    jsonb NOT NULL DEFAULT '{}'::jsonb,
    effective_from         timestamptz NOT NULL,
    effective_to           timestamptz,
    renewal_terms          jsonb NOT NULL DEFAULT '{}'::jsonb,
    termination_terms      jsonb NOT NULL DEFAULT '{}'::jsonb,
    document_references    jsonb NOT NULL DEFAULT '[]'::jsonb,
    approval_request_id    text,
    signatory_references   jsonb NOT NULL DEFAULT '[]'::jsonb,
    content_fingerprint    text NOT NULL,
    activated_by           text NOT NULL,
    activated_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (contract_id, version_number)
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_contract_versions_contract
    ON fleet_partner_contract_versions (organization_id, contract_id, version_number DESC);

CREATE TRIGGER trg_fleet_crm_contract_versions_immutable
    BEFORE UPDATE OR DELETE ON fleet_partner_contract_versions
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- Media references only. No bytes, no URLs, no storage paths, no credentials.
CREATE TABLE IF NOT EXISTS fleet_partner_contract_documents (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    contract_id         text NOT NULL REFERENCES fleet_partner_contracts (id) ON DELETE RESTRICT,
    contract_version_id text REFERENCES fleet_partner_contract_versions (id) ON DELETE RESTRICT,
    media_object_id     text NOT NULL,
    document_purpose    text NOT NULL
                        CHECK (document_purpose IN ('DRAFT_CONTRACT', 'SIGNED_CONTRACT', 'SUPPORTING',
                                                    'TERMINATION_EVIDENCE', 'RENEWAL')),
    is_mandatory        boolean NOT NULL DEFAULT false,
    status_projection   text NOT NULL DEFAULT 'UNKNOWN'
                        CHECK (status_projection IN ('PENDING', 'READY', 'UNSAFE', 'PENDING_REVIEW',
                                                     'REVOKED', 'DELETED', 'REPLACED', 'UNKNOWN')),
    readiness_state     text NOT NULL DEFAULT 'UNKNOWN'
                        CHECK (readiness_state IN ('READY', 'NOT_READY', 'UNKNOWN', 'STALE')),
    retention_reference text NOT NULL DEFAULT '',
    source_service      text NOT NULL DEFAULT 'media-file-service',
    source_version      bigint NOT NULL DEFAULT 0,
    last_event_id       text NOT NULL DEFAULT '',
    synced_at           timestamptz,
    version             bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by          text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, contract_id, media_object_id, document_purpose)
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_contract_documents_contract
    ON fleet_partner_contract_documents (organization_id, contract_id, document_purpose);

CREATE TABLE IF NOT EXISTS fleet_partner_contract_history (
    id               text PRIMARY KEY,
    organization_id  text NOT NULL,
    contract_id      text NOT NULL REFERENCES fleet_partner_contracts (id) ON DELETE RESTRICT,
    previous_status  text NOT NULL,
    new_status       text NOT NULL,
    reason_code      text NOT NULL DEFAULT '',
    actor_type       text NOT NULL,
    actor_id         text NOT NULL,
    correlation_id   text NOT NULL DEFAULT '',
    occurred_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_contract_history
    ON fleet_partner_contract_history (contract_id, occurred_at);

CREATE TRIGGER trg_fleet_crm_contract_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_contract_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Service level agreements
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS fleet_partner_service_level_agreements (
    id                            text PRIMARY KEY,
    organization_id               text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    customer_id                   text NOT NULL
                                  REFERENCES fleet_partner_company_customers (id) ON DELETE RESTRICT,
    contract_id                   text REFERENCES fleet_partner_contracts (id) ON DELETE RESTRICT,
    -- Opaque forward reference. Section 15 owns the catalogue; nothing here
    -- resolves this value.
    future_service_catalog_item_id text,
    metric_code                   text NOT NULL,
    target_value                  numeric(18, 6) NOT NULL,
    measurement_unit              text NOT NULL,
    measurement_window            text NOT NULL,
    escalation_policy             jsonb NOT NULL DEFAULT '{}'::jsonb,
    effective_from                timestamptz NOT NULL,
    effective_to                  timestamptz,
    status                        text NOT NULL DEFAULT 'DRAFT'
                                  CHECK (status IN ('DRAFT', 'ACTIVE', 'SUSPENDED', 'EXPIRED',
                                                    'TERMINATED', 'SUPERSEDED')),
    version                       bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by                    text NOT NULL,
    updated_by                    text NOT NULL DEFAULT '',
    created_at                    timestamptz NOT NULL DEFAULT now(),
    updated_at                    timestamptz NOT NULL DEFAULT now(),
    CHECK (effective_to IS NULL OR effective_to > effective_from)
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_sla_cursor
    ON fleet_partner_service_level_agreements (organization_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_fleet_crm_sla_customer
    ON fleet_partner_service_level_agreements (organization_id, customer_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_crm_sla_active_metric
    ON fleet_partner_service_level_agreements (organization_id, customer_id, contract_id, metric_code)
    WHERE status = 'ACTIVE' AND contract_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS fleet_partner_sla_observations (
    id               text PRIMARY KEY,
    organization_id  text NOT NULL,
    sla_id           text NOT NULL
                     REFERENCES fleet_partner_service_level_agreements (id) ON DELETE RESTRICT,
    source_type      text NOT NULL,
    source_reference text NOT NULL DEFAULT '',
    measured_value   numeric(18, 6),
    measurement_unit text NOT NULL DEFAULT '',
    measured_at      timestamptz NOT NULL DEFAULT now(),
    window_start     timestamptz NOT NULL,
    window_end       timestamptz NOT NULL,
    -- UNKNOWN is a first-class outcome and is never collapsed into MET. An SLA
    -- that could not be measured has not been met; it has not been measured.
    outcome          text NOT NULL
                     CHECK (outcome IN ('MET', 'MISSED', 'NOT_EVALUATED', 'INSUFFICIENT_DATA', 'UNKNOWN')),
    source_version   bigint NOT NULL DEFAULT 0,
    correlation_id   text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    CHECK (window_end >= window_start)
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_sla_observations
    ON fleet_partner_sla_observations (organization_id, sla_id, measured_at DESC, id DESC);

CREATE TRIGGER trg_fleet_crm_sla_observations_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_sla_observations
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Customer credit profiles
-- ---------------------------------------------------------------------------
--
-- Policy only. There is no receivables ledger, no debt calculation and no
-- aging: Fleet does not own what a customer owes, and pretending to would
-- produce a second, wrong set of books.

CREATE TABLE IF NOT EXISTS fleet_partner_customer_credit_profiles (
    id                       text PRIMARY KEY,
    organization_id          text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    customer_id              text NOT NULL
                             REFERENCES fleet_partner_company_customers (id) ON DELETE RESTRICT,
    credit_limit_amount_minor bigint CHECK (credit_limit_amount_minor IS NULL OR credit_limit_amount_minor >= 0),
    currency                 char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    payment_terms_days       integer NOT NULL DEFAULT 0 CHECK (payment_terms_days >= 0),
    hold_status              text NOT NULL DEFAULT 'NO_HOLD'
                             CHECK (hold_status IN ('NO_HOLD', 'REVIEW_REQUIRED', 'ON_HOLD', 'RESTRICTED')),
    hold_reason              text,
    risk_status_snapshot     text NOT NULL DEFAULT 'NOT_ASSESSED',
    approved_by              text,
    approval_request_id      text,
    effective_from           timestamptz NOT NULL DEFAULT now(),
    effective_to             timestamptz,
    version                  bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by               text NOT NULL,
    updated_by               text NOT NULL DEFAULT '',
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, customer_id)
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_credit_hold
    ON fleet_partner_customer_credit_profiles (organization_id, hold_status);

CREATE TABLE IF NOT EXISTS fleet_partner_customer_credit_history (
    id                 text PRIMARY KEY,
    organization_id    text NOT NULL,
    credit_profile_id  text NOT NULL
                       REFERENCES fleet_partner_customer_credit_profiles (id) ON DELETE RESTRICT,
    customer_id        text NOT NULL,
    previous_hold_status text NOT NULL,
    new_hold_status    text NOT NULL,
    previous_limit_minor bigint,
    new_limit_minor    bigint,
    reason_code        text NOT NULL DEFAULT '',
    actor_type         text NOT NULL,
    actor_id           text NOT NULL,
    approval_request_id text,
    correlation_id     text NOT NULL DEFAULT '',
    occurred_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_credit_history
    ON fleet_partner_customer_credit_history (credit_profile_id, occurred_at);

CREATE TRIGGER trg_fleet_crm_credit_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_customer_credit_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- CRM notification delivery operations
-- ---------------------------------------------------------------------------
--
-- A quotation is issued or a contract is activated inside one transaction. The
-- notification is not part of that transaction: a notification outage must not
-- roll back a commercial commitment that has already been made. The intent is
-- recorded here and retried durably instead.

CREATE TABLE IF NOT EXISTS fleet_partner_crm_notification_operations (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    subject_type        text NOT NULL,
    subject_id          text NOT NULL,
    intent_type         text NOT NULL,
    recipient_reference text NOT NULL DEFAULT '',
    channel             text NOT NULL DEFAULT 'EMAIL',
    payload             jsonb NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key     text NOT NULL,
    status              text NOT NULL DEFAULT 'PENDING'
                        CHECK (status IN ('PENDING', 'PROCESSING', 'UNKNOWN', 'DELIVERED',
                                          'FAILED_RETRYABLE', 'FAILED_PERMANENT', 'DEAD_LETTER')),
    attempts            integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at     timestamptz NOT NULL DEFAULT now(),
    lease_owner         text,
    lease_expires_at    timestamptz,
    delivery_reference  text NOT NULL DEFAULT '',
    safe_error_code     text NOT NULL DEFAULT '',
    correlation_id      text NOT NULL DEFAULT '',
    version             bigint NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_notifications_due
    ON fleet_partner_crm_notification_operations (status, next_attempt_at)
    WHERE status IN ('PENDING', 'FAILED_RETRYABLE', 'UNKNOWN');
CREATE INDEX IF NOT EXISTS idx_fleet_crm_notifications_lease
    ON fleet_partner_crm_notification_operations (lease_expires_at)
    WHERE lease_owner IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_fleet_crm_notifications_subject
    ON fleet_partner_crm_notification_operations (organization_id, subject_type, subject_id);

-- ---------------------------------------------------------------------------
-- Deferred foreign keys resolved after both sides exist
-- ---------------------------------------------------------------------------

ALTER TABLE fleet_partner_company_customers
    ADD CONSTRAINT fleet_partner_company_customers_credit_profile_fkey
    FOREIGN KEY (credit_profile_id)
    REFERENCES fleet_partner_customer_credit_profiles (id) ON DELETE RESTRICT;

ALTER TABLE fleet_partner_quotations
    ADD CONSTRAINT fleet_partner_quotations_issued_version_fkey
    FOREIGN KEY (issued_version_id)
    REFERENCES fleet_partner_quotation_versions (id) ON DELETE RESTRICT;

ALTER TABLE fleet_partner_quotations
    ADD CONSTRAINT fleet_partner_quotations_accepted_version_fkey
    FOREIGN KEY (accepted_version_id)
    REFERENCES fleet_partner_quotation_versions (id) ON DELETE RESTRICT;

ALTER TABLE fleet_partner_contracts
    ADD CONSTRAINT fleet_partner_contracts_active_version_fkey
    FOREIGN KEY (active_version_id)
    REFERENCES fleet_partner_contract_versions (id) ON DELETE RESTRICT;

-- ---------------------------------------------------------------------------
-- Permissions
-- ---------------------------------------------------------------------------

INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('crm.customers.read',      'crm', 'Read organization CRM customers',              true),
    ('crm.customers.manage',    'crm', 'Create, update and archive CRM customers',     true),
    ('crm.contacts.read',       'crm', 'Read customer contacts',                       true),
    ('crm.contacts.manage',     'crm', 'Create, update and archive customer contacts', true),
    ('crm.locations.read',      'crm', 'Read customer locations',                      true),
    ('crm.locations.manage',    'crm', 'Create, update and archive customer locations', true),
    ('crm.opportunities.read',  'crm', 'Read sales opportunities',                     true),
    ('crm.opportunities.manage','crm', 'Create and update sales opportunities',        true),
    ('crm.quotations.read',     'crm', 'Read quotations',                              true),
    ('crm.quotations.manage',   'crm', 'Create, update and cancel quotations',         true),
    ('crm.quotations.submit',   'crm', 'Submit a quotation for approval',              true),
    ('crm.quotations.issue',    'crm', 'Issue an approved quotation to a customer',    true),
    ('crm.quotations.accept',   'crm', 'Record a quotation acceptance',                true),
    ('crm.contracts.read',      'crm', 'Read contracts',                               true),
    ('crm.contracts.manage',    'crm', 'Create, update, suspend and renew contracts',  true),
    ('crm.contracts.activate',  'crm', 'Activate a contract version',                  true),
    ('crm.contracts.terminate', 'crm', 'Terminate a contract',                         true),
    ('crm.sla.read',            'crm', 'Read service level agreements and observations', true),
    ('crm.sla.manage',          'crm', 'Create and update service level agreements',   true),
    ('crm.sla.activate',        'crm', 'Activate or suspend a service level agreement', true),
    ('crm.credit.read',         'crm', 'Read customer credit policy',                  true),
    ('crm.credit.manage',       'crm', 'Change customer credit limits and holds',      true),
    ('crm.exports.create',      'crm', 'Create a redacted CRM export',                 true),
    -- Sensitive CRM data is a separate grant from ordinary CRM read. Someone
    -- who may see that a customer exists does not thereby get their phone
    -- number, tax reference or a site's access instructions.
    ('crm.sensitive.read',      'crm', 'Read unmasked CRM contact, tax and site access data', true)
ON CONFLICT (code) DO UPDATE
    SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_owner', code FROM fleet_partner_permissions
WHERE code LIKE 'crm.%'
ON CONFLICT DO NOTHING;

-- The customer relationship manager runs CRM day to day but does not set
-- credit policy: extending credit is a finance decision.
INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_crm', code FROM fleet_partner_permissions
WHERE code IN ('crm.customers.read', 'crm.customers.manage', 'crm.contacts.read',
               'crm.contacts.manage', 'crm.locations.read', 'crm.locations.manage',
               'crm.opportunities.read', 'crm.opportunities.manage', 'crm.quotations.read',
               'crm.quotations.manage', 'crm.quotations.submit', 'crm.quotations.issue',
               'crm.quotations.accept', 'crm.contracts.read', 'crm.contracts.manage',
               'crm.sla.read', 'crm.sla.manage', 'crm.credit.read', 'crm.sensitive.read')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_finance', code FROM fleet_partner_permissions
WHERE code IN ('crm.customers.read', 'crm.quotations.read', 'crm.contracts.read',
               'crm.credit.read', 'crm.credit.manage', 'crm.exports.create')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_admin', code FROM fleet_partner_permissions
WHERE code IN ('crm.customers.read', 'crm.contacts.read', 'crm.locations.read',
               'crm.opportunities.read', 'crm.quotations.read', 'crm.contracts.read',
               'crm.contracts.activate', 'crm.contracts.terminate', 'crm.sla.read',
               'crm.sla.activate', 'crm.credit.read')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_operations', code FROM fleet_partner_permissions
WHERE code IN ('crm.customers.read', 'crm.locations.read', 'crm.contracts.read', 'crm.sla.read')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_analyst', code FROM fleet_partner_permissions
WHERE code IN ('crm.customers.read', 'crm.opportunities.read', 'crm.quotations.read',
               'crm.contracts.read', 'crm.sla.read')
ON CONFLICT DO NOTHING;
