-- Section 13C-2
-- Governed semantic tags and dietary-claim vocabulary.
--
-- Source catalogue records remain owned by their source domain.
-- Assignments store stable source_type/source_ref references only.

CREATE TABLE canonical_taxonomy_tags (
    id TEXT PRIMARY KEY,

    namespace_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_namespaces(id)
        ON DELETE RESTRICT,

    code TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT,

    tag_type TEXT NOT NULL
        CHECK (
            tag_type IN (
                'ATTRIBUTE',
                'DISCOVERY',
                'PREPARATION',
                'MERCHANDISING'
            )
        ),

    status TEXT NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','ARCHIVED')),

    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),

    revision BIGINT NOT NULL DEFAULT 1
        CHECK (revision > 0),

    created_by_actor_ref TEXT NOT NULL,
    updated_by_actor_ref TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT canonical_taxonomy_tag_code_ck
        CHECK (code ~ '^[A-Z][A-Z0-9_]{0,95}$'),

    CONSTRAINT canonical_taxonomy_tag_display_ck
        CHECK (char_length(btrim(display_name)) BETWEEN 1 AND 160),

    CONSTRAINT canonical_taxonomy_tag_actor_ck
        CHECK (
            btrim(created_by_actor_ref) <> ''
            AND btrim(updated_by_actor_ref) <> ''
        ),

    CONSTRAINT canonical_taxonomy_tag_code_uq
        UNIQUE (namespace_id, code)
);

CREATE INDEX canonical_taxonomy_tags_lookup_idx
    ON canonical_taxonomy_tags (
        namespace_id,
        tag_type,
        status,
        code
    );


CREATE TABLE canonical_taxonomy_tag_assignments (
    id TEXT PRIMARY KEY,

    namespace_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_namespaces(id)
        ON DELETE RESTRICT,

    tag_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_tags(id)
        ON DELETE RESTRICT,

    source_type TEXT NOT NULL,
    source_ref TEXT NOT NULL,

    business_ref TEXT,
    branch_ref TEXT,

    status TEXT NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','ARCHIVED')),

    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),

    revision BIGINT NOT NULL DEFAULT 1
        CHECK (revision > 0),

    created_by_actor_ref TEXT NOT NULL,
    updated_by_actor_ref TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT canonical_taxonomy_tag_assignment_source_type_ck
        CHECK (btrim(source_type) <> ''),

    CONSTRAINT canonical_taxonomy_tag_assignment_source_ref_ck
        CHECK (btrim(source_ref) <> ''),

    CONSTRAINT canonical_taxonomy_tag_assignment_identity_uq
        UNIQUE (
            namespace_id,
            tag_id,
            source_type,
            source_ref
        )
);

CREATE INDEX canonical_taxonomy_tag_assignment_source_idx
    ON canonical_taxonomy_tag_assignments (
        namespace_id,
        source_type,
        source_ref,
        status
    );


CREATE TABLE canonical_taxonomy_dietary_claims (
    id TEXT PRIMARY KEY,

    namespace_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_namespaces(id)
        ON DELETE RESTRICT,

    code TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT,

    verification_policy TEXT NOT NULL
        CHECK (
            verification_policy IN (
                'DECLARATION',
                'EVIDENCE_REQUIRED',
                'PLATFORM_VERIFIED'
            )
        ),

    risk_level TEXT NOT NULL
        CHECK (
            risk_level IN (
                'LOW',
                'MODERATE',
                'HIGH'
            )
        ),

    status TEXT NOT NULL DEFAULT 'ACTIVE'
        CHECK (status IN ('ACTIVE','ARCHIVED')),

    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),

    revision BIGINT NOT NULL DEFAULT 1
        CHECK (revision > 0),

    created_by_actor_ref TEXT NOT NULL,
    updated_by_actor_ref TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT canonical_taxonomy_dietary_claim_code_ck
        CHECK (code ~ '^[A-Z][A-Z0-9_]{0,95}$'),

    CONSTRAINT canonical_taxonomy_dietary_claim_display_ck
        CHECK (char_length(btrim(display_name)) BETWEEN 1 AND 160),

    CONSTRAINT canonical_taxonomy_dietary_claim_code_uq
        UNIQUE (namespace_id, code)
);

CREATE INDEX canonical_taxonomy_dietary_claim_lookup_idx
    ON canonical_taxonomy_dietary_claims (
        namespace_id,
        status,
        code
    );


CREATE TABLE canonical_taxonomy_dietary_claim_assignments (
    id TEXT PRIMARY KEY,

    namespace_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_namespaces(id)
        ON DELETE RESTRICT,

    claim_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_dietary_claims(id)
        ON DELETE RESTRICT,

    source_type TEXT NOT NULL,
    source_ref TEXT NOT NULL,

    business_ref TEXT,
    branch_ref TEXT,

    status TEXT NOT NULL DEFAULT 'DECLARED'
        CHECK (
            status IN (
                'DECLARED',
                'PENDING_VERIFICATION',
                'VERIFIED',
                'REJECTED',
                'EXPIRED'
            )
        ),

    evidence_ref TEXT,

    verified_by_actor_ref TEXT,
    verified_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,

    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),

    revision BIGINT NOT NULL DEFAULT 1
        CHECK (revision > 0),

    created_by_actor_ref TEXT NOT NULL,
    updated_by_actor_ref TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT canonical_taxonomy_dietary_assignment_source_type_ck
        CHECK (btrim(source_type) <> ''),

    CONSTRAINT canonical_taxonomy_dietary_assignment_source_ref_ck
        CHECK (btrim(source_ref) <> ''),

    CONSTRAINT canonical_taxonomy_dietary_assignment_identity_uq
        UNIQUE (
            namespace_id,
            claim_id,
            source_type,
            source_ref
        )
);

CREATE INDEX canonical_taxonomy_dietary_assignment_source_idx
    ON canonical_taxonomy_dietary_claim_assignments (
        namespace_id,
        source_type,
        source_ref,
        status
    );


CREATE TABLE canonical_taxonomy_semantic_history (
    id BIGSERIAL PRIMARY KEY,

    entity_type TEXT NOT NULL
        CHECK (
            entity_type IN (
                'TAG',
                'TAG_ASSIGNMENT',
                'DIETARY_CLAIM',
                'DIETARY_CLAIM_ASSIGNMENT'
            )
        ),

    entity_id TEXT NOT NULL,
    namespace_id TEXT NOT NULL,

    operation TEXT NOT NULL
        CHECK (
            operation IN (
                'INSERT',
                'UPDATE',
                'DELETE'
            )
        ),

    actor_ref TEXT NOT NULL,

    before_state JSONB,
    after_state JSONB,

    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);


CREATE OR REPLACE FUNCTION canonical_taxonomy_validate_tag_assignment()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    tag_namespace TEXT;
    tag_status TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;

    NEW.source_type := upper(btrim(NEW.source_type));
    NEW.source_ref := btrim(NEW.source_ref);

    SELECT namespace_id, status
      INTO tag_namespace, tag_status
      FROM canonical_taxonomy_tags
     WHERE id=NEW.tag_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'taxonomy tag not found';
    END IF;

    IF tag_namespace <> NEW.namespace_id THEN
        RAISE EXCEPTION 'taxonomy tag namespace mismatch';
    END IF;

    IF tag_status <> 'ACTIVE' THEN
        RAISE EXCEPTION 'archived taxonomy tag cannot be assigned';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF NEW.id <> OLD.id
           OR NEW.namespace_id <> OLD.namespace_id
           OR NEW.tag_id <> OLD.tag_id
           OR NEW.source_type <> OLD.source_type
           OR NEW.source_ref <> OLD.source_ref THEN
            RAISE EXCEPTION
                'taxonomy tag assignment identity is immutable';
        END IF;

        IF NEW.revision <> OLD.revision THEN
            RAISE EXCEPTION
                'taxonomy tag assignment revision is server-managed';
        END IF;

        NEW.revision := OLD.revision + 1;
    END IF;

    NEW.updated_at := now();

    RETURN NEW;
END;
$$;

CREATE TRIGGER canonical_taxonomy_tag_assignment_guard
BEFORE INSERT OR UPDATE
ON canonical_taxonomy_tag_assignments
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_validate_tag_assignment();


CREATE OR REPLACE FUNCTION canonical_taxonomy_validate_dietary_assignment()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    claim_namespace TEXT;
    claim_status TEXT;
    claim_policy TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;

    NEW.source_type := upper(btrim(NEW.source_type));
    NEW.source_ref := btrim(NEW.source_ref);

    SELECT
        namespace_id,
        status,
        verification_policy
      INTO
        claim_namespace,
        claim_status,
        claim_policy
      FROM canonical_taxonomy_dietary_claims
     WHERE id=NEW.claim_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'dietary claim not found';
    END IF;

    IF claim_namespace <> NEW.namespace_id THEN
        RAISE EXCEPTION 'dietary claim namespace mismatch';
    END IF;

    IF claim_status <> 'ACTIVE' THEN
        RAISE EXCEPTION 'archived dietary claim cannot be assigned';
    END IF;

    IF NEW.status = 'VERIFIED' THEN
        IF NEW.verified_by_actor_ref IS NULL
           OR btrim(NEW.verified_by_actor_ref) = '' THEN
            RAISE EXCEPTION
                'verified dietary claim requires verifier';
        END IF;

        IF claim_policy IN (
            'EVIDENCE_REQUIRED',
            'PLATFORM_VERIFIED'
        )
        AND (
            NEW.evidence_ref IS NULL
            OR btrim(NEW.evidence_ref) = ''
        ) THEN
            RAISE EXCEPTION
                'verified dietary claim requires evidence';
        END IF;

        IF NEW.verified_at IS NULL THEN
            NEW.verified_at := now();
        END IF;
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF NEW.id <> OLD.id
           OR NEW.namespace_id <> OLD.namespace_id
           OR NEW.claim_id <> OLD.claim_id
           OR NEW.source_type <> OLD.source_type
           OR NEW.source_ref <> OLD.source_ref THEN
            RAISE EXCEPTION
                'dietary claim assignment identity is immutable';
        END IF;

        IF NEW.revision <> OLD.revision THEN
            RAISE EXCEPTION
                'dietary claim assignment revision is server-managed';
        END IF;

        NEW.revision := OLD.revision + 1;
    END IF;

    NEW.updated_at := now();

    RETURN NEW;
END;
$$;

CREATE TRIGGER canonical_taxonomy_dietary_assignment_guard
BEFORE INSERT OR UPDATE
ON canonical_taxonomy_dietary_claim_assignments
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_validate_dietary_assignment();


CREATE OR REPLACE FUNCTION canonical_taxonomy_semantic_history_record()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    entity_kind TEXT;
    entity_ref TEXT;
    namespace_ref TEXT;
    actor_ref TEXT;
BEGIN
    entity_kind :=
        CASE TG_TABLE_NAME
            WHEN 'canonical_taxonomy_tags'
                THEN 'TAG'
            WHEN 'canonical_taxonomy_tag_assignments'
                THEN 'TAG_ASSIGNMENT'
            WHEN 'canonical_taxonomy_dietary_claims'
                THEN 'DIETARY_CLAIM'
            WHEN 'canonical_taxonomy_dietary_claim_assignments'
                THEN 'DIETARY_CLAIM_ASSIGNMENT'
        END;

    IF TG_OP = 'DELETE' THEN
        entity_ref := OLD.id;
        namespace_ref := OLD.namespace_id;
        actor_ref := OLD.updated_by_actor_ref;

        INSERT INTO canonical_taxonomy_semantic_history (
            entity_type,
            entity_id,
            namespace_id,
            operation,
            actor_ref,
            before_state,
            after_state
        )
        VALUES (
            entity_kind,
            entity_ref,
            namespace_ref,
            'DELETE',
            actor_ref,
            to_jsonb(OLD),
            NULL
        );

        RETURN OLD;
    END IF;

    entity_ref := NEW.id;
    namespace_ref := NEW.namespace_id;

    IF TG_OP = 'INSERT' THEN
        actor_ref := NEW.created_by_actor_ref;

        INSERT INTO canonical_taxonomy_semantic_history (
            entity_type,
            entity_id,
            namespace_id,
            operation,
            actor_ref,
            before_state,
            after_state
        )
        VALUES (
            entity_kind,
            entity_ref,
            namespace_ref,
            'INSERT',
            actor_ref,
            NULL,
            to_jsonb(NEW)
        );
    ELSE
        actor_ref := NEW.updated_by_actor_ref;

        INSERT INTO canonical_taxonomy_semantic_history (
            entity_type,
            entity_id,
            namespace_id,
            operation,
            actor_ref,
            before_state,
            after_state
        )
        VALUES (
            entity_kind,
            entity_ref,
            namespace_ref,
            'UPDATE',
            actor_ref,
            to_jsonb(OLD),
            to_jsonb(NEW)
        );
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER canonical_taxonomy_tag_history
AFTER INSERT OR UPDATE OR DELETE
ON canonical_taxonomy_tags
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_semantic_history_record();

CREATE TRIGGER canonical_taxonomy_tag_assignment_history
AFTER INSERT OR UPDATE OR DELETE
ON canonical_taxonomy_tag_assignments
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_semantic_history_record();

CREATE TRIGGER canonical_taxonomy_dietary_claim_history
AFTER INSERT OR UPDATE OR DELETE
ON canonical_taxonomy_dietary_claims
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_semantic_history_record();

CREATE TRIGGER canonical_taxonomy_dietary_assignment_history
AFTER INSERT OR UPDATE OR DELETE
ON canonical_taxonomy_dietary_claim_assignments
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_semantic_history_record();


CREATE OR REPLACE FUNCTION canonical_taxonomy_semantic_history_append_only()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION
        'canonical taxonomy semantic history is append-only';
END;
$$;

CREATE TRIGGER canonical_taxonomy_semantic_history_guard
BEFORE UPDATE OR DELETE
ON canonical_taxonomy_semantic_history
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_semantic_history_append_only();