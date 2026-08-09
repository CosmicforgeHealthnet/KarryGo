-- WP10: fleet-side document REFERENCES to authoritative media objects.
--
-- Fleet owns the business meaning of a document — which company or vehicle it
-- belongs to, what category it is, whether a human verified it, when it
-- expires, whether it was replaced or revoked, and whether it may satisfy an
-- eligibility requirement. Fleet owns none of the file: not the bytes, not the
-- storage location, not the detected type, not the safety verdict, and not
-- whether the object still physically exists. Those belong to
-- media-file-service and arrive here only as a projection with a version.
--
-- Boundaries enforced by the schema rather than by convention:
--
--   1. media_object_id is opaque text with NO foreign key. A cross-service
--      foreign key would make fleet's writes depend on another service's
--      table, which is the coupling this architecture exists to avoid.
--   2. There is no column anywhere for a URL, a signed link, a storage path or
--      a token. A private document that had a durable URL would not be
--      private, and a stored signed URL is a credential nobody rotates.
--   3. safety_projection has no default of SAFE and projection_freshness has
--      no default of CURRENT for a document that has heard nothing. A document
--      fleet has not heard about is UNKNOWN, and UNKNOWN blocks.
--   4. History and verification decisions are append-only. A verification that
--      could be edited afterwards is not evidence of anything.

CREATE TABLE IF NOT EXISTS fleet_partner_documents (
    id                    text PRIMARY KEY,
    organization_id       text NOT NULL REFERENCES fleet_partner_organizations (id) ON DELETE RESTRICT,

    -- What the document is about. owner_id is opaque within its owner_type;
    -- the use case proves the organization owns it before any write.
    owner_type            text NOT NULL
                          CHECK (owner_type IN ('COMPANY', 'BRANCH', 'PROVIDER_LINK', 'VEHICLE_LINK',
                                                'INCIDENT', 'DISPUTE', 'APPROVAL')),
    owner_id              text NOT NULL,
    branch_id             text REFERENCES fleet_partner_branches (id) ON DELETE RESTRICT,

    -- Opaque media-file-service identifier. Deliberately not a foreign key and
    -- deliberately not parsed.
    media_object_id       text NOT NULL,

    category              text NOT NULL
                          CHECK (category IN ('INSURANCE', 'REGISTRATION', 'PERMIT', 'COMPLIANCE',
                                              'PROOF', 'INCIDENT_ATTACHMENT', 'DISPUTE_ATTACHMENT',
                                              'APPROVAL_ATTACHMENT', 'SUPPORTING')),
    document_type         text NOT NULL,
    issue_date            date,
    expiry_date           date,

    -- The full controlled lifecycle. STALE, GAP_DETECTED, RECONCILING and
    -- UNKNOWN are real states meaning "nobody can currently vouch for this",
    -- and every gate treats them exactly as it treats UNSAFE: it blocks.
    lifecycle_state       text NOT NULL DEFAULT 'UPLOAD_PENDING'
                          CHECK (lifecycle_state IN (
                              'UPLOAD_PENDING', 'UPLOADED', 'VALIDATION_PENDING', 'SAFE', 'UNSAFE',
                              'QUARANTINED', 'VERIFICATION_PENDING', 'VERIFIED', 'REJECTED',
                              'EXPIRED', 'REVOKED', 'REPLACED', 'DELETED', 'UNKNOWN', 'STALE',
                              'GAP_DETECTED', 'RECONCILING')),

    verification_state    text NOT NULL DEFAULT 'PENDING'
                          CHECK (verification_state IN ('PENDING', 'REQUESTED', 'VERIFIED', 'REJECTED', 'INVALIDATED')),

    -- Projection of media's authority. Never fleet's own opinion.
    safety_projection     text NOT NULL DEFAULT 'UNKNOWN'
                          CHECK (safety_projection IN ('PENDING', 'SAFE', 'UNSAFE', 'QUARANTINED', 'UNKNOWN')),
    media_upload_status   text NOT NULL DEFAULT 'PENDING'
                          CHECK (media_upload_status IN ('PENDING', 'AUTHORIZED', 'UPLOADING',
                                                         'COMPLETED', 'FAILED', 'EXPIRED', 'UNKNOWN')),
    media_deletion_state  text NOT NULL DEFAULT 'ACTIVE'
                          CHECK (media_deletion_state IN ('ACTIVE', 'DELETION_PENDING', 'DELETED',
                                                          'DELETION_UNKNOWN', 'DELETION_FAILED')),
    media_source_version  bigint NOT NULL DEFAULT 0 CHECK (media_source_version >= 0),
    projection_freshness  text NOT NULL DEFAULT 'UNKNOWN'
                          CHECK (projection_freshness IN ('CURRENT', 'STALE', 'GAP_DETECTED',
                                                          'UNKNOWN', 'RECONCILING')),
    last_media_event_id   text,
    synced_at             timestamptz,

    -- Replacement chain. The old document is preserved and marked REPLACED;
    -- the new one must pass validation and verification on its own.
    replacement_document_id text REFERENCES fleet_partner_documents (id) ON DELETE RESTRICT,
    replaces_document_id    text REFERENCES fleet_partner_documents (id) ON DELETE RESTRICT,

    revoked_at            timestamptz,
    revoked_reason        text NOT NULL DEFAULT '',
    rejected_reason       text NOT NULL DEFAULT '',
    deleted_at            timestamptz,

    -- Retention lives on both sides: fleet decides when a document may be
    -- destroyed, media decides whether it actually can be.
    retention_class       text NOT NULL DEFAULT 'STANDARD'
                          CHECK (retention_class IN ('STANDARD', 'EXTENDED', 'LEGAL')),
    retention_until       timestamptz,
    legal_hold            boolean NOT NULL DEFAULT false,
    legal_hold_reason     text NOT NULL DEFAULT '',

    created_by_actor_type text NOT NULL
                          CHECK (created_by_actor_type IN ('ORGANIZATION_USER', 'PROVIDER', 'PLATFORM_SERVICE', 'SYSTEM')),
    created_by_actor_id   text NOT NULL,
    correlation_id        text NOT NULL DEFAULT '',
    causation_id          text NOT NULL DEFAULT '',
    idempotency_key       text NOT NULL,
    request_fingerprint   text NOT NULL,
    version               bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),

    -- A revoked document must say why. "Revoked" with no reason is an
    -- unanswerable question three months later.
    CONSTRAINT chk_fleet_partner_document_revocation CHECK (
        (revoked_at IS NULL) = (revoked_reason = '')
    ),
    -- A document may never be VERIFIED unless media confirmed the content is
    -- SAFE. This is the single rule the whole work package exists to protect,
    -- so it is a constraint and not only a code path.
    CONSTRAINT chk_fleet_partner_document_verified_requires_safe CHECK (
        verification_state <> 'VERIFIED' OR safety_projection = 'SAFE'
    ),
    -- A branch document must name its branch.
    CONSTRAINT chk_fleet_partner_document_branch CHECK (
        owner_type <> 'BRANCH' OR branch_id IS NOT NULL
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_document_idempotency
    ON fleet_partner_documents (organization_id, idempotency_key);

-- One fleet document per media object. Two documents sharing an object would
-- let a revoked document's file stay reachable through its twin.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_document_media_object
    ON fleet_partner_documents (media_object_id);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_tenant
    ON fleet_partner_documents (organization_id, owner_type, owner_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_branch
    ON fleet_partner_documents (organization_id, branch_id)
    WHERE branch_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_category
    ON fleet_partner_documents (organization_id, category, document_type, lifecycle_state);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_state
    ON fleet_partner_documents (organization_id, lifecycle_state, verification_state);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_expiry
    ON fleet_partner_documents (expiry_date)
    WHERE expiry_date IS NOT NULL AND lifecycle_state NOT IN ('EXPIRED', 'REVOKED', 'REPLACED', 'DELETED');

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_retention
    ON fleet_partner_documents (retention_until)
    WHERE retention_until IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_source_version
    ON fleet_partner_documents (media_object_id, media_source_version);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_freshness
    ON fleet_partner_documents (projection_freshness)
    WHERE projection_freshness <> 'CURRENT';

-- Append-only history of everything that happened to a document.
CREATE TABLE IF NOT EXISTS fleet_partner_document_history (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    document_id     text NOT NULL,
    action          text NOT NULL,
    from_state      text NOT NULL DEFAULT '',
    to_state        text NOT NULL DEFAULT '',
    actor_type      text NOT NULL,
    actor_id        text NOT NULL,
    metadata        jsonb NOT NULL DEFAULT '{}'::jsonb,
    correlation_id  text NOT NULL DEFAULT '',
    causation_id    text NOT NULL DEFAULT '',
    occurred_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_history
    ON fleet_partner_document_history (organization_id, document_id, occurred_at DESC);

DROP TRIGGER IF EXISTS trg_fleet_partner_document_history_append_only
    ON fleet_partner_document_history;
CREATE TRIGGER trg_fleet_partner_document_history_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_document_history
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- Immutable verification decisions. Separate from history because a decision
-- carries an accountable identity and a controlled reason, and must be
-- readable as a decision record rather than dug out of an event log.
CREATE TABLE IF NOT EXISTS fleet_partner_document_verifications (
    id                    text PRIMARY KEY,
    organization_id       text NOT NULL,
    document_id           text NOT NULL,
    decision              text NOT NULL CHECK (decision IN ('REQUESTED', 'VERIFIED', 'REJECTED', 'INVALIDATED')),
    reason_code           text NOT NULL DEFAULT '',
    reason_note           text NOT NULL DEFAULT '',
    -- The media version the decision was taken against. A later version means
    -- the decision was about a different file.
    media_source_version  bigint NOT NULL DEFAULT 0,
    decided_by_actor_type text NOT NULL,
    decided_by_actor_id   text NOT NULL,
    document_version      bigint NOT NULL DEFAULT 1,
    correlation_id        text NOT NULL DEFAULT '',
    causation_id          text NOT NULL DEFAULT '',
    decided_at            timestamptz NOT NULL DEFAULT now(),

    -- A rejection with no reason cannot be acted on by whoever has to fix it.
    CONSTRAINT chk_fleet_partner_document_rejection_reason CHECK (
        decision <> 'REJECTED' OR reason_code <> ''
    )
);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_verifications
    ON fleet_partner_document_verifications (organization_id, document_id, decided_at DESC);

DROP TRIGGER IF EXISTS trg_fleet_partner_document_verifications_append_only
    ON fleet_partner_document_verifications;
CREATE TRIGGER trg_fleet_partner_document_verifications_append_only
    BEFORE UPDATE OR DELETE ON fleet_partner_document_verifications
    FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- Forward-only source version tracking for media events.
--
-- Holding the payload hash beside the version is what makes the difference
-- between "the same event arrived twice" (ignore) and "a different payload
-- claims the same version" (a contract error that must never be applied).
CREATE TABLE IF NOT EXISTS fleet_partner_document_media_versions (
    media_object_id text PRIMARY KEY,
    source_version  bigint NOT NULL CHECK (source_version >= 0),
    last_event_id   text NOT NULL DEFAULT '',
    payload_hash    text NOT NULL DEFAULT '',
    updated_at      timestamptz NOT NULL DEFAULT now()
);

-- Durable reconciliation cases with leases, retries and dead-lettering.
CREATE TABLE IF NOT EXISTS fleet_partner_document_reconciliation_cases (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL,
    document_id         text NOT NULL REFERENCES fleet_partner_documents (id) ON DELETE CASCADE,
    media_object_id     text NOT NULL,
    reason              text NOT NULL
                        CHECK (reason IN ('VERSION_GAP', 'STALE_PROJECTION', 'UNKNOWN_OUTCOME',
                                          'DELETION_UNKNOWN', 'UPLOAD_UNKNOWN', 'SAFETY_UNKNOWN',
                                          'PAYLOAD_CONFLICT')),
    status              text NOT NULL DEFAULT 'OPEN'
                        CHECK (status IN ('OPEN', 'IN_PROGRESS', 'RESOLVED', 'DEAD_LETTERED')),
    attempts            integer NOT NULL DEFAULT 0,
    last_error_code     text NOT NULL DEFAULT '',
    next_attempt_at     timestamptz NOT NULL DEFAULT now(),
    lease_owner         text,
    lease_expires_at    timestamptz,
    resolved_at         timestamptz,
    correlation_id      text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

-- At most one open case per document. A second case for the same problem would
-- double the retry rate against an authority that is already struggling.
CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_document_reconciliation_open
    ON fleet_partner_document_reconciliation_cases (document_id)
    WHERE status IN ('OPEN', 'IN_PROGRESS');

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_reconciliation_due
    ON fleet_partner_document_reconciliation_cases (status, next_attempt_at)
    WHERE status IN ('OPEN', 'IN_PROGRESS');

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_reconciliation_lease
    ON fleet_partner_document_reconciliation_cases (lease_expires_at)
    WHERE lease_owner IS NOT NULL;

-- Durable deletion scheduling. Fleet decides when; media decides whether.
CREATE TABLE IF NOT EXISTS fleet_partner_document_deletion_requests (
    id                  text PRIMARY KEY,
    organization_id     text NOT NULL,
    document_id         text NOT NULL REFERENCES fleet_partner_documents (id) ON DELETE CASCADE,
    media_object_id     text NOT NULL,
    reason              text NOT NULL DEFAULT '',
    status              text NOT NULL DEFAULT 'SCHEDULED'
                        CHECK (status IN ('SCHEDULED', 'IN_PROGRESS', 'COMPLETED', 'BLOCKED',
                                          'UNKNOWN', 'FAILED', 'DEAD_LETTERED')),
    block_reason        text NOT NULL DEFAULT '',
    attempts            integer NOT NULL DEFAULT 0,
    last_error_code     text NOT NULL DEFAULT '',
    scheduled_for       timestamptz NOT NULL DEFAULT now(),
    next_attempt_at     timestamptz NOT NULL DEFAULT now(),
    lease_owner         text,
    lease_expires_at    timestamptz,
    requested_by        text NOT NULL,
    idempotency_key     text NOT NULL,
    request_fingerprint text NOT NULL,
    correlation_id      text NOT NULL DEFAULT '',
    causation_id        text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    completed_at        timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_fleet_partner_document_deletion_idempotency
    ON fleet_partner_document_deletion_requests (organization_id, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_deletion_due
    ON fleet_partner_document_deletion_requests (status, next_attempt_at)
    WHERE status IN ('SCHEDULED', 'IN_PROGRESS', 'UNKNOWN');

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_deletion_lease
    ON fleet_partner_document_deletion_requests (lease_expires_at)
    WHERE lease_owner IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_fleet_partner_document_deletion_document
    ON fleet_partner_document_deletion_requests (document_id, created_at DESC);

-- Eligibility reason codes for the document checks. They are rows rather than
-- free text so a typo fails the write instead of producing a reason nobody can
-- filter for later.
INSERT INTO fleet_partner_eligibility_reason_codes (code, subject_type, category, description) VALUES
    ('document_missing',            'COMPANY', 'documents', 'A required document has not been provided'),
    ('document_not_verified',       'COMPANY', 'documents', 'A required document has not been verified'),
    ('document_not_safe',           'COMPANY', 'documents', 'A required document has not been confirmed safe by the media authority'),
    ('document_expired',            'COMPANY', 'documents', 'A required document has passed its expiry date'),
    ('document_revoked',            'COMPANY', 'documents', 'A required document was revoked'),
    ('document_replaced',           'COMPANY', 'documents', 'A required document was replaced and the latest version is required'),
    ('document_deleted',            'COMPANY', 'documents', 'A required document has been deleted'),
    ('document_projection_stale',   'COMPANY', 'documents', 'The media projection for a required document is not current'),
    ('document_reconciling',        'COMPANY', 'documents', 'A required document has an unresolved reconciliation case')
ON CONFLICT (code) DO NOTHING;

-- Permissions. Reading a document, uploading one, downloading its content and
-- deciding whether it is genuine are four different capabilities. Someone who
-- may attach an insurance certificate is not automatically someone who may
-- declare it verified, and neither is automatically someone who may read the
-- file itself.
INSERT INTO fleet_partner_permissions (code, category, description, is_active) VALUES
    ('documents.read',     'documents', 'Read document records held by the organization', true),
    ('documents.manage',   'documents', 'Upload, replace and revoke organization documents', true),
    ('documents.download', 'documents', 'Obtain short-lived access to document content', true),
    ('documents.verify',   'documents', 'Approve or reject document verification', true),
    ('documents.delete',   'documents', 'Schedule deletion and manage retention and legal hold', true)
ON CONFLICT (code) DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code)
SELECT 'rol_system_owner', code
FROM fleet_partner_permissions
WHERE code IN ('documents.read', 'documents.manage', 'documents.download',
               'documents.verify', 'documents.delete')
ON CONFLICT DO NOTHING;

INSERT INTO fleet_partner_role_permissions (role_id, permission_code) VALUES
    ('rol_system_admin',      'documents.read'),
    ('rol_system_admin',      'documents.manage'),
    ('rol_system_admin',      'documents.download'),
    ('rol_system_admin',      'documents.verify'),
    ('rol_system_admin',      'documents.delete'),
    -- Compliance is the verifying role: it reads and decides, and may reach
    -- the file to do so, but does not upload the evidence it judges.
    ('rol_system_compliance', 'documents.read'),
    ('rol_system_compliance', 'documents.download'),
    ('rol_system_compliance', 'documents.verify'),
    -- Operations and fleet supply documents; they do not sign them off.
    ('rol_system_operations', 'documents.read'),
    ('rol_system_operations', 'documents.manage'),
    ('rol_system_fleet',      'documents.read'),
    ('rol_system_fleet',      'documents.manage'),
    ('rol_system_dispatcher', 'documents.read')
ON CONFLICT DO NOTHING;
