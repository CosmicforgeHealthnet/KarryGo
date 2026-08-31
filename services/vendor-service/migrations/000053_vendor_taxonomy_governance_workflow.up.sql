-- Section 13C-4
-- Governed taxonomy review workflow, command idempotency and audit history.

CREATE TABLE canonical_taxonomy_governance_commands (
    id TEXT PRIMARY KEY,

    command_ref TEXT NOT NULL UNIQUE,

    actor_ref TEXT NOT NULL,
    operation TEXT NOT NULL
        CHECK (
            operation IN (
                'OPEN_CASE',
                'START_REVIEW',
                'ASSIGN_CASE',
                'RESOLVE_CASE',
                'REJECT_CASE',
                'CANCEL_CASE'
            )
        ),

    case_id TEXT,

    request_hash TEXT NOT NULL,
    result_snapshot JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT canonical_taxonomy_governance_command_ref_ck
        CHECK (btrim(command_ref) <> ''),

    CONSTRAINT canonical_taxonomy_governance_command_actor_ck
        CHECK (btrim(actor_ref) <> ''),

    CONSTRAINT canonical_taxonomy_governance_command_hash_ck
        CHECK (btrim(request_hash) <> '')
);

CREATE INDEX canonical_taxonomy_governance_commands_case_idx
    ON canonical_taxonomy_governance_commands (
        case_id,
        created_at
    );


CREATE UNIQUE INDEX
    canonical_taxonomy_governance_open_subject_uq
    ON canonical_taxonomy_governance_cases (
        namespace_id,
        version_id,
        subject_type,
        subject_ref,
        case_type
    )
    WHERE status IN ('OPEN','IN_REVIEW');


CREATE OR REPLACE FUNCTION canonical_taxonomy_governance_case_guard()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    version_namespace TEXT;
BEGIN
    NEW.subject_type := upper(btrim(NEW.subject_type));
    NEW.subject_ref := btrim(NEW.subject_ref);

    IF NEW.assigned_actor_ref IS NOT NULL THEN
        NEW.assigned_actor_ref :=
            NULLIF(btrim(NEW.assigned_actor_ref), '');
    END IF;

    IF NEW.reason_code IS NOT NULL THEN
        NEW.reason_code :=
            NULLIF(upper(btrim(NEW.reason_code)), '');
    END IF;

    IF NEW.resolution_code IS NOT NULL THEN
        NEW.resolution_code :=
            NULLIF(upper(btrim(NEW.resolution_code)), '');
    END IF;

    SELECT namespace_id
      INTO version_namespace
      FROM canonical_taxonomy_versions
     WHERE id=NEW.version_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION
            'taxonomy governance version not found';
    END IF;

    IF version_namespace <> NEW.namespace_id THEN
        RAISE EXCEPTION
            'taxonomy governance namespace/version ownership invalid';
    END IF;

    IF TG_OP = 'INSERT' THEN
        IF NEW.status <> 'OPEN' THEN
            RAISE EXCEPTION
                'new taxonomy governance case must start OPEN';
        END IF;

        NEW.revision := 1;
        NEW.updated_at := NEW.created_at;

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
    ) THEN
        RAISE EXCEPTION
            'closed taxonomy governance case is immutable';
    END IF;

    IF NEW.status <> OLD.status THEN
        IF OLD.status = 'OPEN'
           AND NEW.status NOT IN (
               'IN_REVIEW',
               'RESOLVED',
               'REJECTED',
               'CANCELLED'
           ) THEN
            RAISE EXCEPTION
                'invalid taxonomy governance transition from OPEN';
        END IF;

        IF OLD.status = 'IN_REVIEW'
           AND NEW.status NOT IN (
               'RESOLVED',
               'REJECTED',
               'CANCELLED'
           ) THEN
            RAISE EXCEPTION
                'invalid taxonomy governance transition from IN_REVIEW';
        END IF;
    END IF;

    IF NEW.status = 'IN_REVIEW'
       AND (
           NEW.assigned_actor_ref IS NULL
           OR btrim(NEW.assigned_actor_ref) = ''
       ) THEN
        RAISE EXCEPTION
            'IN_REVIEW governance case requires assignee';
    END IF;

    IF NEW.status IN (
        'RESOLVED',
        'REJECTED'
    ) THEN
        IF NEW.resolved_by_actor_ref IS NULL
           OR btrim(NEW.resolved_by_actor_ref) = '' THEN
            RAISE EXCEPTION
                'closed governance case requires resolver';
        END IF;

        IF NEW.resolution_code IS NULL
           OR btrim(NEW.resolution_code) = '' THEN
            RAISE EXCEPTION
                'resolved/rejected governance case requires resolution code';
        END IF;

        IF NEW.resolved_at IS NULL THEN
            NEW.resolved_at := now();
        END IF;
    ELSIF NEW.status = 'CANCELLED' THEN
        IF NEW.resolved_by_actor_ref IS NULL
           OR btrim(NEW.resolved_by_actor_ref) = '' THEN
            RAISE EXCEPTION
                'cancelled governance case requires actor';
        END IF;

        IF NEW.resolved_at IS NULL THEN
            NEW.resolved_at := now();
        END IF;
    ELSE
        NEW.resolved_by_actor_ref := NULL;
        NEW.resolved_at := NULL;
    END IF;

    NEW.revision := OLD.revision + 1;
    NEW.updated_at := now();

    RETURN NEW;
END;
$$;


CREATE OR REPLACE FUNCTION canonical_taxonomy_record_governance_history()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    operation_name TEXT;
    operation_actor TEXT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        operation_name := 'OPENED';
        operation_actor := NEW.opened_by_actor_ref;

        INSERT INTO canonical_taxonomy_governance_history (
            case_id,
            operation,
            actor_ref,
            before_state,
            after_state
        )
        VALUES (
            NEW.id,
            operation_name,
            operation_actor,
            NULL,
            to_jsonb(NEW)
        );

        RETURN NEW;
    END IF;

    IF NEW.status <> OLD.status THEN
        operation_name :=
            CASE NEW.status
                WHEN 'IN_REVIEW' THEN 'ASSIGNED'
                WHEN 'RESOLVED' THEN 'RESOLVED'
                WHEN 'REJECTED' THEN 'REJECTED'
                WHEN 'CANCELLED' THEN 'CANCELLED'
                ELSE 'UPDATED'
            END;
    ELSIF NEW.assigned_actor_ref IS DISTINCT FROM
          OLD.assigned_actor_ref THEN
        operation_name := 'ASSIGNED';
    ELSE
        operation_name := 'UPDATED';
    END IF;

    operation_actor :=
        COALESCE(
            NEW.resolved_by_actor_ref,
            NEW.assigned_actor_ref,
            NEW.opened_by_actor_ref
        );

    INSERT INTO canonical_taxonomy_governance_history (
        case_id,
        operation,
        actor_ref,
        before_state,
        after_state
    )
    VALUES (
        NEW.id,
        operation_name,
        operation_actor,
        to_jsonb(OLD),
        to_jsonb(NEW)
    );

    RETURN NEW;
END;
$$;


CREATE TRIGGER canonical_taxonomy_governance_history_trigger
AFTER INSERT OR UPDATE
ON canonical_taxonomy_governance_cases
FOR EACH ROW
EXECUTE FUNCTION canonical_taxonomy_record_governance_history();