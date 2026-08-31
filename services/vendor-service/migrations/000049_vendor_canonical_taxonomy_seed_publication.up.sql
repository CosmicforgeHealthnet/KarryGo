-- Section 13B-4
-- Idempotent canonical taxonomy seed publication and external-authority
-- vocabulary persistence.

CREATE TABLE canonical_taxonomy_seed_runs (
    id TEXT PRIMARY KEY,
    namespace_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_namespaces(id) ON DELETE RESTRICT,
    seed_key TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    version_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_versions(id) ON DELETE RESTRICT,
    status TEXT NOT NULL
        CHECK (status IN ('PREPARED', 'PUBLISHED')),
    facet_count INTEGER NOT NULL DEFAULT 0 CHECK (facet_count >= 0),
    node_count INTEGER NOT NULL DEFAULT 0 CHECK (node_count >= 0),
    term_count INTEGER NOT NULL DEFAULT 0 CHECK (term_count >= 0),
    authority_vocabulary_count INTEGER NOT NULL DEFAULT 0
        CHECK (authority_vocabulary_count >= 0),
    actor_ref TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,

    CONSTRAINT canonical_taxonomy_seed_runs_identity_uq
        UNIQUE (namespace_id, seed_key, fingerprint),

    CONSTRAINT canonical_taxonomy_seed_runs_version_uq
        UNIQUE (version_id),

    CONSTRAINT canonical_taxonomy_seed_runs_seed_key_not_blank
        CHECK (btrim(seed_key) <> ''),

    CONSTRAINT canonical_taxonomy_seed_runs_fingerprint_not_blank
        CHECK (btrim(fingerprint) <> ''),

    CONSTRAINT canonical_taxonomy_seed_runs_actor_not_blank
        CHECK (btrim(actor_ref) <> '')
);

CREATE INDEX canonical_taxonomy_seed_runs_namespace_idx
    ON canonical_taxonomy_seed_runs
    (namespace_id, status, created_at DESC);


CREATE OR REPLACE FUNCTION canonical_taxonomy_validate_seed_run_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'canonical taxonomy seed run evidence is append-only';
    END IF;

    IF NEW.id <> OLD.id
       OR NEW.namespace_id <> OLD.namespace_id
       OR NEW.seed_key <> OLD.seed_key
       OR NEW.fingerprint <> OLD.fingerprint
       OR NEW.version_id <> OLD.version_id
       OR NEW.facet_count <> OLD.facet_count
       OR NEW.node_count <> OLD.node_count
       OR NEW.term_count <> OLD.term_count
       OR NEW.authority_vocabulary_count <> OLD.authority_vocabulary_count
       OR NEW.actor_ref <> OLD.actor_ref
       OR NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION 'canonical taxonomy seed run identity/evidence is immutable';
    END IF;

    IF OLD.status = 'PUBLISHED' AND NEW.status <> 'PUBLISHED' THEN
        RAISE EXCEPTION 'published taxonomy seed run cannot regress';
    END IF;

    IF OLD.status = 'PREPARED'
       AND NEW.status NOT IN ('PREPARED', 'PUBLISHED') THEN
        RAISE EXCEPTION 'invalid taxonomy seed run transition';
    END IF;

    NEW.updated_at := now();

    RETURN NEW;
END;
$$;

CREATE TRIGGER canonical_taxonomy_seed_runs_guard
BEFORE UPDATE OR DELETE ON canonical_taxonomy_seed_runs
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_validate_seed_run_mutation();


CREATE TABLE canonical_taxonomy_authority_vocabulary (
    id TEXT PRIMARY KEY,

    namespace_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_namespaces(id) ON DELETE RESTRICT,

    version_id TEXT NOT NULL
        REFERENCES canonical_taxonomy_versions(id) ON DELETE CASCADE,

    authority_type TEXT NOT NULL,
    authority_ref TEXT NOT NULL,
    authority_code TEXT NOT NULL,

    term TEXT NOT NULL,
    normalized_term TEXT NOT NULL,

    locale TEXT NOT NULL DEFAULT 'en-NG',
    region TEXT,

    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(metadata) = 'object'),

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT canonical_taxonomy_authority_type_not_blank
        CHECK (btrim(authority_type) <> ''),

    CONSTRAINT canonical_taxonomy_authority_ref_not_blank
        CHECK (btrim(authority_ref) <> ''),

    CONSTRAINT canonical_taxonomy_authority_code_not_blank
        CHECK (btrim(authority_code) <> ''),

    CONSTRAINT canonical_taxonomy_authority_term_not_blank
        CHECK (btrim(term) <> ''),

    CONSTRAINT canonical_taxonomy_authority_normalized_not_blank
        CHECK (btrim(normalized_term) <> ''),

    CONSTRAINT canonical_taxonomy_authority_identity_uq
        UNIQUE (
            namespace_id,
            version_id,
            authority_type,
            authority_ref,
            normalized_term
        )
);

CREATE INDEX canonical_taxonomy_authority_lookup_idx
    ON canonical_taxonomy_authority_vocabulary
    (namespace_id, version_id, normalized_term);


CREATE TABLE canonical_taxonomy_authority_vocabulary_history (
    id BIGSERIAL PRIMARY KEY,

    vocabulary_id TEXT NOT NULL,
    namespace_id TEXT NOT NULL,
    version_id TEXT NOT NULL,

    operation TEXT NOT NULL
        CHECK (operation IN ('INSERT', 'UPDATE', 'DELETE')),

    before_state JSONB,
    after_state JSONB,

    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);


CREATE OR REPLACE FUNCTION canonical_taxonomy_validate_authority_vocabulary()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    version_namespace TEXT;
    version_status TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        SELECT namespace_id, status
          INTO version_namespace, version_status
          FROM canonical_taxonomy_versions
         WHERE id = OLD.version_id;

        IF NOT FOUND THEN
            RAISE EXCEPTION 'taxonomy authority vocabulary version not found';
        END IF;

        IF version_status <> 'DRAFT' THEN
            RAISE EXCEPTION
                'published or retired authority vocabulary is immutable';
        END IF;

        RETURN OLD;
    END IF;

    NEW.term := btrim(NEW.term);

    NEW.normalized_term :=
        lower(
            regexp_replace(
                btrim(NEW.normalized_term),
                '\s+',
                ' ',
                'g'
            )
        );

    NEW.locale := btrim(NEW.locale);

    SELECT namespace_id, status
      INTO version_namespace, version_status
      FROM canonical_taxonomy_versions
     WHERE id = NEW.version_id;

    IF NOT FOUND OR version_namespace <> NEW.namespace_id THEN
        RAISE EXCEPTION
            'authority vocabulary namespace/version ownership invalid';
    END IF;

    IF version_status <> 'DRAFT' THEN
        RAISE EXCEPTION
            'published or retired authority vocabulary is immutable';
    END IF;

    NEW.updated_at := now();

    RETURN NEW;
END;
$$;

CREATE TRIGGER canonical_taxonomy_authority_vocabulary_validate
BEFORE INSERT OR UPDATE OR DELETE
ON canonical_taxonomy_authority_vocabulary
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_validate_authority_vocabulary();


CREATE OR REPLACE FUNCTION canonical_taxonomy_record_authority_vocabulary_history()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO canonical_taxonomy_authority_vocabulary_history (
            vocabulary_id,
            namespace_id,
            version_id,
            operation,
            before_state,
            after_state
        )
        VALUES (
            NEW.id,
            NEW.namespace_id,
            NEW.version_id,
            'INSERT',
            NULL,
            to_jsonb(NEW)
        );

        RETURN NEW;

    ELSIF TG_OP = 'UPDATE' THEN
        INSERT INTO canonical_taxonomy_authority_vocabulary_history (
            vocabulary_id,
            namespace_id,
            version_id,
            operation,
            before_state,
            after_state
        )
        VALUES (
            NEW.id,
            NEW.namespace_id,
            NEW.version_id,
            'UPDATE',
            to_jsonb(OLD),
            to_jsonb(NEW)
        );

        RETURN NEW;

    ELSE
        INSERT INTO canonical_taxonomy_authority_vocabulary_history (
            vocabulary_id,
            namespace_id,
            version_id,
            operation,
            before_state,
            after_state
        )
        VALUES (
            OLD.id,
            OLD.namespace_id,
            OLD.version_id,
            'DELETE',
            to_jsonb(OLD),
            NULL
        );

        RETURN OLD;
    END IF;
END;
$$;

CREATE TRIGGER canonical_taxonomy_authority_vocabulary_history
AFTER INSERT OR UPDATE OR DELETE
ON canonical_taxonomy_authority_vocabulary
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_record_authority_vocabulary_history();


CREATE OR REPLACE FUNCTION canonical_taxonomy_prevent_authority_history_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION
        'canonical taxonomy authority vocabulary history is append-only';
END;
$$;

CREATE TRIGGER canonical_taxonomy_authority_history_append_only
BEFORE UPDATE OR DELETE
ON canonical_taxonomy_authority_vocabulary_history
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_prevent_authority_history_mutation();