DROP INDEX IF EXISTS
    canonical_taxonomy_active_primary_per_facet_uq;

DROP INDEX IF EXISTS
    canonical_taxonomy_mappings_facet_idx;


-- Restore the 13C-1 mapping guard before removing facet_id.
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
        NEW.branch_ref := NULLIF(
            btrim(NEW.branch_ref),
            ''
        );
    END IF;

    IF NEW.business_ref IS NOT NULL THEN
        NEW.business_ref := NULLIF(
            btrim(NEW.business_ref),
            ''
        );
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


ALTER TABLE canonical_taxonomy_mappings
    DROP COLUMN IF EXISTS facet_id;