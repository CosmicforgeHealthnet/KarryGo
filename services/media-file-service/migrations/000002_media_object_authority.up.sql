-- WP10.2: the authoritative media object.
--
-- media_assets (migration 001) records a file that was already uploaded
-- through the public multipart route. It cannot express the states this work
-- package needs, because in that model the upload, the validation and the
-- record all happen inside one request: there is no moment at which an upload
-- is authorized but not yet complete, and no way to say "we have bytes but
-- nobody has vouched for them".
--
-- media_objects is that model. It is additive — nothing here modifies or drops
-- media_assets, and the existing public upload path keeps working unchanged.
--
-- Three boundaries are enforced by the schema rather than by convention:
--
--   1. safety_status has no default of SAFE anywhere. A deployment with no
--      scanner records PENDING or UNKNOWN, and every consumer that requires
--      SAFE therefore blocks. Defaulting to SAFE would make "we never looked"
--      indistinguishable from "we looked and it was clean".
--   2. detected_mime is separate from declared_mime and both are stored. The
--      client's claim is evidence about the client, not about the file.
--   3. storage_object_key is never chosen by the caller. The upload
--      authorization mints it, and the unique constraint means two objects can
--      never share one key.

CREATE TABLE IF NOT EXISTS media_objects (
    id                       text PRIMARY KEY,
    -- Opaque tenant identifier owned by the calling service. Deliberately not
    -- a foreign key: media holds no organization table and must never depend
    -- on another service's rows to accept a write.
    organization_id          text NOT NULL,
    owner_service            text NOT NULL,
    -- What the owning service calls the thing this file belongs to
    -- (a fleet document id, a provider id). Opaque here.
    owner_type               text NOT NULL DEFAULT '',
    owner_reference          text NOT NULL DEFAULT '',
    upload_purpose           text NOT NULL,

    declared_mime            text NOT NULL DEFAULT '',
    detected_mime            text NOT NULL DEFAULT '',
    size_bytes               bigint NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
    checksum_sha256          text NOT NULL DEFAULT '',
    -- Result of the magic-byte / file-signature check.
    signature_validation     text NOT NULL DEFAULT 'PENDING'
                             CHECK (signature_validation IN ('PENDING', 'PASSED', 'FAILED', 'UNSUPPORTED')),

    upload_status            text NOT NULL DEFAULT 'PENDING'
                             CHECK (upload_status IN ('PENDING', 'AUTHORIZED', 'UPLOADING',
                                                      'COMPLETED', 'FAILED', 'EXPIRED')),
    safety_status            text NOT NULL DEFAULT 'PENDING'
                             CHECK (safety_status IN ('PENDING', 'SAFE', 'UNSAFE',
                                                      'QUARANTINED', 'UNKNOWN')),
    -- Why the file was rejected or quarantined. A controlled code, never the
    -- scanner's raw output.
    safety_reason            text NOT NULL DEFAULT '',
    scanned_at               timestamptz,
    validation_failure_code  text NOT NULL DEFAULT '',

    storage_bucket           text NOT NULL DEFAULT '',
    -- Server-chosen object key. A caller-chosen path would let one tenant
    -- write over another tenant's object.
    storage_object_key       text NOT NULL,

    deletion_state           text NOT NULL DEFAULT 'ACTIVE'
                             CHECK (deletion_state IN ('ACTIVE', 'DELETION_PENDING', 'DELETED',
                                                       'DELETION_UNKNOWN', 'DELETION_FAILED')),
    deletion_requested_at    timestamptz,
    deleted_at               timestamptz,
    deletion_failure_code    text NOT NULL DEFAULT '',

    -- Media owns physical retention: it is the only service that can refuse to
    -- destroy bytes it holds.
    retention_class          text NOT NULL DEFAULT 'STANDARD',
    retention_until          timestamptz,
    legal_hold               boolean NOT NULL DEFAULT false,

    version                  bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    correlation_id           text NOT NULL DEFAULT '',
    causation_id             text NOT NULL DEFAULT '',
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_media_object_storage_key UNIQUE (storage_object_key),
    -- A completed upload must have the evidence that made it completable.
    CONSTRAINT chk_media_object_completed_evidence CHECK (
        upload_status <> 'COMPLETED'
        OR (checksum_sha256 <> '' AND detected_mime <> '' AND size_bytes > 0)
    ),
    -- Physical deletion is only ever recorded alongside its timestamp.
    CONSTRAINT chk_media_object_deleted_at CHECK (
        (deletion_state = 'DELETED') = (deleted_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_media_objects_tenant
    ON media_objects (organization_id, owner_service, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_media_objects_owner
    ON media_objects (organization_id, owner_type, owner_reference)
    WHERE owner_reference <> '';

CREATE INDEX IF NOT EXISTS idx_media_objects_upload_status
    ON media_objects (upload_status, created_at);

CREATE INDEX IF NOT EXISTS idx_media_objects_safety_status
    ON media_objects (safety_status)
    WHERE safety_status IN ('PENDING', 'UNKNOWN');

CREATE INDEX IF NOT EXISTS idx_media_objects_deletion_state
    ON media_objects (deletion_state)
    WHERE deletion_state <> 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_media_objects_retention
    ON media_objects (retention_until)
    WHERE retention_until IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_media_objects_version
    ON media_objects (id, version);

-- One-shot upload authorization.
--
-- The grant is what carries the constraints the upload must satisfy, so those
-- constraints cannot be renegotiated at completion time by a client that
-- simply sends different numbers.
CREATE TABLE IF NOT EXISTS media_upload_authorizations (
    id                    text PRIMARY KEY,
    media_object_id       text NOT NULL REFERENCES media_objects (id) ON DELETE CASCADE,
    organization_id       text NOT NULL,
    caller_service        text NOT NULL,
    owner_service         text NOT NULL,
    owner_type            text NOT NULL DEFAULT '',
    owner_reference       text NOT NULL DEFAULT '',
    document_category     text NOT NULL DEFAULT '',
    upload_purpose        text NOT NULL,
    allowed_mime_types    text[] NOT NULL DEFAULT ARRAY[]::text[],
    max_size_bytes        bigint NOT NULL CHECK (max_size_bytes > 0),
    -- Only the hash of the upload token is stored. Holding the token itself
    -- would turn a database read into the ability to upload as somebody else.
    upload_token_hash     text NOT NULL,
    state                 text NOT NULL DEFAULT 'ISSUED'
                          CHECK (state IN ('ISSUED', 'CONSUMED', 'EXPIRED', 'FAILED')),
    expires_at            timestamptz NOT NULL,
    consumed_at           timestamptz,
    idempotency_key       text NOT NULL,
    request_fingerprint   text NOT NULL,
    correlation_id        text NOT NULL DEFAULT '',
    causation_id          text NOT NULL DEFAULT '',
    payload_version       integer NOT NULL DEFAULT 1,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

-- Single-use is enforced here, not in application code: exactly one object per
-- authorization, and one authorization per caller idempotency key.
CREATE UNIQUE INDEX IF NOT EXISTS uq_media_upload_authorization_object
    ON media_upload_authorizations (media_object_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_media_upload_authorization_idempotency
    ON media_upload_authorizations (caller_service, organization_id, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_media_upload_authorization_expiry
    ON media_upload_authorizations (state, expires_at);

-- Append-only history of everything that happened to a media object.
CREATE TABLE IF NOT EXISTS media_object_history (
    id              text PRIMARY KEY,
    organization_id text NOT NULL,
    media_object_id text NOT NULL,
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

CREATE INDEX IF NOT EXISTS idx_media_object_history
    ON media_object_history (media_object_id, occurred_at DESC);

CREATE OR REPLACE FUNCTION media_forbid_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'media: % on % is forbidden; this table is append-only',
        TG_OP, TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_media_object_history_append_only ON media_object_history;
CREATE TRIGGER trg_media_object_history_append_only
    BEFORE UPDATE OR DELETE ON media_object_history
    FOR EACH ROW EXECUTE FUNCTION media_forbid_mutation();

-- Append-only audit trail. Deliberately separate from history: history is the
-- object's own story, audit is the record of who acted.
CREATE TABLE IF NOT EXISTS media_audit_records (
    id               text PRIMARY KEY,
    organization_id  text,
    actor_type       text NOT NULL,
    actor_id         text NOT NULL,
    action           text NOT NULL,
    resource_type    text NOT NULL,
    resource_id      text NOT NULL,
    previous_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
    new_summary      jsonb NOT NULL DEFAULT '{}'::jsonb,
    reason           text,
    request_id       text NOT NULL DEFAULT '',
    correlation_id   text NOT NULL DEFAULT '',
    occurred_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_media_audit_resource
    ON media_audit_records (resource_type, resource_id, occurred_at DESC);

DROP TRIGGER IF EXISTS trg_media_audit_append_only ON media_audit_records;
CREATE TRIGGER trg_media_audit_append_only
    BEFORE UPDATE OR DELETE ON media_audit_records
    FOR EACH ROW EXECUTE FUNCTION media_forbid_mutation();

-- Transactional outbox.
CREATE TABLE IF NOT EXISTS media_outbox (
    id                  text PRIMARY KEY,
    event_type          text NOT NULL,
    schema_version      integer NOT NULL CHECK (schema_version >= 1),
    aggregate_type      text NOT NULL,
    aggregate_id        text NOT NULL,
    aggregate_version   bigint NOT NULL CHECK (aggregate_version >= 1),
    organization_id     text NOT NULL,
    payload             jsonb NOT NULL,
    headers             jsonb NOT NULL DEFAULT '{}'::jsonb,
    status              text NOT NULL DEFAULT 'PENDING'
                        CHECK (status IN ('PENDING', 'PUBLISHED', 'DEAD_LETTERED')),
    available_at        timestamptz NOT NULL DEFAULT now(),
    attempts            integer NOT NULL DEFAULT 0,
    lease_owner         text,
    lease_expires_at    timestamptz,
    last_error_redacted text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    published_at        timestamptz
);

CREATE INDEX IF NOT EXISTS idx_media_outbox_claimable
    ON media_outbox (status, available_at)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_media_outbox_aggregate
    ON media_outbox (aggregate_type, aggregate_id, aggregate_version);

-- Private-call replay protection. The shared HMAC timestamp bounds how long a
-- captured request stays valid; this makes each one usable exactly once inside
-- that window.
CREATE TABLE IF NOT EXISTS media_service_request_nonces (
    service_name        text NOT NULL,
    nonce               text NOT NULL,
    request_fingerprint text NOT NULL,
    expires_at          timestamptz NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (service_name, nonce)
);

CREATE INDEX IF NOT EXISTS idx_media_service_nonce_expiry
    ON media_service_request_nonces (expires_at);

-- Single-runner coordination for the sweep workers.
CREATE TABLE IF NOT EXISTS media_worker_leases (
    worker_name      text PRIMARY KEY,
    lease_owner      text NOT NULL,
    lease_expires_at timestamptz NOT NULL,
    updated_at       timestamptz NOT NULL DEFAULT now()
);

-- Durable deletion requests. Fleet asks; media decides and records.
CREATE TABLE IF NOT EXISTS media_deletion_requests (
    id                  text PRIMARY KEY,
    media_object_id     text NOT NULL REFERENCES media_objects (id) ON DELETE CASCADE,
    organization_id     text NOT NULL,
    requested_by        text NOT NULL,
    reason              text NOT NULL DEFAULT '',
    status              text NOT NULL DEFAULT 'PENDING'
                        CHECK (status IN ('PENDING', 'COMPLETED', 'BLOCKED', 'FAILED', 'UNKNOWN')),
    -- Why a deletion was refused: LEGAL_HOLD or RETENTION_ACTIVE. Recorded so
    -- the requester learns the reason rather than retrying forever.
    block_reason        text NOT NULL DEFAULT '',
    attempts            integer NOT NULL DEFAULT 0,
    last_error_redacted text NOT NULL DEFAULT '',
    idempotency_key     text NOT NULL,
    request_fingerprint text NOT NULL,
    correlation_id      text NOT NULL DEFAULT '',
    causation_id        text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    completed_at        timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_media_deletion_request_idempotency
    ON media_deletion_requests (requested_by, organization_id, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_media_deletion_request_object
    ON media_deletion_requests (media_object_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_media_deletion_request_status
    ON media_deletion_requests (status)
    WHERE status IN ('PENDING', 'UNKNOWN');
