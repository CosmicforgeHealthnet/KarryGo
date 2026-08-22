-- Section 13B — canonical taxonomy vocabulary foundation.
-- Adds namespace/version scoped lexical relationships without seeding
-- Food or Shop taxonomy content yet.

CREATE TABLE canonical_taxonomy_node_terms (
    id TEXT PRIMARY KEY,
    namespace_id TEXT NOT NULL REFERENCES canonical_taxonomy_namespaces(id) ON DELETE RESTRICT,
    version_id TEXT NOT NULL REFERENCES canonical_taxonomy_versions(id) ON DELETE CASCADE,
    node_id TEXT NOT NULL REFERENCES canonical_taxonomy_nodes(id) ON DELETE CASCADE,
    term TEXT NOT NULL,
    normalized_term TEXT NOT NULL,
    relationship_type TEXT NOT NULL CHECK (relationship_type IN ('ALIAS', 'SYNONYM', 'RELATED_TERM')),
    locale TEXT NOT NULL DEFAULT 'en-NG',
    region TEXT,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'ARCHIVED')),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT canonical_taxonomy_node_terms_term_not_blank
        CHECK (btrim(term) <> ''),
    CONSTRAINT canonical_taxonomy_node_terms_normalized_not_blank
        CHECK (btrim(normalized_term) <> ''),
    CONSTRAINT canonical_taxonomy_node_terms_locale_not_blank
        CHECK (btrim(locale) <> ''),
    CONSTRAINT canonical_taxonomy_node_terms_unique_mapping
        UNIQUE (namespace_id, version_id, normalized_term, relationship_type)
);

CREATE INDEX canonical_taxonomy_node_terms_node_idx
    ON canonical_taxonomy_node_terms (node_id);

CREATE INDEX canonical_taxonomy_node_terms_lookup_idx
    ON canonical_taxonomy_node_terms
    (namespace_id, version_id, normalized_term, status);

CREATE TABLE canonical_taxonomy_term_history (
    id BIGSERIAL PRIMARY KEY,
    namespace_id TEXT NOT NULL,
    version_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    term_id TEXT NOT NULL,
    operation TEXT NOT NULL CHECK (operation IN ('INSERT', 'UPDATE', 'DELETE')),
    before_state JSONB,
    after_state JSONB,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX canonical_taxonomy_term_history_term_idx
    ON canonical_taxonomy_term_history (term_id, recorded_at);

CREATE OR REPLACE FUNCTION canonical_taxonomy_validate_node_term()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    target_namespace_id TEXT;
    target_version_id TEXT;
    target_version_status TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        SELECT status
          INTO target_version_status
          FROM canonical_taxonomy_versions
         WHERE id = OLD.version_id;

        IF NOT FOUND THEN
            RAISE EXCEPTION 'taxonomy term version % not found', OLD.version_id;
        END IF;

        IF target_version_status <> 'DRAFT' THEN
            RAISE EXCEPTION 'published or retired taxonomy vocabulary is immutable';
        END IF;

        RETURN OLD;
    END IF;

    NEW.term := btrim(NEW.term);
    NEW.normalized_term := lower(regexp_replace(btrim(NEW.normalized_term), '\s+', ' ', 'g'));
    NEW.locale := btrim(NEW.locale);

    SELECT n.namespace_id, n.version_id
      INTO target_namespace_id, target_version_id
      FROM canonical_taxonomy_nodes n
     WHERE n.id = NEW.node_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'taxonomy node % not found', NEW.node_id;
    END IF;

    IF target_namespace_id <> NEW.namespace_id THEN
        RAISE EXCEPTION 'taxonomy term namespace does not own node';
    END IF;

    IF target_version_id <> NEW.version_id THEN
        RAISE EXCEPTION 'taxonomy term version does not own node';
    END IF;

    SELECT status
      INTO target_version_status
      FROM canonical_taxonomy_versions
     WHERE id = NEW.version_id
       AND namespace_id = NEW.namespace_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'taxonomy term namespace/version ownership invalid';
    END IF;

    IF target_version_status <> 'DRAFT' THEN
        RAISE EXCEPTION 'published or retired taxonomy vocabulary is immutable';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        NEW.revision := OLD.revision + 1;
    END IF;

    NEW.updated_at := now();
    RETURN NEW;
END;
$$;

CREATE TRIGGER canonical_taxonomy_node_terms_validate
BEFORE INSERT OR UPDATE OR DELETE ON canonical_taxonomy_node_terms
FOR EACH ROW EXECUTE FUNCTION canonical_taxonomy_validate_node_term();

CREATE OR REPLACE FUNCTION canonical_taxonomy_record_term_history()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO canonical_taxonomy_term_history
            (namespace_id, version_id, node_id, term_id, operation, before_state, after_state)
        VALUES
            (NEW.namespace_id, NEW.version_id, NEW.node_id, NEW.id, 'INSERT', NULL, to_jsonb(NEW));
        RETURN NEW;
    ELSIF TG_OP = 'UPDATE' THEN
        INSERT INTO canonical_taxonomy_term_history
            (namespace_id, version_id, node_id, term_id, operation, before_state, after_state)
        VALUES
            (NEW.namespace_id, NEW.version_id, NEW.node_id, NEW.id, 'UPDATE', to_jsonb(OLD), to_jsonb(NEW));
        RETURN NEW;
    ELSE
        INSERT INTO canonical_taxonomy_term_history
            (namespace_id, version_id, node_id, term_id, operation, before_state, after_state)
        VALUES
            (OLD.namespace_id, OLD.version_id, OLD.node_id, OLD.id, 'DELETE', to_jsonb(OLD), NULL);
        RETURN OLD;
    END IF;
END;
$$;

CREATE TRIGGER canonical_taxonomy_node_terms_history
AFTER INSERT OR UPDATE OR DELETE ON canonical_taxonomy_node_terms
FOR EACH ROW EXECUTE FUNCTION canonical_taxonomy_record_term_history();

CREATE OR REPLACE FUNCTION canonical_taxonomy_prevent_term_history_mutation()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'canonical taxonomy term history is append-only';
END;
$$;

CREATE TRIGGER canonical_taxonomy_term_history_append_only
BEFORE UPDATE OR DELETE ON canonical_taxonomy_term_history
FOR EACH ROW EXECUTE FUNCTION canonical_taxonomy_prevent_term_history_mutation();
