-- Section 14 completion: the structures the Go service layer needs that
-- `000023_company_crm` did not create.
--
-- `000023` is applied and is not rewritten. Everything here is additive: new
-- append-only history tables, the quotation-side Media reference table, the
-- hashed customer access tokens, and a handful of columns the specification
-- requires that the first pass left out.
--
-- Section 15 remains out of scope: nothing here creates a service catalogue,
-- order, stop, shipment, package, manifest or recurring-order structure.

-- ---------------------------------------------------------------------------
-- CRM histories
-- ---------------------------------------------------------------------------
--
-- `000023` gave opportunities and contracts an append-only history but left
-- customers, contacts, locations, quotations and SLAs without one. A status or
-- risk change with no immutable record is a change nobody can later account
-- for, so each gets the same treatment: append-only, trigger enforced.

CREATE TABLE IF NOT EXISTS fleet_partner_crm_customer_history (
    id                   text PRIMARY KEY,
    organization_id      text NOT NULL,
    customer_id          text NOT NULL
                         REFERENCES fleet_partner_company_customers (id) ON DELETE RESTRICT,
    change_type          text NOT NULL
                         CHECK (change_type IN ('CREATED', 'UPDATED', 'STATUS_CHANGED',
                                                'RISK_CHANGED', 'ARCHIVED')),
    previous_status      text NOT NULL DEFAULT '',
    new_status           text NOT NULL DEFAULT '',
    previous_risk_status text NOT NULL DEFAULT '',
    new_risk_status      text NOT NULL DEFAULT '',
    -- Changed field names only. The values are not repeated here: a history row
    -- that copies a tax reference turns an audit trail into a second, less
    -- guarded, copy of the PII it is meant to account for.
    changed_fields       jsonb NOT NULL DEFAULT '[]'::jsonb,
    reason_code          text NOT NULL DEFAULT '',
    approval_request_id  text,
    actor_type           text NOT NULL,
    actor_id             text NOT NULL,
    correlation_id       text NOT NULL DEFAULT '',
    causation_id         text NOT NULL DEFAULT '',
    occurred_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_customer_history
    ON fleet_partner_crm_customer_history (organization_id, customer_id, occurred_at DESC, id DESC);

CREATE TRIGGER trg_fleet_crm_customer_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_crm_customer_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE IF NOT EXISTS fleet_partner_crm_contact_history (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    customer_id     text NOT NULL,
    contact_id      text NOT NULL
                    REFERENCES fleet_partner_customer_contacts (id) ON DELETE RESTRICT,
    change_type     text NOT NULL
                    CHECK (change_type IN ('CREATED', 'UPDATED', 'ARCHIVED', 'PRIMARY_CHANGED')),
    previous_status text NOT NULL DEFAULT '',
    new_status      text NOT NULL DEFAULT '',
    changed_fields  jsonb NOT NULL DEFAULT '[]'::jsonb,
    reason_code     text NOT NULL DEFAULT '',
    actor_type      text NOT NULL,
    actor_id        text NOT NULL,
    correlation_id  text NOT NULL DEFAULT '',
    causation_id    text NOT NULL DEFAULT '',
    occurred_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_contact_history
    ON fleet_partner_crm_contact_history (organization_id, contact_id, occurred_at DESC, id DESC);

CREATE TRIGGER trg_fleet_crm_contact_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_crm_contact_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE IF NOT EXISTS fleet_partner_crm_location_history (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    customer_id     text NOT NULL,
    location_id     text NOT NULL
                    REFERENCES fleet_partner_customer_locations (id) ON DELETE RESTRICT,
    change_type     text NOT NULL
                    CHECK (change_type IN ('CREATED', 'UPDATED', 'ARCHIVED')),
    previous_status text NOT NULL DEFAULT '',
    new_status      text NOT NULL DEFAULT '',
    changed_fields  jsonb NOT NULL DEFAULT '[]'::jsonb,
    reason_code     text NOT NULL DEFAULT '',
    actor_type      text NOT NULL,
    actor_id        text NOT NULL,
    correlation_id  text NOT NULL DEFAULT '',
    causation_id    text NOT NULL DEFAULT '',
    occurred_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_location_history
    ON fleet_partner_crm_location_history (organization_id, location_id, occurred_at DESC, id DESC);

CREATE TRIGGER trg_fleet_crm_location_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_crm_location_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE IF NOT EXISTS fleet_partner_quotation_history (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL,
    quotation_id        text NOT NULL
                        REFERENCES fleet_partner_quotations (id) ON DELETE RESTRICT,
    previous_status     text NOT NULL,
    new_status          text NOT NULL,
    quotation_version   integer NOT NULL DEFAULT 1,
    reason_code         text NOT NULL DEFAULT '',
    approval_request_id text,
    -- CUSTOMER covers the secure-token flows. The actor reference for those is
    -- the token record ID, never the raw token and never an IP address.
    actor_type          text NOT NULL,
    actor_id            text NOT NULL DEFAULT '',
    correlation_id      text NOT NULL DEFAULT '',
    causation_id        text NOT NULL DEFAULT '',
    occurred_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_quotation_history
    ON fleet_partner_quotation_history (organization_id, quotation_id, occurred_at DESC, id DESC);

CREATE TRIGGER trg_fleet_crm_quotation_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_quotation_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE IF NOT EXISTS fleet_partner_sla_history (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL,
    sla_id              text NOT NULL
                        REFERENCES fleet_partner_service_level_agreements (id) ON DELETE RESTRICT,
    previous_status     text NOT NULL,
    new_status          text NOT NULL,
    reason_code         text NOT NULL DEFAULT '',
    approval_request_id text,
    actor_type          text NOT NULL,
    actor_id            text NOT NULL,
    correlation_id      text NOT NULL DEFAULT '',
    causation_id        text NOT NULL DEFAULT '',
    occurred_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_sla_history
    ON fleet_partner_sla_history (organization_id, sla_id, occurred_at DESC, id DESC);

CREATE TRIGGER trg_fleet_crm_sla_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_sla_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Quotation documents
-- ---------------------------------------------------------------------------
--
-- Contracts got a Media reference table in `000023`; quotations did not, yet
-- issue must fail closed on an unsafe or unready quotation document. Same
-- shape, same rule: opaque object ID, purpose, projection. No bytes, no URL,
-- no storage path, no checksum, no credential.

CREATE TABLE IF NOT EXISTS fleet_partner_quotation_documents (
    id                   text PRIMARY KEY,
    organization_id      text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    quotation_id         text NOT NULL REFERENCES fleet_partner_quotations (id) ON DELETE RESTRICT,
    quotation_version_id text REFERENCES fleet_partner_quotation_versions (id) ON DELETE RESTRICT,
    media_object_id      text NOT NULL,
    document_purpose     text NOT NULL
                         CHECK (document_purpose IN ('QUOTATION', 'SUPPORTING', 'ACCEPTANCE_EVIDENCE',
                                                     'REJECTION_EVIDENCE')),
    is_mandatory         boolean NOT NULL DEFAULT false,
    status_projection    text NOT NULL DEFAULT 'UNKNOWN'
                         CHECK (status_projection IN ('PENDING', 'READY', 'UNSAFE', 'PENDING_REVIEW',
                                                      'REVOKED', 'DELETED', 'REPLACED', 'UNKNOWN')),
    readiness_state      text NOT NULL DEFAULT 'UNKNOWN'
                         CHECK (readiness_state IN ('READY', 'NOT_READY', 'UNKNOWN', 'STALE')),
    retention_reference  text NOT NULL DEFAULT '',
    source_service       text NOT NULL DEFAULT 'media-file-service',
    source_version       bigint NOT NULL DEFAULT 0,
    last_event_id        text NOT NULL DEFAULT '',
    synced_at            timestamptz,
    version              bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_by           text NOT NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, quotation_id, media_object_id, document_purpose)
);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_quotation_documents
    ON fleet_partner_quotation_documents (organization_id, quotation_id, document_purpose);

-- ---------------------------------------------------------------------------
-- Secure quotation customer access tokens
-- ---------------------------------------------------------------------------
--
-- The raw token exists once, in the response that carries it to the customer.
-- What is stored is a hash, scoped to one organization, one quotation and one
-- issued version, with an expiry. A database dump therefore cannot be replayed
-- as a customer.

CREATE TABLE IF NOT EXISTS fleet_partner_quotation_access_tokens (
    id                   text PRIMARY KEY,
    organization_id      text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    quotation_id         text NOT NULL REFERENCES fleet_partner_quotations (id) ON DELETE RESTRICT,
    quotation_version_id text NOT NULL
                         REFERENCES fleet_partner_quotation_versions (id) ON DELETE RESTRICT,
    token_hash           text NOT NULL,
    scope                text NOT NULL DEFAULT 'VIEW_AND_RESPOND'
                         CHECK (scope IN ('VIEW_ONLY', 'VIEW_AND_RESPOND')),
    single_use           boolean NOT NULL DEFAULT false,
    status               text NOT NULL DEFAULT 'ACTIVE'
                         CHECK (status IN ('ACTIVE', 'CONSUMED', 'REVOKED')),
    issued_to_contact_id text REFERENCES fleet_partner_customer_contacts (id) ON DELETE RESTRICT,
    expires_at           timestamptz NOT NULL,
    first_viewed_at      timestamptz,
    last_viewed_at       timestamptz,
    view_count           integer NOT NULL DEFAULT 0 CHECK (view_count >= 0),
    revoked_reason       text NOT NULL DEFAULT '',
    created_by           text NOT NULL,
    version              bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

-- The hash is the lookup key: validation is a single indexed read that never
-- needs the quotation ID from the request to be trusted first.
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_crm_quotation_tokens_hash
    ON fleet_partner_quotation_access_tokens (token_hash);
CREATE INDEX IF NOT EXISTS idx_fleet_crm_quotation_tokens_quotation
    ON fleet_partner_quotation_access_tokens (organization_id, quotation_id, status);

-- ---------------------------------------------------------------------------
-- Columns the service layer requires
-- ---------------------------------------------------------------------------

-- An opportunity put ON_HOLD must say why, exactly as a lost one must.
ALTER TABLE fleet_partner_sales_opportunities
    ADD COLUMN IF NOT EXISTS hold_reason text;

-- Internal notes are staff-only and never reach the customer view; they live
-- on the quotation, not in the issued version snapshot.
ALTER TABLE fleet_partner_quotations
    ADD COLUMN IF NOT EXISTS internal_notes text NOT NULL DEFAULT '';
ALTER TABLE fleet_partner_quotations
    ADD COLUMN IF NOT EXISTS issued_at timestamptz;
ALTER TABLE fleet_partner_quotations
    ADD COLUMN IF NOT EXISTS resolved_at timestamptz;

ALTER TABLE fleet_partner_contracts
    ADD COLUMN IF NOT EXISTS activated_at timestamptz;

ALTER TABLE fleet_partner_customer_contacts
    ADD COLUMN IF NOT EXISTS consent_source text NOT NULL DEFAULT '';

ALTER TABLE fleet_partner_quotation_acceptances
    ADD COLUMN IF NOT EXISTS causation_id text NOT NULL DEFAULT '';
ALTER TABLE fleet_partner_quotation_conversion_intents
    ADD COLUMN IF NOT EXISTS causation_id text NOT NULL DEFAULT '';
ALTER TABLE fleet_partner_crm_notification_operations
    ADD COLUMN IF NOT EXISTS causation_id text NOT NULL DEFAULT '';
ALTER TABLE fleet_partner_crm_notification_operations
    ADD COLUMN IF NOT EXISTS dead_letter_reason text NOT NULL DEFAULT '';

-- Observation identity. Without the payload hash there is no way to tell a
-- duplicate delivery from a source that changed its mind about the same
-- window, and the two must not be answered the same way.
ALTER TABLE fleet_partner_sla_observations
    ADD COLUMN IF NOT EXISTS event_id text NOT NULL DEFAULT '';
ALTER TABLE fleet_partner_sla_observations
    ADD COLUMN IF NOT EXISTS payload_hash text NOT NULL DEFAULT '';
ALTER TABLE fleet_partner_sla_observations
    ADD COLUMN IF NOT EXISTS causation_id text NOT NULL DEFAULT '';

-- One observation per source, reference and version. A second delivery of the
-- same version is a duplicate when the payload matches and a conflict when it
-- does not; the unique index is what makes that distinction enforceable rather
-- than advisory.
CREATE UNIQUE INDEX IF NOT EXISTS idx_fleet_crm_sla_observation_source
    ON fleet_partner_sla_observations (sla_id, source_type, source_reference, source_version);

CREATE INDEX IF NOT EXISTS idx_fleet_crm_sla_observations_outcome
    ON fleet_partner_sla_observations (organization_id, sla_id, outcome, measured_at DESC);

-- Renewal-pending and expiry sweeps read these; without them both workers
-- degrade into full scans of every contract in the platform.
CREATE INDEX IF NOT EXISTS idx_fleet_crm_contracts_renewal
    ON fleet_partner_contracts (effective_to)
    WHERE status IN ('ACTIVE', 'SUSPENDED', 'RENEWAL_PENDING') AND effective_to IS NOT NULL;
