-- Section 13C-1
-- Taxonomy mapping and governance persistence foundation.
--
-- Catalogue/source records remain owned by their source domain.
-- Taxonomy stores stable source references rather than duplicating
-- catalogue entities or introducing cross-domain hard foreign keys.

CREATE TABLE canonical_taxonomy_mappings (
    id TEXT PRIMARY KEY,

    namespace_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_namespaces(id)
        ON DELETE RESTRICT,

    version_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_versions(id)
        ON DELETE RESTRICT,

    node_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_nodes(id)
        ON DELETE RESTRICT,

    source_type TEXT NOT NULL,
    source_ref TEXT NOT NULL,

    mapping_type TEXT NOT NULL
        CHECK (
            mapping_type IN (
                'PRIMARY',
                'SECONDARY',
                'DISCOVERY'
            )
        ),

    status TEXT NOT NULL DEFAULT 'ACTIVE'
        CHECK (
            status IN (
                'ACTIVE',
                'ARCHIVED'
            )
        ),

    confidence_basis TEXT NOT NULL DEFAULT 'MANUAL'
        CHECK (
            confidence_basis IN (
                'MANUAL',
                'RULE',
                'IMPORT',
                'SYSTEM'
            )
        ),

    confidence_score INTEGER
        CHECK (
            confidence_score IS NULL
            OR confidence_score BETWEEN 0 AND 10000
        ),

    branch_ref TEXT,
    business_ref TEXT,

    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),

    revision BIGINT NOT NULL DEFAULT 1
        CHECK (revision > 0),

    created_by_actor_ref TEXT NOT NULL,
    updated_by_actor_ref TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT canonical_taxonomy_mapping_source_type_ck
        CHECK (btrim(source_type) <> ''),

    CONSTRAINT canonical_taxonomy_mapping_source_ref_ck
        CHECK (btrim(source_ref) <> ''),

    CONSTRAINT canonical_taxonomy_mapping_actor_ck
        CHECK (
            btrim(created_by_actor_ref) <> ''
            AND btrim(updated_by_actor_ref) <> ''
        ),

    CONSTRAINT canonical_taxonomy_mapping_identity_uq
        UNIQUE (
            namespace_id,
            version_id,
            source_type,
            source_ref,
            node_id,
            mapping_type
        )
);

CREATE INDEX canonical_taxonomy_mappings_source_idx
    ON canonical_taxonomy_mappings (
        namespace_id,
        source_type,
        source_ref,
        status
    );

CREATE INDEX canonical_taxonomy_mappings_node_idx
    ON canonical_taxonomy_mappings (
        node_id,
        status
    );

CREATE INDEX canonical_taxonomy_mappings_business_idx
    ON canonical_taxonomy_mappings (
        business_ref,
        branch_ref,
        status
    );


CREATE TABLE canonical_taxonomy_mapping_history (
    id BIGSERIAL PRIMARY KEY,

    mapping_id TEXT NOT NULL,
    namespace_id TEXT NOT NULL,
    version_id TEXT NOT NULL,
    node_id TEXT NOT NULL,

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


CREATE TABLE canonical_taxonomy_mapping_commands (
    id TEXT PRIMARY KEY,

    command_ref TEXT NOT NULL UNIQUE,

    actor_ref TEXT NOT NULL,
    operation TEXT NOT NULL,

    mapping_id TEXT,

    request_hash TEXT NOT NULL,
    result_snapshot JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT canonical_taxonomy_mapping_command_ref_ck
        CHECK (btrim(command_ref) <> ''),

    CONSTRAINT canonical_taxonomy_mapping_command_actor_ck
        CHECK (btrim(actor_ref) <> ''),

    CONSTRAINT canonical_taxonomy_mapping_request_hash_ck
        CHECK (btrim(request_hash) <> '')
);


CREATE TABLE canonical_taxonomy_governance_cases (
    id TEXT PRIMARY KEY,

    namespace_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_namespaces(id)
        ON DELETE RESTRICT,

    version_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_versions(id)
        ON DELETE RESTRICT,

    subject_type TEXT NOT NULL,
    subject_ref TEXT NOT NULL,

    case_type TEXT NOT NULL
        CHECK (
            case_type IN (
                'MAPPING_REVIEW',
                'CLASSIFICATION_DISPUTE',
                'DUPLICATE_CONCEPT',
                'MISCLASSIFICATION',
                'VOCABULARY_REVIEW',
                'POLICY_REVIEW'
            )
        ),

    status TEXT NOT NULL DEFAULT 'OPEN'
        CHECK (
            status IN (
                'OPEN',
                'IN_REVIEW',
                'RESOLVED',
                'REJECTED',
                'CANCELLED'
            )
        ),

    priority TEXT NOT NULL DEFAULT 'NORMAL'
        CHECK (
            priority IN (
                'LOW',
                'NORMAL',
                'HIGH',
                'CRITICAL'
            )
        ),

    reason_code TEXT,
    description TEXT,

    resolution_code TEXT,
    resolution_notes TEXT,

    assigned_actor_ref TEXT,

    opened_by_actor_ref TEXT NOT NULL,
    resolved_by_actor_ref TEXT,

    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),

    revision BIGINT NOT NULL DEFAULT 1
        CHECK (revision > 0),

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,

    CONSTRAINT canonical_taxonomy_governance_subject_type_ck
        CHECK (btrim(subject_type) <> ''),

    CONSTRAINT canonical_taxonomy_governance_subject_ref_ck
        CHECK (btrim(subject_ref) <> ''),

    CONSTRAINT canonical_taxonomy_governance_opened_actor_ck
        CHECK (btrim(opened_by_actor_ref) <> '')
);

CREATE INDEX canonical_taxonomy_governance_queue_idx
    ON canonical_taxonomy_governance_cases (
        status,
        priority,
        created_at
    );

CREATE INDEX canonical_taxonomy_governance_subject_idx
    ON canonical_taxonomy_governance_cases (
        subject_type,
        subject_ref,
        status
    );


CREATE TABLE canonical_taxonomy_governance_history (
    id BIGSERIAL PRIMARY KEY,

    case_id TEXT NOT NULL,

    operation TEXT NOT NULL
        CHECK (
            operation IN (
                'OPENED',
                'UPDATED',
                'ASSIGNED',
                'RESOLVED',
                'REJECTED',
                'CANCELLED'
            )
        ),

    actor_ref TEXT NOT NULL,

    before_state JSONB,
    after_state JSONB,

    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);


CREATE OR REPLACE FUNCTION canonical_taxonomy_validate_mapping()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    version_namespace TEXT;
    version_status TEXT;

    node_namespace TEXT;
    node_version TEXT;
    node_status TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;

    NEW.source_type := upper(btrim(NEW.source_type));
    NEW.source_ref := btrim(NEW.source_ref);

    IF NEW.branch_ref IS NOT NULL THEN
        NEW.branch_ref := NULLIF(btrim(NEW.branch_ref), '');
    END IF;

    IF NEW.business_ref IS NOT NULL THEN
        NEW.business_ref := NULLIF(btrim(NEW.business_ref), '');
    END IF;

    SELECT namespace_id, status
      INTO version_namespace, version_status
      FROM canonical_taxonomy_versions
     WHERE id=NEW.version_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION
            'taxonomy mapping version not found';
    END IF;

    IF version_namespace <> NEW.namespace_id THEN
        RAISE EXCEPTION
            'taxonomy mapping namespace/version ownership invalid';
    END IF;

    IF version_status <> 'PUBLISHED' THEN
        RAISE EXCEPTION
            'taxonomy mappings may target only published taxonomy versions';
    END IF;

    SELECT namespace_id, version_id, status
      INTO node_namespace, node_version, node_status
      FROM canonical_taxonomy_nodes
     WHERE id=NEW.node_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION
            'taxonomy mapping node not found';
    END IF;

    IF node_namespace <> NEW.namespace_id
       OR node_version <> NEW.version_id THEN
        RAISE EXCEPTION
            'taxonomy mapping node ownership invalid';
    END IF;

    IF node_status <> 'ACTIVE' THEN
        RAISE EXCEPTION
            'taxonomy mapping requires ACTIVE node';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF NEW.id <> OLD.id
           OR NEW.namespace_id <> OLD.namespace_id
           OR NEW.version_id <> OLD.version_id
           OR NEW.node_id <> OLD.node_id
           OR NEW.source_type <> OLD.source_type
           OR NEW.source_ref <> OLD.source_ref
           OR NEW.mapping_type <> OLD.mapping_type THEN
            RAISE EXCEPTION
                'taxonomy mapping identity is immutable; archive and create another mapping';
        END IF;

        IF NEW.revision <> OLD.revision THEN
            RAISE EXCEPTION
                'taxonomy mapping revision is server-managed';
        END IF;

        NEW.revision := OLD.revision + 1;
    END IF;

    NEW.updated_at := now();

    RETURN NEW;
END;
$$;

CREATE TRIGGER canonical_taxonomy_mapping_validate
BEFORE INSERT OR UPDATE
ON canonical_taxonomy_mappings
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_validate_mapping();


CREATE OR REPLACE FUNCTION canonical_taxonomy_record_mapping_history()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO canonical_taxonomy_mapping_history (
            mapping_id,
            namespace_id,
            version_id,
            node_id,
            operation,
            actor_ref,
            before_state,
            after_state
        )
        VALUES (
            NEW.id,
            NEW.namespace_id,
            NEW.version_id,
            NEW.node_id,
            'INSERT',
            NEW.created_by_actor_ref,
            NULL,
            to_jsonb(NEW)
        );

        RETURN NEW;
    ELSIF TG_OP = 'UPDATE' THEN
        INSERT INTO canonical_taxonomy_mapping_history (
            mapping_id,
            namespace_id,
            version_id,
            node_id,
            operation,
            actor_ref,
            before_state,
            after_state
        )
        VALUES (
            NEW.id,
            NEW.namespace_id,
            NEW.version_id,
            NEW.node_id,
            'UPDATE',
            NEW.updated_by_actor_ref,
            to_jsonb(OLD),
            to_jsonb(NEW)
        );

        RETURN NEW;
    ELSE
        INSERT INTO canonical_taxonomy_mapping_history (
            mapping_id,
            namespace_id,
            version_id,
            node_id,
            operation,
            actor_ref,
            before_state,
            after_state
        )
        VALUES (
            OLD.id,
            OLD.namespace_id,
            OLD.version_id,
            OLD.node_id,
            'DELETE',
            OLD.updated_by_actor_ref,
            to_jsonb(OLD),
            NULL
        );

        RETURN OLD;
    END IF;
END;
$$;

CREATE TRIGGER canonical_taxonomy_mapping_history_trigger
AFTER INSERT OR UPDATE OR DELETE
ON canonical_taxonomy_mappings
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_record_mapping_history();


CREATE OR REPLACE FUNCTION canonical_taxonomy_mapping_history_append_only()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION
        'canonical taxonomy mapping history is append-only';
END;
$$;

CREATE TRIGGER canonical_taxonomy_mapping_history_guard
BEFORE UPDATE OR DELETE
ON canonical_taxonomy_mapping_history
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_mapping_history_append_only();


CREATE OR REPLACE FUNCTION canonical_taxonomy_governance_case_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        RETURN NEW;
    END IF;

    IF NEW.id <> OLD.id
       OR NEW.namespace_id <> OLD.namespace_id
       OR NEW.version_id <> OLD.version_id
       OR NEW.subject_type <> OLD.subject_type
       OR NEW.subject_ref <> OLD.subject_ref
       OR NEW.case_type <> OLD.case_type
       OR NEW.opened_by_actor_ref <> OLD.opened_by_actor_ref
       OR NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION
            'taxonomy governance case identity is immutable';
    END IF;

    IF NEW.revision <> OLD.revision THEN
        RAISE EXCEPTION
            'taxonomy governance revision is server-managed';
    END IF;

    IF OLD.status IN ('RESOLVED','REJECTED','CANCELLED')
       AND NEW.status <> OLD.status THEN
        RAISE EXCEPTION
            'closed taxonomy governance case cannot transition';
    END IF;

    IF NEW.status IN ('RESOLVED','REJECTED') THEN
        IF NEW.resolved_by_actor_ref IS NULL
           OR btrim(NEW.resolved_by_actor_ref) = '' THEN
            RAISE EXCEPTION
                'closed governance case requires resolver';
        END IF;

        IF NEW.resolved_at IS NULL THEN
            NEW.resolved_at := now();
        END IF;
    END IF;

    NEW.revision := OLD.revision + 1;
    NEW.updated_at := now();

    RETURN NEW;
END;
$$;

CREATE TRIGGER canonical_taxonomy_governance_case_guard_trigger
BEFORE UPDATE
ON canonical_taxonomy_governance_cases
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_governance_case_guard();


CREATE OR REPLACE FUNCTION canonical_taxonomy_governance_history_append_only()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION
        'canonical taxonomy governance history is append-only';
END;
$$;

CREATE TRIGGER canonical_taxonomy_governance_history_guard
BEFORE UPDATE OR DELETE
ON canonical_taxonomy_governance_history
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_governance_history_append_only();