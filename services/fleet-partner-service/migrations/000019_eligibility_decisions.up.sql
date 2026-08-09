-- WP9.1: durable eligibility decisions.
--
-- An eligibility answer is a decision with a lifetime, not a boolean computed
-- inline and thrown away. Three properties make the difference:
--
--   1. Durability. The decision that authorized an assignment is still
--      readable months later, with the policy version it was taken under and
--      the authoritative source versions it was based on. A gate whose reasons
--      evaporate cannot be audited or disputed.
--   2. Expiry. Every decision states when it stops being usable. A vehicle
--      that was eligible last March is not thereby eligible today.
--   3. Fail-closed statuses. UNKNOWN and STALE are first-class outcomes, not
--      error paths. When an authority is unreachable, its data is older than
--      policy allows, or two sources disagree, the answer is recorded as
--      UNKNOWN or STALE — never as ELIGIBLE and never as a silent skip.
--
-- Only a decision that is current (not superseded), unexpired and ELIGIBLE may
-- authorize a sensitive operation. Everything else blocks.

-- ---------------------------------------------------------------------------
-- Controlled reason vocabulary
-- ---------------------------------------------------------------------------

-- Reason codes are a closed set held in a table with a foreign key, not free
-- text in a jsonb blob. Free text cannot be filtered on, cannot be translated,
-- and drifts into leaking personal data the moment someone writes a helpful
-- message into it.
CREATE TABLE IF NOT EXISTS fleet_partner_eligibility_reason_codes (
    code        text PRIMARY KEY,
    subject_type text NOT NULL CHECK (subject_type IN ('COMPANY', 'PROVIDER', 'VEHICLE', 'ANY')),
    category    text NOT NULL,
    description text NOT NULL,
    is_active   boolean NOT NULL DEFAULT true
);

INSERT INTO fleet_partner_eligibility_reason_codes (code, subject_type, category, description) VALUES
    -- Company
    ('company_not_found',              'COMPANY',  'lifecycle',    'No organization record exists in this tenant scope'),
    ('company_not_active',             'COMPANY',  'lifecycle',    'Organization lifecycle status is not ACTIVE'),
    ('company_suspended',              'COMPANY',  'lifecycle',    'Organization is suspended'),
    ('company_archived',               'COMPANY',  'lifecycle',    'Organization is archived or closed'),
    ('company_verification_pending',   'COMPANY',  'verification', 'Organization verification has not been approved'),
    ('company_verification_rejected',  'COMPANY',  'verification', 'Organization verification was rejected'),
    ('company_verification_unknown',   'COMPANY',  'verification', 'Organization verification outcome is unknown'),
    ('company_not_approved',           'COMPANY',  'approval',     'Organization onboarding approval is incomplete'),
    ('company_restricted',             'COMPANY',  'restriction',  'An active restriction blocks this operation'),
    ('company_capability_missing',     'COMPANY',  'capability',   'Organization has no approved capability for this service'),
    ('company_branch_invalid',         'COMPANY',  'branch',       'Operating branch is missing, inactive or archived'),
    ('company_document_missing',       'COMPANY',  'documents',    'A required organization document is absent'),
    ('company_document_expired',       'COMPANY',  'documents',    'A required organization document has expired'),
    ('company_document_rejected',      'COMPANY',  'documents',    'A required organization document was rejected'),
    ('company_subscription_inactive',  'COMPANY',  'subscription', 'No current active subscription projection'),
    ('company_subscription_stale',     'COMPANY',  'subscription', 'Subscription projection is not current'),
    -- Provider
    ('provider_not_found',             'PROVIDER', 'identity',     'No provider membership exists in this tenant scope'),
    ('provider_membership_not_active', 'PROVIDER', 'lifecycle',    'Provider membership is not ACTIVE'),
    ('provider_suspended',             'PROVIDER', 'restriction',  'Provider membership is suspended'),
    ('provider_not_linked',            'PROVIDER', 'identity',     'Provider membership carries no authoritative provider identity'),
    ('provider_unverified',            'PROVIDER', 'verification', 'Authoritative provider verification is not satisfied'),
    ('provider_service_mismatch',      'PROVIDER', 'capability',   'Provider service category does not match the operation'),
    ('provider_branch_mismatch',       'PROVIDER', 'branch',       'Provider is not linked to the operating branch'),
    ('provider_consent_missing',       'PROVIDER', 'consent',      'Provider has not accepted the organization association'),
    ('provider_not_available',         'PROVIDER', 'availability', 'Authoritative provider availability blocks the operation'),
    ('provider_shift_blocked',         'PROVIDER', 'availability', 'Provider is outside an eligible shift window'),
    ('provider_document_missing',      'PROVIDER', 'documents',    'A required provider document is absent'),
    ('provider_document_expired',      'PROVIDER', 'documents',    'A required provider document has expired'),
    ('provider_assignment_conflict',   'PROVIDER', 'conflict',     'Provider already holds conflicting live work'),
    ('provider_projection_stale',      'PROVIDER', 'freshness',    'Provider projection is stale, gapped or reconciling'),
    -- Vehicle
    ('vehicle_not_found',              'VEHICLE',  'identity',     'The vehicle authority does not know this vehicle'),
    ('vehicle_ownership_mismatch',     'VEHICLE',  'ownership',    'The vehicle is not controlled by this organization or provider'),
    ('vehicle_provider_mismatch',      'VEHICLE',  'ownership',    'The vehicle is not associated with the nominated provider'),
    ('vehicle_service_mismatch',       'VEHICLE',  'capability',   'Vehicle service type does not match the operation'),
    ('vehicle_unverified',             'VEHICLE',  'verification', 'Vehicle verification state is not satisfied'),
    ('vehicle_not_available',          'VEHICLE',  'availability', 'The vehicle is not available'),
    ('vehicle_restricted',             'VEHICLE',  'restriction',  'The vehicle is restricted or suspended'),
    ('vehicle_document_missing',       'VEHICLE',  'documents',    'A required vehicle document is absent'),
    ('vehicle_document_expired',       'VEHICLE',  'documents',    'A required vehicle document has expired'),
    ('vehicle_assignment_conflict',    'VEHICLE',  'conflict',     'The vehicle is already committed to live work'),
    -- Cross-cutting: these are the fail-closed vocabulary.
    ('authority_unavailable',          'ANY',      'freshness',    'An authoritative source could not be reached'),
    ('authority_not_configured',       'ANY',      'freshness',    'No authoritative client is configured for this check'),
    ('authority_conflict',             'ANY',      'freshness',    'Authoritative sources disagree'),
    ('source_version_stale',           'ANY',      'freshness',    'Authoritative data is older than policy allows'),
    ('decision_expired',               'ANY',      'expiry',       'The recorded decision has passed its expiry'),
    ('policy_not_found',               'ANY',      'policy',       'No active policy version governs this operation'),
    ('subject_out_of_scope',           'ANY',      'tenant',       'The subject does not belong to this tenant or branch')
ON CONFLICT (code) DO UPDATE
    SET subject_type = EXCLUDED.subject_type, category = EXCLUDED.category,
        description = EXCLUDED.description, is_active = true;

-- ---------------------------------------------------------------------------
-- Versioned eligibility policies
-- ---------------------------------------------------------------------------

-- A policy version is a row, never an edit. Publishing a stricter rule must
-- not retroactively change what an earlier decision meant, so historical
-- decisions keep pointing at the exact version they were taken under and that
-- version is immutable except for being archived.
CREATE TABLE IF NOT EXISTS fleet_partner_eligibility_policies (
    id                     text PRIMARY KEY,
    -- NULL organization means a platform default that every tenant inherits.
    organization_id        text REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    policy_key             text NOT NULL,
    operation_type         text NOT NULL,
    version                bigint NOT NULL CHECK (version >= 1),

    -- Which subject checks this operation requires. Absent means "not
    -- required"; it never means "assume pass".
    required_company_checks jsonb NOT NULL DEFAULT '[]'::jsonb,
    required_provider_checks jsonb NOT NULL DEFAULT '[]'::jsonb,
    required_vehicle_checks jsonb NOT NULL DEFAULT '[]'::jsonb,
    required_documents     jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- How long an ELIGIBLE answer may be reused before it must be recomputed.
    decision_ttl_seconds   integer NOT NULL DEFAULT 900 CHECK (decision_ttl_seconds > 0),
    -- How old an authoritative source version may be before the decision is
    -- recorded STALE instead of ELIGIBLE.
    source_max_age_seconds integer NOT NULL DEFAULT 900 CHECK (source_max_age_seconds > 0),

    -- Approval controls for the same operation, versioned alongside the
    -- eligibility rules so one row describes the whole gate.
    approval_required      boolean NOT NULL DEFAULT false,
    minimum_approvals      integer NOT NULL DEFAULT 1 CHECK (minimum_approvals >= 1),
    eligible_role_ids      jsonb NOT NULL DEFAULT '[]'::jsonb,
    self_approval_allowed  boolean NOT NULL DEFAULT false,
    separation_of_duties   boolean NOT NULL DEFAULT true,
    approval_ttl_seconds   integer NOT NULL DEFAULT 86400 CHECK (approval_ttl_seconds > 0),

    -- Operational requirements the same policy governs.
    acknowledgement_required boolean NOT NULL DEFAULT false,
    subscription_required    boolean NOT NULL DEFAULT false,
    -- Financial approval threshold in integer minor units. A NULL amount means
    -- the policy sets no financial threshold; it never means zero.
    financial_threshold_minor bigint CHECK (financial_threshold_minor IS NULL OR financial_threshold_minor >= 0),
    financial_threshold_currency char(3) CHECK (financial_threshold_currency IS NULL OR financial_threshold_currency ~ '^[A-Z]{3}$'),

    status                 text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ARCHIVED')),
    effective_from         timestamptz NOT NULL DEFAULT now(),
    archived_at            timestamptz,
    created_by_actor_type  text NOT NULL DEFAULT 'SYSTEM',
    created_by_actor_id    text NOT NULL DEFAULT '',
    created_at             timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_fleet_partner_eligibility_threshold_pair CHECK (
        (financial_threshold_minor IS NULL AND financial_threshold_currency IS NULL)
        OR (financial_threshold_minor IS NOT NULL AND financial_threshold_currency IS NOT NULL)
    )
);

-- One version number per policy per scope. Platform defaults (NULL
-- organization) and tenant overrides are separate series, so two partial
-- unique indexes rather than one that treats NULL as distinct.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_eligibility_policy_tenant_version
    ON fleet_partner_eligibility_policies (organization_id, policy_key, version)
    WHERE organization_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_eligibility_policy_default_version
    ON fleet_partner_eligibility_policies (policy_key, version)
    WHERE organization_id IS NULL;

-- At most one ACTIVE version per policy per scope: "the current policy" must
-- be a single unambiguous row, otherwise two concurrent decisions can be taken
-- under different rules and both look correct.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_eligibility_policy_tenant_active
    ON fleet_partner_eligibility_policies (organization_id, policy_key)
    WHERE organization_id IS NOT NULL AND status = 'ACTIVE';

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_eligibility_policy_default_active
    ON fleet_partner_eligibility_policies (policy_key)
    WHERE organization_id IS NULL AND status = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_fleet_partner_eligibility_policy_operation
    ON fleet_partner_eligibility_policies (operation_type, status);

-- A published policy version is immutable apart from archival. Anything else
-- would let a rule be rewritten under decisions that already cite it.
CREATE OR REPLACE FUNCTION fleet_partner_eligibility_policy_immutable() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'eligibility policy versions are immutable';
    END IF;
    IF NEW.id IS DISTINCT FROM OLD.id
        OR NEW.organization_id IS DISTINCT FROM OLD.organization_id
        OR NEW.policy_key IS DISTINCT FROM OLD.policy_key
        OR NEW.operation_type IS DISTINCT FROM OLD.operation_type
        OR NEW.version IS DISTINCT FROM OLD.version
        OR NEW.required_company_checks IS DISTINCT FROM OLD.required_company_checks
        OR NEW.required_provider_checks IS DISTINCT FROM OLD.required_provider_checks
        OR NEW.required_vehicle_checks IS DISTINCT FROM OLD.required_vehicle_checks
        OR NEW.required_documents IS DISTINCT FROM OLD.required_documents
        OR NEW.decision_ttl_seconds IS DISTINCT FROM OLD.decision_ttl_seconds
        OR NEW.source_max_age_seconds IS DISTINCT FROM OLD.source_max_age_seconds
        OR NEW.approval_required IS DISTINCT FROM OLD.approval_required
        OR NEW.minimum_approvals IS DISTINCT FROM OLD.minimum_approvals
        OR NEW.eligible_role_ids IS DISTINCT FROM OLD.eligible_role_ids
        OR NEW.self_approval_allowed IS DISTINCT FROM OLD.self_approval_allowed
        OR NEW.separation_of_duties IS DISTINCT FROM OLD.separation_of_duties
        OR NEW.approval_ttl_seconds IS DISTINCT FROM OLD.approval_ttl_seconds
        OR NEW.acknowledgement_required IS DISTINCT FROM OLD.acknowledgement_required
        OR NEW.subscription_required IS DISTINCT FROM OLD.subscription_required
        OR NEW.financial_threshold_minor IS DISTINCT FROM OLD.financial_threshold_minor
        OR NEW.financial_threshold_currency IS DISTINCT FROM OLD.financial_threshold_currency
        OR NEW.effective_from IS DISTINCT FROM OLD.effective_from
        OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'eligibility policy versions are immutable; publish a new version';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_fleet_partner_eligibility_policy_immutable
    ON fleet_partner_eligibility_policies;
CREATE TRIGGER trg_fleet_partner_eligibility_policy_immutable
    BEFORE UPDATE OR DELETE ON fleet_partner_eligibility_policies
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_eligibility_policy_immutable();

-- ---------------------------------------------------------------------------
-- Decisions
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS fleet_partner_eligibility_decisions (
    id                   text PRIMARY KEY,
    organization_id      text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,
    -- Branch scope of the decision, so a branch-scoped operator cannot read or
    -- reuse a decision taken for a branch they have no rights over.
    branch_id            text,
    subject_type         text NOT NULL CHECK (subject_type IN ('COMPANY', 'PROVIDER', 'VEHICLE')),
    -- Opaque subject identifier. Deliberately not a foreign key: a vehicle is
    -- owned by another service and a cross-service foreign key is exactly the
    -- coupling this architecture forbids.
    subject_id           text NOT NULL,
    operation_type       text NOT NULL,

    policy_id            text NOT NULL REFERENCES fleet_partner_eligibility_policies (id) ON DELETE RESTRICT,
    policy_key           text NOT NULL,
    policy_version       bigint NOT NULL CHECK (policy_version >= 1),

    status               text NOT NULL
                         CHECK (status IN ('ELIGIBLE', 'INELIGIBLE', 'UNKNOWN', 'STALE', 'RECONCILING')),

    -- Hash of the normalized decision inputs. A later authorization must
    -- present the same fingerprint, so a decision taken for one shape of
    -- request cannot be reused for another.
    input_fingerprint    text NOT NULL,

    checked_at           timestamptz NOT NULL DEFAULT now(),
    expires_at           timestamptz NOT NULL,
    -- Set when a newer decision replaces this one. The old row stays exactly
    -- as it was: superseding is not editing.
    superseded_at        timestamptz,
    superseded_by        text,

    -- PROVIDER is in the set because a provider accepting or departing is a
    -- real actor whose decision must be attributable to them, not laundered
    -- into "the relay service did it".
    actor_type           text NOT NULL
                         CHECK (actor_type IN ('ORGANIZATION_USER', 'PROVIDER',
                                               'PLATFORM_SERVICE', 'SYSTEM')),
    actor_id             text NOT NULL DEFAULT '',

    correlation_id       text NOT NULL DEFAULT '',
    causation_id         text NOT NULL DEFAULT '',
    -- References to the accountability records written in the same
    -- transaction. Stored as opaque ids, not foreign keys, so archiving audit
    -- or outbox rows can never block a decision read.
    audit_record_id      text NOT NULL DEFAULT '',
    outbox_event_id      text NOT NULL DEFAULT '',

    created_at           timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_fleet_partner_eligibility_expiry CHECK (expires_at > checked_at)
);

-- One live decision per subject and operation. A second concurrent evaluation
-- must supersede the first rather than sit beside it, otherwise a caller can
-- pick whichever answer it prefers.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_eligibility_current
    ON fleet_partner_eligibility_decisions
       (organization_id, subject_type, subject_id, operation_type)
    WHERE superseded_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_eligibility_tenant
    ON fleet_partner_eligibility_decisions (organization_id, checked_at DESC);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_eligibility_subject
    ON fleet_partner_eligibility_decisions (organization_id, subject_type, subject_id, checked_at DESC);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_eligibility_operation
    ON fleet_partner_eligibility_decisions (organization_id, operation_type, status, checked_at DESC);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_eligibility_status
    ON fleet_partner_eligibility_decisions (organization_id, status)
    WHERE superseded_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_eligibility_expiry
    ON fleet_partner_eligibility_decisions (expires_at)
    WHERE superseded_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_eligibility_branch
    ON fleet_partner_eligibility_decisions (organization_id, branch_id)
    WHERE branch_id IS NOT NULL;

-- A decision is immutable except for being superseded.
CREATE OR REPLACE FUNCTION fleet_partner_eligibility_decision_immutable() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'eligibility decisions are append-only';
    END IF;
    IF NEW.id IS DISTINCT FROM OLD.id
        OR NEW.organization_id IS DISTINCT FROM OLD.organization_id
        OR NEW.branch_id IS DISTINCT FROM OLD.branch_id
        OR NEW.subject_type IS DISTINCT FROM OLD.subject_type
        OR NEW.subject_id IS DISTINCT FROM OLD.subject_id
        OR NEW.operation_type IS DISTINCT FROM OLD.operation_type
        OR NEW.policy_id IS DISTINCT FROM OLD.policy_id
        OR NEW.policy_key IS DISTINCT FROM OLD.policy_key
        OR NEW.policy_version IS DISTINCT FROM OLD.policy_version
        OR NEW.status IS DISTINCT FROM OLD.status
        OR NEW.input_fingerprint IS DISTINCT FROM OLD.input_fingerprint
        OR NEW.checked_at IS DISTINCT FROM OLD.checked_at
        OR NEW.expires_at IS DISTINCT FROM OLD.expires_at
        OR NEW.actor_type IS DISTINCT FROM OLD.actor_type
        OR NEW.actor_id IS DISTINCT FROM OLD.actor_id
        OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'eligibility decisions are immutable; record a new decision';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_fleet_partner_eligibility_decision_immutable
    ON fleet_partner_eligibility_decisions;
CREATE TRIGGER trg_fleet_partner_eligibility_decision_immutable
    BEFORE UPDATE OR DELETE ON fleet_partner_eligibility_decisions
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_eligibility_decision_immutable();

-- Reasons carried by a decision. Foreign-keyed to the controlled vocabulary,
-- so an unrecognised reason cannot be written at all.
CREATE TABLE IF NOT EXISTS fleet_partner_eligibility_decision_reasons (
    decision_id  text NOT NULL REFERENCES fleet_partner_eligibility_decisions (id) ON DELETE RESTRICT,
    reason_code  text NOT NULL REFERENCES fleet_partner_eligibility_reason_codes (code) ON DELETE RESTRICT,
    -- Which authority produced this reason, when one did.
    authority    text NOT NULL DEFAULT '',
    ordinal      integer NOT NULL DEFAULT 0,
    PRIMARY KEY (decision_id, reason_code)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_eligibility_reason_code
    ON fleet_partner_eligibility_decision_reasons (reason_code);

DROP TRIGGER IF EXISTS trg_fleet_partner_eligibility_reasons_append_only
    ON fleet_partner_eligibility_decision_reasons;
CREATE TRIGGER trg_fleet_partner_eligibility_reasons_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_eligibility_decision_reasons
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- Authoritative sources consulted, with the version each answered at. This is
-- what makes a decision reproducible: without the source versions, "the
-- provider was verified" is an unfalsifiable claim.
CREATE TABLE IF NOT EXISTS fleet_partner_eligibility_decision_sources (
    id                text PRIMARY KEY,
    decision_id       text NOT NULL REFERENCES fleet_partner_eligibility_decisions (id) ON DELETE RESTRICT,
    organization_id   text NOT NULL,
    authority_service text NOT NULL,
    source_kind       text NOT NULL,
    -- Opaque identifier as the authority knows it.
    source_reference  text NOT NULL,
    source_version    bigint NOT NULL DEFAULT 0 CHECK (source_version >= 0),
    -- CURRENT, STALE, GAP_DETECTED, UNKNOWN and RECONCILING mirror the
    -- freshness vocabulary already used by the finance projections, so one
    -- reader does not have to learn two spellings of the same idea.
    freshness_state   text NOT NULL
                      CHECK (freshness_state IN ('CURRENT', 'STALE', 'GAP_DETECTED', 'UNKNOWN', 'RECONCILING')),
    observed_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_fleet_partner_eligibility_source
        UNIQUE (decision_id, authority_service, source_kind, source_reference)
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_eligibility_source_decision
    ON fleet_partner_eligibility_decision_sources (decision_id);

DROP TRIGGER IF EXISTS trg_fleet_partner_eligibility_sources_append_only
    ON fleet_partner_eligibility_decision_sources;
CREATE TRIGGER trg_fleet_partner_eligibility_sources_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_eligibility_decision_sources
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- Append-only history of what happened to a subject's eligibility.
CREATE TABLE IF NOT EXISTS fleet_partner_eligibility_decision_history (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    decision_id     text NOT NULL,
    subject_type    text NOT NULL,
    subject_id      text NOT NULL,
    operation_type  text NOT NULL,
    action          text NOT NULL
                    CHECK (action IN ('EVALUATED', 'SUPERSEDED', 'EXPIRED', 'INVALIDATED', 'CONSUMED')),
    from_status     text NOT NULL DEFAULT '',
    to_status       text NOT NULL DEFAULT '',
    actor_type      text NOT NULL,
    actor_id        text NOT NULL DEFAULT '',
    reason_code     text NOT NULL DEFAULT '',
    correlation_id  text NOT NULL DEFAULT '',
    causation_id    text NOT NULL DEFAULT '',
    occurred_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_eligibility_history
    ON fleet_partner_eligibility_decision_history (organization_id, decision_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_eligibility_history_subject
    ON fleet_partner_eligibility_decision_history (organization_id, subject_type, subject_id, occurred_at DESC);

DROP TRIGGER IF EXISTS trg_fleet_partner_eligibility_history_append_only
    ON fleet_partner_eligibility_decision_history;
CREATE TRIGGER trg_fleet_partner_eligibility_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_eligibility_decision_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- ---------------------------------------------------------------------------
-- Platform default policy versions
-- ---------------------------------------------------------------------------

-- Version 1 of each default. Tenants inherit these until they publish their
-- own version; a tenant override is a separate row, so a platform change never
-- silently rewrites a tenant's rules either.
INSERT INTO fleet_partner_eligibility_policies
    (id, organization_id, policy_key, operation_type, version,
     required_company_checks, required_provider_checks, required_vehicle_checks,
     required_documents, decision_ttl_seconds, source_max_age_seconds,
     approval_required, minimum_approvals, eligible_role_ids,
     self_approval_allowed, separation_of_duties, approval_ttl_seconds,
     acknowledgement_required, subscription_required,
     financial_threshold_minor, financial_threshold_currency)
VALUES
    ('elp_default_company_activation', NULL, 'COMPANY_ACTIVATION', 'COMPANY_ACTIVATION', 1,
     '["lifecycle","verification","approval","restrictions","documents","branch","projection_freshness"]'::jsonb,
     '[]'::jsonb, '[]'::jsonb,
     '["REGISTRATION_CERTIFICATE","TAX_IDENTIFICATION"]'::jsonb,
     900, 900, true, 1, '["rol_system_owner","rol_system_admin"]'::jsonb, false, true, 86400, false, false, NULL, NULL),

    ('elp_default_provider_linking', NULL, 'PROVIDER_LINKING', 'PROVIDER_LINKING', 1,
     '["lifecycle","verification","restrictions","subscription"]'::jsonb,
     '["existence","membership","verification","service_type","consent","restrictions","projection_freshness"]'::jsonb,
     '[]'::jsonb, '[]'::jsonb,
     900, 900, true, 1, '["rol_system_owner","rol_system_admin","rol_system_operations"]'::jsonb, false, true, 86400, false, true, NULL, NULL),

    ('elp_default_vehicle_linking', NULL, 'VEHICLE_LINKING', 'VEHICLE_LINKING', 1,
     '["lifecycle","restrictions"]'::jsonb,
     '["existence","membership"]'::jsonb,
     '["existence","ownership","provider_association","service_type","verification","restrictions","documents","source_version"]'::jsonb,
     '[]'::jsonb,
     900, 900, true, 1, '["rol_system_owner","rol_system_admin","rol_system_fleet"]'::jsonb, false, true, 86400, false, false, NULL, NULL),

    ('elp_default_assignment', NULL, 'ASSIGNMENT', 'ASSIGNMENT', 1,
     '["lifecycle","restrictions","subscription","branch"]'::jsonb,
     '["existence","membership","verification","service_type","branch","consent","availability","documents","restrictions","assignment_conflict","projection_freshness"]'::jsonb,
     '["existence","ownership","provider_association","verification","availability","restrictions","assignment_conflict"]'::jsonb,
     '[]'::jsonb,
     300, 300, false, 1, '[]'::jsonb, false, true, 86400, false, true, NULL, NULL),

    ('elp_default_execution', NULL, 'EXECUTION', 'EXECUTION', 1,
     '["lifecycle","restrictions"]'::jsonb,
     '["existence","membership","verification","restrictions","availability","documents"]'::jsonb,
     '["existence","ownership","verification","restrictions"]'::jsonb,
     '[]'::jsonb,
     300, 300, false, 1, '[]'::jsonb, false, true, 86400, true, false, NULL, NULL),

    ('elp_default_paid_feature', NULL, 'PAID_FEATURE', 'PAID_FEATURE', 1,
     '["lifecycle","restrictions","subscription"]'::jsonb,
     '[]'::jsonb, '[]'::jsonb, '[]'::jsonb,
     600, 600, false, 1, '[]'::jsonb, false, true, 86400, false, true, NULL, NULL),

    ('elp_default_financial_release', NULL, 'FINANCIAL_RELEASE', 'FINANCIAL_RELEASE', 1,
     '["lifecycle","restrictions","subscription","projection_freshness"]'::jsonb,
     '[]'::jsonb, '[]'::jsonb, '[]'::jsonb,
     300, 300, true, 2, '["rol_system_owner","rol_system_finance"]'::jsonb, false, true, 86400, false, true, 0, 'NGN'),

    ('elp_default_financial_reversal', NULL, 'FINANCIAL_REVERSAL', 'FINANCIAL_REVERSAL', 1,
     '["lifecycle","restrictions","projection_freshness"]'::jsonb,
     '[]'::jsonb, '[]'::jsonb, '[]'::jsonb,
     300, 300, true, 2, '["rol_system_owner","rol_system_finance"]'::jsonb, false, true, 86400, false, false, 0, 'NGN'),

    ('elp_default_document_action', NULL, 'DOCUMENT_ACTION', 'DOCUMENT_ACTION', 1,
     '["lifecycle","restrictions","documents"]'::jsonb,
     '[]'::jsonb, '[]'::jsonb, '[]'::jsonb,
     900, 900, false, 1, '[]'::jsonb, false, true, 86400, false, false, NULL, NULL)
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Permissions
-- ---------------------------------------------------------------------------

-- Reading an eligibility answer, forcing a re-evaluation, and publishing the
-- policy that decides the answer are three separate capabilities. Anyone who
-- can rewrite the policy can make anything eligible.
INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('eligibility.decisions.read',   'eligibility', 'Read recorded eligibility decisions', true),
    ('eligibility.decisions.refresh','eligibility', 'Force re-evaluation of an eligibility decision', true),
    ('eligibility.policies.read',    'eligibility', 'Read eligibility and approval policy versions', true),
    ('eligibility.policies.manage',  'eligibility', 'Publish a new eligibility or approval policy version', true),
    ('approvals.requests.read',      'approvals',   'Read approval requests and their decisions', true),
    ('approvals.requests.create',    'approvals',   'Request approval for a controlled operation', true),
    ('approvals.requests.decide',    'approvals',   'Approve or reject an approval request', true),
    ('approvals.requests.cancel',    'approvals',   'Cancel an approval request that has not executed', true),
    ('approvals.requests.execute',   'approvals',   'Execute an approved controlled operation', true)
ON CONFLICT (code) DO UPDATE
    SET category = EXCLUDED.category, description = EXCLUDED.description, is_active = true;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_owner', code
FROM fleet_partner_permissions
WHERE code IN ('eligibility.decisions.read', 'eligibility.decisions.refresh',
               'eligibility.policies.read', 'eligibility.policies.manage',
               'approvals.requests.read', 'approvals.requests.create',
               'approvals.requests.decide', 'approvals.requests.cancel',
               'approvals.requests.execute')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code) VALUES
    ('rol_system_admin',      'eligibility.decisions.read'),
    ('rol_system_admin',      'eligibility.decisions.refresh'),
    ('rol_system_admin',      'eligibility.policies.read'),
    ('rol_system_admin',      'eligibility.policies.manage'),
    ('rol_system_admin',      'approvals.requests.read'),
    ('rol_system_admin',      'approvals.requests.create'),
    ('rol_system_admin',      'approvals.requests.decide'),
    ('rol_system_admin',      'approvals.requests.cancel'),
    ('rol_system_admin',      'approvals.requests.execute'),
    ('rol_system_operations', 'eligibility.decisions.read'),
    ('rol_system_operations', 'eligibility.decisions.refresh'),
    ('rol_system_operations', 'eligibility.policies.read'),
    ('rol_system_operations', 'approvals.requests.read'),
    ('rol_system_operations', 'approvals.requests.create'),
    ('rol_system_operations', 'approvals.requests.cancel'),
    ('rol_system_operations', 'approvals.requests.execute'),
    ('rol_system_dispatcher', 'eligibility.decisions.read'),
    ('rol_system_dispatcher', 'eligibility.decisions.refresh'),
    ('rol_system_dispatcher', 'approvals.requests.read'),
    ('rol_system_dispatcher', 'approvals.requests.create'),
    ('rol_system_fleet',      'eligibility.decisions.read'),
    ('rol_system_fleet',      'eligibility.decisions.refresh'),
    ('rol_system_fleet',      'approvals.requests.read'),
    ('rol_system_fleet',      'approvals.requests.create'),
    -- Compliance reviews and decides; it does not request its own approvals.
    ('rol_system_compliance', 'eligibility.decisions.read'),
    ('rol_system_compliance', 'eligibility.policies.read'),
    ('rol_system_compliance', 'approvals.requests.read'),
    ('rol_system_compliance', 'approvals.requests.decide'),
    ('rol_system_finance',    'eligibility.decisions.read'),
    ('rol_system_finance',    'approvals.requests.read'),
    ('rol_system_finance',    'approvals.requests.decide'),
    ('rol_system_analyst',    'eligibility.decisions.read')
ON CONFLICT DO NOTHING;
