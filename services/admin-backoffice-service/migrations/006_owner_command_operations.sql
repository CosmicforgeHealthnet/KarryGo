-- Bridge orchestration metadata only. Each source owner remains the sole
-- business writer; these rows record identity, approval binding and the
-- owner's original immutable receipt, never a business state.
CREATE TABLE admin_command_approvals (
 id uuid PRIMARY KEY,
 owner text NOT NULL,
 contract text NOT NULL,
 requester_ref text NOT NULL,
 purpose_code text NOT NULL,
 product text NOT NULL CHECK(product='karryam'),
 market text NOT NULL,
 zone text NOT NULL,
 organization_ref text NOT NULL,
 tenant_ref text NOT NULL,
 subject_kind text NOT NULL,
 subject_id text NOT NULL,
 case_ref text NOT NULL,
 action text NOT NULL,
 expected_version bigint NOT NULL CHECK(expected_version>=0),
 intent_digest text NOT NULL CHECK(intent_digest ~ '^[0-9a-f]{64}$'),
 intent jsonb NOT NULL,
 status text NOT NULL CHECK(status IN ('PENDING','APPROVED','REJECTED','CONSUMED')),
 approver_ref text,
 decided_at timestamptz,
 consumed_operation_id uuid UNIQUE,
 expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK (approver_ref IS NULL OR approver_ref<>requester_ref),
 CHECK ((status='PENDING')=(approver_ref IS NULL AND decided_at IS NULL)),
 CHECK ((status='CONSUMED')=(consumed_operation_id IS NOT NULL))
);
CREATE INDEX admin_command_approvals_pending ON admin_command_approvals(owner,status,expires_at);
CREATE FUNCTION admin_command_approval_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP <> 'UPDATE' THEN RAISE EXCEPTION 'command approval identity is durable'; END IF;
 IF (to_jsonb(OLD)-ARRAY['status','approver_ref','decided_at','consumed_operation_id','updated_at']) IS DISTINCT FROM
    (to_jsonb(NEW)-ARRAY['status','approver_ref','decided_at','consumed_operation_id','updated_at']) THEN
  RAISE EXCEPTION 'command approval intent is immutable';
 END IF;
 IF NOT ((OLD.status='PENDING' AND NEW.status IN ('APPROVED','REJECTED')) OR (OLD.status='APPROVED' AND NEW.status='CONSUMED')) THEN
  RAISE EXCEPTION 'illegal command approval transition';
 END IF;
 IF OLD.status<>'PENDING' AND (NEW.approver_ref IS DISTINCT FROM OLD.approver_ref OR NEW.decided_at IS DISTINCT FROM OLD.decided_at) THEN
  RAISE EXCEPTION 'command approval decision is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER admin_command_approval_guard BEFORE UPDATE OR DELETE ON admin_command_approvals FOR EACH ROW EXECUTE FUNCTION admin_command_approval_guard();
CREATE TRIGGER admin_command_approval_no_truncate BEFORE TRUNCATE ON admin_command_approvals FOR EACH STATEMENT EXECUTE FUNCTION admin_command_approval_guard();
CREATE TABLE admin_command_approval_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 approval_id uuid NOT NULL REFERENCES admin_command_approvals(id),
 actor_ref text NOT NULL,
 event text NOT NULL CHECK(event IN ('PROPOSED','APPROVED','REJECTED','CONSUMED')),
 reason_code text NOT NULL,
 policy_id text NOT NULL,
 policy_version bigint NOT NULL CHECK(policy_version>0),
 policy_checksum text NOT NULL CHECK(policy_checksum ~ '^[0-9a-f]{64}$'),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER admin_command_approval_audit_immutable BEFORE UPDATE OR DELETE ON admin_command_approval_audit FOR EACH ROW EXECUTE FUNCTION admin_immutable_evidence();
CREATE TRIGGER admin_command_approval_audit_no_truncate BEFORE TRUNCATE ON admin_command_approval_audit FOR EACH STATEMENT EXECUTE FUNCTION admin_immutable_evidence();

CREATE TABLE admin_owner_command_operations (
 id uuid PRIMARY KEY,
 principal text NOT NULL CHECK(principal='cosmicforge-hq-karryam-admin'),
 owner text NOT NULL,
 contract text NOT NULL,
 staff_ref text NOT NULL,
 purpose_code text NOT NULL,
 product text NOT NULL CHECK(product='karryam'),
 market text NOT NULL,
 zone text NOT NULL,
 organization_ref text NOT NULL,
 tenant_ref text NOT NULL,
 subject_kind text NOT NULL,
 subject_id text NOT NULL,
 case_ref text NOT NULL,
 action text NOT NULL,
 version_key text NOT NULL,
 expected_version bigint NOT NULL CHECK(expected_version>=0),
 idempotency_key text NOT NULL UNIQUE,
 intent_digest text NOT NULL CHECK(intent_digest ~ '^[0-9a-f]{64}$'),
 operation_digest text NOT NULL CHECK(operation_digest ~ '^[0-9a-f]{64}$'),
 approval_id uuid UNIQUE REFERENCES admin_command_approvals(id),
 command jsonb NOT NULL,
 status text NOT NULL CHECK(status IN ('EXECUTING','UNKNOWN_OUTCOME','CONFIRMED','REJECTED')),
 claim_token uuid NOT NULL,
 receipt jsonb,
 rejection_code text,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((status='CONFIRMED')=(receipt IS NOT NULL)),
 CHECK ((status='REJECTED')=(rejection_code IS NOT NULL))
);
-- One live (executing, uncertain or confirmed) operation per versioned slot.
-- A definitively refused or superseded operation frees the slot; an uncertain
-- one never does, so an unresolved outcome cannot be raced by a new command.
CREATE UNIQUE INDEX admin_owner_command_live_slot ON admin_owner_command_operations(owner,subject_kind,subject_id,version_key,expected_version) WHERE status<>'REJECTED';
CREATE TABLE admin_owner_command_operation_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 operation_id uuid NOT NULL REFERENCES admin_owner_command_operations(id),
 staff_ref text NOT NULL,
 purpose_code text NOT NULL,
 product text NOT NULL CHECK(product='karryam'),
 market text NOT NULL,
 zone text NOT NULL,
 organization_ref text NOT NULL,
 tenant_ref text NOT NULL,
 policy_id text NOT NULL,
 policy_version bigint NOT NULL CHECK(policy_version>0),
 policy_checksum text NOT NULL CHECK(policy_checksum ~ '^[0-9a-f]{64}$'),
 outcome text NOT NULL CHECK(outcome IN ('EXECUTING','UNKNOWN_OUTCOME','CONFIRMED','REJECTED','RECONCILED')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER admin_owner_command_audit_immutable BEFORE UPDATE OR DELETE ON admin_owner_command_operation_audit FOR EACH ROW EXECUTE FUNCTION admin_immutable_evidence();
CREATE TRIGGER admin_owner_command_audit_no_truncate BEFORE TRUNCATE ON admin_owner_command_operation_audit FOR EACH STATEMENT EXECUTE FUNCTION admin_immutable_evidence();
CREATE FUNCTION admin_owner_command_operation_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP <> 'UPDATE' THEN RAISE EXCEPTION 'owner command identity is durable'; END IF;
 IF (to_jsonb(OLD)-ARRAY['status','receipt','rejection_code','updated_at']) IS DISTINCT FROM
    (to_jsonb(NEW)-ARRAY['status','receipt','rejection_code','updated_at']) THEN
  RAISE EXCEPTION 'owner command identity is immutable';
 END IF;
 IF OLD.status IN ('CONFIRMED','REJECTED') AND (NEW.status<>OLD.status OR NEW.receipt IS DISTINCT FROM OLD.receipt OR NEW.rejection_code IS DISTINCT FROM OLD.rejection_code) THEN
  RAISE EXCEPTION 'settled owner command is immutable';
 END IF;
 IF OLD.status='UNKNOWN_OUTCOME' AND NEW.status='EXECUTING' THEN
  RAISE EXCEPTION 'uncertain owner command cannot be retried';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER admin_owner_command_operation_guard BEFORE UPDATE OR DELETE ON admin_owner_command_operations FOR EACH ROW EXECUTE FUNCTION admin_owner_command_operation_guard();
CREATE TRIGGER admin_owner_command_operation_no_truncate BEFORE TRUNCATE ON admin_owner_command_operations FOR EACH STATEMENT EXECUTE FUNCTION admin_owner_command_operation_guard();
REVOKE ALL ON admin_command_approvals,admin_command_approval_audit,admin_owner_command_operations,admin_owner_command_operation_audit FROM PUBLIC;
