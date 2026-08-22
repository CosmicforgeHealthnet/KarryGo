DROP TRIGGER IF EXISTS
    canonical_taxonomy_governance_history_trigger
ON canonical_taxonomy_governance_cases;

DROP FUNCTION IF EXISTS
    canonical_taxonomy_record_governance_history();

DROP INDEX IF EXISTS
    canonical_taxonomy_governance_open_subject_uq;

DROP INDEX IF EXISTS
    canonical_taxonomy_governance_commands_case_idx;

DROP TABLE IF EXISTS
    canonical_taxonomy_governance_commands;


-- Restore 13C-1 governance guard.
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

    IF OLD.status IN (
        'RESOLVED',
        'REJECTED',
        'CANCELLED'
    )
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