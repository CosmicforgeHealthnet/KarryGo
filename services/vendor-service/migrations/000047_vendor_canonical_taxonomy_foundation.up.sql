-- Section 13A — KarryGo Canonical Discovery Taxonomy
--
-- This schema is platform-owned discovery taxonomy.
-- It does NOT replace:
--   * vendor Food catalogue structures
--   * vendor Shop category structures
--
-- FOOD and SHOP are seeded only as namespace identities.
-- Canonical category/facet/node seed content belongs to Section 13B.
--
-- The existing vendor_outbox remains the transactional event outbox.
-- This migration deliberately does not create another outbox.

-- ============================================================================
-- 1. Canonical namespaces
-- ============================================================================
--
-- Namespace rows are also the publication/version-allocation serialization
-- point. Services lock the namespace row FOR UPDATE before allocating a
-- version number or publishing a draft.

CREATE TABLE canonical_taxonomy_namespaces (
    id              text PRIMARY KEY,
    code            text NOT NULL UNIQUE,
    display_name    text NOT NULL,
    status          text NOT NULL DEFAULT 'ACTIVE',
    revision        bigint NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT canonical_taxonomy_namespace_code_ck
        CHECK (code ~ '^[A-Z][A-Z0-9_]{0,63}$'),

    CONSTRAINT canonical_taxonomy_namespace_display_name_ck
        CHECK (char_length(display_name) BETWEEN 1 AND 120),

    CONSTRAINT canonical_taxonomy_namespace_status_ck
        CHECK (status IN ('ACTIVE','RETIRED')),

    CONSTRAINT canonical_taxonomy_namespace_revision_ck
        CHECK (revision > 0)
);

INSERT INTO canonical_taxonomy_namespaces
    (id, code, display_name)
VALUES
    ('ctns_food', 'FOOD', 'Food'),
    ('ctns_shop', 'SHOP', 'Shop')
ON CONFLICT (code) DO NOTHING;

-- ============================================================================
-- 2. Taxonomy versions
-- ============================================================================

CREATE TABLE canonical_taxonomy_versions (
    id                      text PRIMARY KEY,
    namespace_id            text NOT NULL
                                REFERENCES canonical_taxonomy_namespaces(id)
                                ON DELETE RESTRICT,
    version_number          integer NOT NULL,
    status                  text NOT NULL DEFAULT 'DRAFT',
    source_version_id       text,
    revision                bigint NOT NULL DEFAULT 1,

    created_by_actor_ref    text NOT NULL,
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),

    published_by_actor_ref  text,
    published_at            timestamptz,

    retired_by_actor_ref    text,
    retired_at              timestamptz,

    UNIQUE (id, namespace_id),
    UNIQUE (namespace_id, version_number),

    CONSTRAINT canonical_taxonomy_version_number_ck
        CHECK (version_number > 0),

    CONSTRAINT canonical_taxonomy_version_status_ck
        CHECK (status IN ('DRAFT','PUBLISHED','RETIRED')),

    CONSTRAINT canonical_taxonomy_version_revision_ck
        CHECK (revision > 0),

    CONSTRAINT canonical_taxonomy_version_creator_ck
        CHECK (char_length(created_by_actor_ref) BETWEEN 1 AND 200),

    CONSTRAINT canonical_taxonomy_version_publisher_ck
        CHECK (
            published_by_actor_ref IS NULL OR
            char_length(published_by_actor_ref) BETWEEN 1 AND 200
        ),

    CONSTRAINT canonical_taxonomy_version_retirer_ck
        CHECK (
            retired_by_actor_ref IS NULL OR
            char_length(retired_by_actor_ref) BETWEEN 1 AND 200
        ),

    CONSTRAINT canonical_taxonomy_version_lifecycle_ck
        CHECK (
            (
                status = 'DRAFT'
                AND published_at IS NULL
                AND published_by_actor_ref IS NULL
                AND retired_at IS NULL
                AND retired_by_actor_ref IS NULL
            )
            OR
            (
                status = 'PUBLISHED'
                AND published_at IS NOT NULL
                AND published_by_actor_ref IS NOT NULL
                AND retired_at IS NULL
                AND retired_by_actor_ref IS NULL
            )
            OR
            (
                status = 'RETIRED'
                AND published_at IS NOT NULL
                AND published_by_actor_ref IS NOT NULL
                AND retired_at IS NOT NULL
                AND retired_by_actor_ref IS NOT NULL
            )
        ),

    CONSTRAINT canonical_taxonomy_version_source_not_self_ck
        CHECK (source_version_id IS NULL OR source_version_id <> id),

    CONSTRAINT canonical_taxonomy_version_source_fk
        FOREIGN KEY (source_version_id, namespace_id)
        REFERENCES canonical_taxonomy_versions(id, namespace_id)
        ON DELETE RESTRICT
        DEFERRABLE INITIALLY IMMEDIATE
);

-- Database safety barrier: one active published version per namespace.
CREATE UNIQUE INDEX canonical_taxonomy_one_published_version_idx
    ON canonical_taxonomy_versions(namespace_id)
    WHERE status = 'PUBLISHED';

CREATE INDEX canonical_taxonomy_versions_namespace_history_idx
    ON canonical_taxonomy_versions(namespace_id, version_number DESC);

CREATE INDEX canonical_taxonomy_versions_status_idx
    ON canonical_taxonomy_versions(namespace_id, status);

-- ============================================================================
-- 3. Version lifecycle guard
-- ============================================================================
--
-- Drafts may be edited.
-- Published versions may only transition to RETIRED.
-- Retired versions are immutable.
-- Historical versions cannot be deleted.

CREATE FUNCTION canonical_taxonomy_version_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.status <> 'DRAFT' THEN
            RAISE EXCEPTION
                'canonical taxonomy versions must be created as DRAFT';
        END IF;

        RETURN NEW;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION
            'canonical taxonomy version history is immutable';
    END IF;

    IF NEW.namespace_id <> OLD.namespace_id
       OR NEW.version_number <> OLD.version_number
       OR NEW.source_version_id IS DISTINCT FROM OLD.source_version_id
       OR NEW.created_by_actor_ref <> OLD.created_by_actor_ref
       OR NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION
            'canonical taxonomy version identity is immutable';
    END IF;

    IF OLD.status = 'RETIRED' THEN
        RAISE EXCEPTION
            'retired canonical taxonomy versions are immutable';
    END IF;

    IF OLD.status = 'PUBLISHED' THEN
        IF NEW.status <> 'RETIRED' THEN
            RAISE EXCEPTION
                'published canonical taxonomy versions may only transition to RETIRED';
        END IF;

        IF NEW.published_at IS DISTINCT FROM OLD.published_at
           OR NEW.published_by_actor_ref IS DISTINCT FROM OLD.published_by_actor_ref THEN
            RAISE EXCEPTION
                'publication evidence is immutable';
        END IF;

        RETURN NEW;
    END IF;

    -- OLD is DRAFT here.
    IF NEW.status NOT IN ('DRAFT','PUBLISHED') THEN
        RAISE EXCEPTION
            'draft canonical taxonomy versions may only remain DRAFT or become PUBLISHED';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER canonical_taxonomy_version_guard_trg
BEFORE INSERT OR UPDATE OR DELETE
ON canonical_taxonomy_versions
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_version_guard();

-- ============================================================================
-- 4. Taxonomy facets
-- ============================================================================
--
-- Facet seed definitions such as CUISINE, DISH_TYPE, CATEGORY etc. belong to
-- Section 13B. This table only provides the data-driven model.

CREATE TABLE canonical_taxonomy_facets (
    id              text PRIMARY KEY,
    namespace_id    text NOT NULL,
    version_id      text NOT NULL,
    code            text NOT NULL,
    display_name    text NOT NULL,
    description     text,
    structure_type  text NOT NULL,
    position        integer NOT NULL DEFAULT 0,
    status          text NOT NULL DEFAULT 'ACTIVE',
    metadata        jsonb NOT NULL DEFAULT '{}'::jsonb,
    revision        bigint NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    archived_at     timestamptz,

    UNIQUE (id, namespace_id, version_id),
    UNIQUE (version_id, code),

    CONSTRAINT canonical_taxonomy_facet_version_fk
        FOREIGN KEY (version_id, namespace_id)
        REFERENCES canonical_taxonomy_versions(id, namespace_id)
        ON DELETE RESTRICT,

    CONSTRAINT canonical_taxonomy_facet_code_ck
        CHECK (code ~ '^[A-Z][A-Z0-9_]{0,63}$'),

    CONSTRAINT canonical_taxonomy_facet_display_name_ck
        CHECK (char_length(display_name) BETWEEN 1 AND 120),

    CONSTRAINT canonical_taxonomy_facet_description_ck
        CHECK (
            description IS NULL OR
            char_length(description) <= 1000
        ),

    CONSTRAINT canonical_taxonomy_facet_structure_ck
        CHECK (structure_type IN ('FLAT','HIERARCHICAL')),

    CONSTRAINT canonical_taxonomy_facet_position_ck
        CHECK (position BETWEEN 0 AND 100000),

    CONSTRAINT canonical_taxonomy_facet_status_ck
        CHECK (status IN ('ACTIVE','ARCHIVED')),

    CONSTRAINT canonical_taxonomy_facet_metadata_ck
        CHECK (jsonb_typeof(metadata) = 'object'),

    CONSTRAINT canonical_taxonomy_facet_revision_ck
        CHECK (revision > 0),

    CONSTRAINT canonical_taxonomy_facet_archive_ck
        CHECK (
            (status = 'ACTIVE' AND archived_at IS NULL)
            OR
            (status = 'ARCHIVED' AND archived_at IS NOT NULL)
        )
);

CREATE INDEX canonical_taxonomy_facets_version_order_idx
    ON canonical_taxonomy_facets(version_id, position, code);

-- ============================================================================
-- 5. Canonical taxonomy nodes
-- ============================================================================
--
-- Parent relationship is intrinsic to the node and is protected by a
-- composite FK so a parent can never cross namespace, version or facet.

CREATE TABLE canonical_taxonomy_nodes (
    id              text PRIMARY KEY,
    namespace_id    text NOT NULL,
    version_id      text NOT NULL,
    facet_id        text NOT NULL,

    canonical_code  text NOT NULL,
    display_name    text NOT NULL,
    description     text,
    parent_id       text,

    position        integer NOT NULL DEFAULT 0,
    status          text NOT NULL DEFAULT 'ACTIVE',
    metadata        jsonb NOT NULL DEFAULT '{}'::jsonb,

    revision        bigint NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    archived_at     timestamptz,

    UNIQUE (id, namespace_id, version_id, facet_id),
    UNIQUE (version_id, facet_id, canonical_code),

    CONSTRAINT canonical_taxonomy_node_version_fk
        FOREIGN KEY (version_id, namespace_id)
        REFERENCES canonical_taxonomy_versions(id, namespace_id)
        ON DELETE RESTRICT,

    CONSTRAINT canonical_taxonomy_node_facet_fk
        FOREIGN KEY (facet_id, namespace_id, version_id)
        REFERENCES canonical_taxonomy_facets(id, namespace_id, version_id)
        ON DELETE RESTRICT,

    CONSTRAINT canonical_taxonomy_node_parent_fk
        FOREIGN KEY (parent_id, namespace_id, version_id, facet_id)
        REFERENCES canonical_taxonomy_nodes(id, namespace_id, version_id, facet_id)
        ON DELETE RESTRICT
        DEFERRABLE INITIALLY IMMEDIATE,

    CONSTRAINT canonical_taxonomy_node_code_ck
        CHECK (canonical_code ~ '^[A-Z][A-Z0-9_]{0,95}$'),

    CONSTRAINT canonical_taxonomy_node_display_name_ck
        CHECK (char_length(display_name) BETWEEN 1 AND 160),

    CONSTRAINT canonical_taxonomy_node_description_ck
        CHECK (
            description IS NULL OR
            char_length(description) <= 2000
        ),

    CONSTRAINT canonical_taxonomy_node_position_ck
        CHECK (position BETWEEN 0 AND 100000),

    CONSTRAINT canonical_taxonomy_node_status_ck
        CHECK (status IN ('ACTIVE','ARCHIVED')),

    CONSTRAINT canonical_taxonomy_node_metadata_ck
        CHECK (jsonb_typeof(metadata) = 'object'),

    CONSTRAINT canonical_taxonomy_node_revision_ck
        CHECK (revision > 0),

    CONSTRAINT canonical_taxonomy_node_not_self_parent_ck
        CHECK (parent_id IS NULL OR parent_id <> id),

    CONSTRAINT canonical_taxonomy_node_archive_ck
        CHECK (
            (status = 'ACTIVE' AND archived_at IS NULL)
            OR
            (status = 'ARCHIVED' AND archived_at IS NOT NULL)
        )
);

CREATE INDEX canonical_taxonomy_nodes_version_facet_order_idx
    ON canonical_taxonomy_nodes(
        version_id,
        facet_id,
        parent_id,
        position,
        canonical_code
    );

CREATE INDEX canonical_taxonomy_nodes_parent_idx
    ON canonical_taxonomy_nodes(parent_id)
    WHERE parent_id IS NOT NULL;

-- ============================================================================
-- 6. Content immutability guards
-- ============================================================================

CREATE FUNCTION canonical_taxonomy_facet_draft_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    owner_namespace text;
    owner_version   text;
    version_status  text;
BEGIN
    IF TG_OP = 'DELETE' THEN
        owner_namespace := OLD.namespace_id;
        owner_version := OLD.version_id;
    ELSE
        owner_namespace := NEW.namespace_id;
        owner_version := NEW.version_id;
    END IF;

    IF TG_OP = 'UPDATE'
       AND (
            NEW.namespace_id <> OLD.namespace_id
            OR NEW.version_id <> OLD.version_id
       ) THEN
        RAISE EXCEPTION
            'canonical taxonomy facets cannot move between namespaces or versions';
    END IF;

    SELECT status
      INTO version_status
      FROM canonical_taxonomy_versions
     WHERE id = owner_version
       AND namespace_id = owner_namespace;

    IF version_status IS DISTINCT FROM 'DRAFT' THEN
        RAISE EXCEPTION
            'canonical taxonomy facet content is mutable only in DRAFT versions';
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER canonical_taxonomy_facet_draft_guard_trg
BEFORE INSERT OR UPDATE OR DELETE
ON canonical_taxonomy_facets
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_facet_draft_guard();

-- ============================================================================
-- 7. Node hierarchy + draft guard
-- ============================================================================
--
-- Maximum hierarchy depth is 12 edges from a node to its root.
-- FLAT facets cannot have parents.
-- Parent composite FK guarantees same namespace/version/facet.
-- Recursive validation rejects cycles.

CREATE FUNCTION canonical_taxonomy_node_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    owner_namespace text;
    owner_version   text;
    version_status  text;
    facet_structure text;
    parent_exists   boolean;
    cycle_found     boolean;
    ancestor_depth  integer;
BEGIN
    IF TG_OP = 'DELETE' THEN
        owner_namespace := OLD.namespace_id;
        owner_version := OLD.version_id;
    ELSE
        owner_namespace := NEW.namespace_id;
        owner_version := NEW.version_id;
    END IF;

    IF TG_OP = 'UPDATE'
       AND (
            NEW.namespace_id <> OLD.namespace_id
            OR NEW.version_id <> OLD.version_id
            OR NEW.facet_id <> OLD.facet_id
       ) THEN
        RAISE EXCEPTION
            'canonical taxonomy nodes cannot move between namespaces, versions or facets';
    END IF;

    SELECT status
      INTO version_status
      FROM canonical_taxonomy_versions
     WHERE id = owner_version
       AND namespace_id = owner_namespace;

    IF version_status IS DISTINCT FROM 'DRAFT' THEN
        RAISE EXCEPTION
            'canonical taxonomy node content is mutable only in DRAFT versions';
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;

    SELECT structure_type
      INTO facet_structure
      FROM canonical_taxonomy_facets
     WHERE id = NEW.facet_id
       AND namespace_id = NEW.namespace_id
       AND version_id = NEW.version_id;

    IF facet_structure IS NULL THEN
        RAISE EXCEPTION
            'canonical taxonomy facet does not exist in this namespace/version';
    END IF;

    IF facet_structure = 'FLAT' AND NEW.parent_id IS NOT NULL THEN
        RAISE EXCEPTION
            'FLAT canonical taxonomy facets cannot contain parent relationships';
    END IF;

    IF NEW.parent_id IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT EXISTS (
        SELECT 1
          FROM canonical_taxonomy_nodes
         WHERE id = NEW.parent_id
           AND namespace_id = NEW.namespace_id
           AND version_id = NEW.version_id
           AND facet_id = NEW.facet_id
    )
    INTO parent_exists;

    IF NOT parent_exists THEN
        RAISE EXCEPTION
            'canonical taxonomy parent must belong to the same namespace, version and facet';
    END IF;

    WITH RECURSIVE ancestry AS (
        SELECT
            n.id,
            n.parent_id,
            1 AS depth
        FROM canonical_taxonomy_nodes n
        WHERE n.id = NEW.parent_id
          AND n.namespace_id = NEW.namespace_id
          AND n.version_id = NEW.version_id
          AND n.facet_id = NEW.facet_id

        UNION ALL

        SELECT
            p.id,
            p.parent_id,
            a.depth + 1
        FROM canonical_taxonomy_nodes p
        JOIN ancestry a
          ON p.id = a.parent_id
        WHERE p.namespace_id = NEW.namespace_id
          AND p.version_id = NEW.version_id
          AND p.facet_id = NEW.facet_id
          AND a.depth <= 12
    )
    SELECT
        COALESCE(bool_or(id = NEW.id), false),
        COALESCE(max(depth), 0)
    INTO cycle_found, ancestor_depth
    FROM ancestry;

    IF cycle_found THEN
        RAISE EXCEPTION
            'canonical taxonomy hierarchy cycle detected';
    END IF;

    IF ancestor_depth > 12 THEN
        RAISE EXCEPTION
            'canonical taxonomy hierarchy exceeds maximum depth 12';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER canonical_taxonomy_node_guard_trg
BEFORE INSERT OR UPDATE OR DELETE
ON canonical_taxonomy_nodes
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_node_guard();

-- ============================================================================
-- 8. Domain command idempotency
-- ============================================================================
--
-- Section 13A has no HTTP API yet, therefore the HTTP idempotency middleware
-- is not sufficient as the only safety boundary. Internal taxonomy commands
-- receive a command_ref + fingerprint and persist their completed result here.
--
-- Same command_ref + same fingerprint may replay.
-- Same command_ref + different fingerprint must be rejected by the service.

CREATE TABLE canonical_taxonomy_commands (
    id                  text PRIMARY KEY,
    namespace_id        text NOT NULL
                            REFERENCES canonical_taxonomy_namespaces(id)
                            ON DELETE RESTRICT,
    command_ref         text NOT NULL,
    command_type        text NOT NULL,
    request_fingerprint text NOT NULL,
    result_type         text NOT NULL,
    result_ref          text NOT NULL,
    result_revision     bigint NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),

    UNIQUE (namespace_id, command_ref),

    CONSTRAINT canonical_taxonomy_command_ref_ck
        CHECK (char_length(command_ref) BETWEEN 1 AND 200),

    CONSTRAINT canonical_taxonomy_command_type_ck
        CHECK (char_length(command_type) BETWEEN 1 AND 120),

    CONSTRAINT canonical_taxonomy_command_fingerprint_ck
        CHECK (request_fingerprint ~ '^[a-f0-9]{64}$'),

    CONSTRAINT canonical_taxonomy_command_result_type_ck
        CHECK (char_length(result_type) BETWEEN 1 AND 120),

    CONSTRAINT canonical_taxonomy_command_result_ref_ck
        CHECK (char_length(result_ref) BETWEEN 1 AND 200),

    CONSTRAINT canonical_taxonomy_command_result_revision_ck
        CHECK (result_revision > 0)
);

-- ============================================================================
-- 9. Append-only change history
-- ============================================================================
--
-- This is canonical-taxonomy domain evidence, not a replacement for the
-- platform audit/outbox infrastructure.

CREATE TABLE canonical_taxonomy_change_history (
    id              text PRIMARY KEY,
    namespace_id    text NOT NULL,
    version_id      text NOT NULL,
    actor_ref       text NOT NULL,
    command_ref     text,
    change_type     text NOT NULL,
    aggregate_type  text NOT NULL,
    aggregate_ref   text NOT NULL,
    before_state    jsonb,
    after_state     jsonb,
    correlation_id  text,
    causation_id    text,
    occurred_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT canonical_taxonomy_change_version_fk
        FOREIGN KEY (version_id, namespace_id)
        REFERENCES canonical_taxonomy_versions(id, namespace_id)
        ON DELETE RESTRICT,

    CONSTRAINT canonical_taxonomy_change_actor_ck
        CHECK (char_length(actor_ref) BETWEEN 1 AND 200),

    CONSTRAINT canonical_taxonomy_change_command_ck
        CHECK (
            command_ref IS NULL OR
            char_length(command_ref) BETWEEN 1 AND 200
        ),

    CONSTRAINT canonical_taxonomy_change_type_ck
        CHECK (
            change_type IN (
                'DRAFT_CREATED',
                'DRAFT_CLONED',
                'FACET_CREATED',
                'FACET_UPDATED',
                'FACET_ARCHIVED',
                'FACET_RESTORED',
                'NODE_CREATED',
                'NODE_UPDATED',
                'NODE_ARCHIVED',
                'NODE_RESTORED'
            )
        ),

    CONSTRAINT canonical_taxonomy_change_aggregate_type_ck
        CHECK (
            aggregate_type IN (
                'TAXONOMY_VERSION',
                'TAXONOMY_FACET',
                'TAXONOMY_NODE'
            )
        ),

    CONSTRAINT canonical_taxonomy_change_aggregate_ref_ck
        CHECK (char_length(aggregate_ref) BETWEEN 1 AND 200),

    CONSTRAINT canonical_taxonomy_change_before_ck
        CHECK (
            before_state IS NULL OR
            jsonb_typeof(before_state) = 'object'
        ),

    CONSTRAINT canonical_taxonomy_change_after_ck
        CHECK (
            after_state IS NULL OR
            jsonb_typeof(after_state) = 'object'
        )
);

CREATE INDEX canonical_taxonomy_change_history_version_idx
    ON canonical_taxonomy_change_history(
        namespace_id,
        version_id,
        occurred_at,
        id
    );

-- ============================================================================
-- 10. Append-only publication history
-- ============================================================================

CREATE TABLE canonical_taxonomy_publication_history (
    id                  text PRIMARY KEY,
    namespace_id        text NOT NULL,
    version_id          text NOT NULL,
    previous_version_id text,
    event_type          text NOT NULL,
    actor_ref           text NOT NULL,
    command_ref         text,
    correlation_id      text,
    causation_id        text,
    metadata            jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT canonical_taxonomy_publication_version_fk
        FOREIGN KEY (version_id, namespace_id)
        REFERENCES canonical_taxonomy_versions(id, namespace_id)
        ON DELETE RESTRICT,

    CONSTRAINT canonical_taxonomy_publication_previous_fk
        FOREIGN KEY (previous_version_id, namespace_id)
        REFERENCES canonical_taxonomy_versions(id, namespace_id)
        ON DELETE RESTRICT,

    CONSTRAINT canonical_taxonomy_publication_previous_not_self_ck
        CHECK (
            previous_version_id IS NULL OR
            previous_version_id <> version_id
        ),

    CONSTRAINT canonical_taxonomy_publication_event_ck
        CHECK (event_type IN ('PUBLISHED','RETIRED')),

    CONSTRAINT canonical_taxonomy_publication_actor_ck
        CHECK (char_length(actor_ref) BETWEEN 1 AND 200),

    CONSTRAINT canonical_taxonomy_publication_command_ck
        CHECK (
            command_ref IS NULL OR
            char_length(command_ref) BETWEEN 1 AND 200
        ),

    CONSTRAINT canonical_taxonomy_publication_metadata_ck
        CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX canonical_taxonomy_publication_history_namespace_idx
    ON canonical_taxonomy_publication_history(
        namespace_id,
        occurred_at,
        id
    );

-- ============================================================================
-- 11. Append-only history protection
-- ============================================================================

CREATE FUNCTION canonical_taxonomy_history_append_only_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION
        'canonical taxonomy history is append-only';

    RETURN NULL;
END;
$$;

CREATE TRIGGER canonical_taxonomy_commands_append_only_trg
BEFORE UPDATE OR DELETE
ON canonical_taxonomy_commands
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_history_append_only_guard();

CREATE TRIGGER canonical_taxonomy_change_history_append_only_trg
BEFORE UPDATE OR DELETE
ON canonical_taxonomy_change_history
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_history_append_only_guard();

CREATE TRIGGER canonical_taxonomy_publication_history_append_only_trg
BEFORE UPDATE OR DELETE
ON canonical_taxonomy_publication_history
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_history_append_only_guard();

-- ============================================================================
-- 12. Namespace identity protection
-- ============================================================================
--
-- Namespace display/status may evolve through a controlled future platform
-- operation, but the machine code itself cannot silently change because it is
-- the discovery isolation boundary.

CREATE FUNCTION canonical_taxonomy_namespace_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION
            'canonical taxonomy namespaces cannot be deleted';
    END IF;

    IF NEW.code <> OLD.code THEN
        RAISE EXCEPTION
            'canonical taxonomy namespace code is immutable';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER canonical_taxonomy_namespace_guard_trg
BEFORE UPDATE OR DELETE
ON canonical_taxonomy_namespaces
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_namespace_guard();

