-- Private Wallet owner boundary. No runtime grant, policy or activation is
-- installed. Admin remains unable to connect to Wallet's database directly.
CREATE TABLE admin_owner_context_heads (
 owner text NOT NULL,context_id text NOT NULL,revision bigint NOT NULL CHECK(revision>0),
 checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),resource_kind text NOT NULL,resource_ref text NOT NULL,
 product text NOT NULL,market text NOT NULL,zone text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL,
 capabilities text[] NOT NULL,PRIMARY KEY(owner,context_id)
);
CREATE INDEX admin_owner_context_resource ON admin_owner_context_heads(owner,resource_kind,resource_ref);
CREATE INDEX admin_owner_context_scope ON admin_owner_context_heads(owner,resource_kind,product,market,zone,organization_ref,tenant_ref,resource_ref);
CREATE TABLE admin_owner_context_history (
 owner text NOT NULL,context_id text NOT NULL,revision bigint NOT NULL CHECK(revision>0),checksum text NOT NULL,
 envelope bytea NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(owner,context_id,revision)
);
CREATE FUNCTION reject_wallet_admin_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Wallet administrative evidence is append-only'; END $$;
CREATE TRIGGER admin_owner_context_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_context_history FOR EACH STATEMENT EXECUTE FUNCTION reject_wallet_admin_immutable();
CREATE TABLE admin_owner_policy_revisions (
 policy_id text PRIMARY KEY,version bigint NOT NULL CHECK(version>0),checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),accepted_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE admin_owner_read_nonces (
 owner text NOT NULL,nonce_hash text NOT NULL,expires_at timestamptz NOT NULL,PRIMARY KEY(owner,nonce_hash)
);
CREATE INDEX admin_owner_read_nonce_expiry ON admin_owner_read_nonces(expires_at);
CREATE TABLE admin_owner_context_preparation_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner text NOT NULL,resource_kind text NOT NULL,target_ref text NOT NULL,
 workload_principal text NOT NULL,purpose_code text NOT NULL,binding_fingerprint text NOT NULL,source_contract text NOT NULL,
 known_source_dimensions jsonb NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER admin_owner_context_preparation_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_context_preparation_audit FOR EACH STATEMENT EXECUTE FUNCTION reject_wallet_admin_immutable();
CREATE TABLE admin_owner_read_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner text NOT NULL,resource_kind text NOT NULL,target_ref text NOT NULL,
 staff_ref text NOT NULL,purpose_code text NOT NULL,market text NOT NULL,zone text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL,
 policy_id text NOT NULL,policy_version bigint NOT NULL,policy_checksum text NOT NULL,as_of timestamptz NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT now(),outcome text NOT NULL DEFAULT 'AUTHORIZED'
);
CREATE TRIGGER admin_owner_read_audit_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_read_audit FOR EACH STATEMENT EXECUTE FUNCTION reject_wallet_admin_immutable();

CREATE TABLE admin_owner_command_decisions (
 idempotency_key text PRIMARY KEY,operation_id uuid NOT NULL UNIQUE,decision_id uuid NOT NULL UNIQUE,
 owner text NOT NULL,contract text NOT NULL,subject_kind text NOT NULL,subject_id text NOT NULL,case_ref text NOT NULL DEFAULT '',
 action text NOT NULL,version_scope text NOT NULL CHECK(version_scope IN ('SUBJECT','CASE')),version_key text NOT NULL,
 recorded_version bigint NOT NULL CHECK(recorded_version>0),operation_digest text NOT NULL CHECK(operation_digest ~ '^[0-9a-f]{64}$'),
 intent_digest text NOT NULL CHECK(intent_digest ~ '^[0-9a-f]{64}$'),approval_id uuid UNIQUE,approver_ref text,staff_ref text NOT NULL,
 purpose_code text NOT NULL,product text NOT NULL,market text NOT NULL,zone text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL,
 policy_id text NOT NULL,policy_version bigint NOT NULL CHECK(policy_version>0),policy_checksum text NOT NULL CHECK(policy_checksum ~ '^[0-9a-f]{64}$'),
 receipt jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),
 CHECK((approval_id IS NULL)=(approver_ref IS NULL)),CHECK(approver_ref IS NULL OR approver_ref<>staff_ref),
 UNIQUE(owner,subject_kind,subject_id,version_scope,version_key,recorded_version)
);
CREATE INDEX admin_owner_command_decisions_subject ON admin_owner_command_decisions(owner,subject_kind,subject_id,created_at DESC);
CREATE TRIGGER admin_owner_command_decisions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_command_decisions FOR EACH STATEMENT EXECUTE FUNCTION reject_wallet_admin_immutable();

-- Financial cases are evidence and coordination records, never an alternate
-- ledger. The optional owner_operation_ref points at a native Wallet aggregate.
CREATE TABLE wallet_admin_cases (
 case_ref text PRIMARY KEY,organization_id text NOT NULL,subject_kind text NOT NULL,subject_id text NOT NULL,
 category text NOT NULL,severity text NOT NULL CHECK(severity IN ('LOW','MEDIUM','HIGH','CRITICAL')),
 state text NOT NULL CHECK(state IN ('OPEN','ESCALATED','REVIEWED','OWNER_ACTION_REQUESTED','CLOSED')),
 version bigint NOT NULL CHECK(version>0),owner_operation_ref text NOT NULL DEFAULT '',opened_by text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE wallet_admin_case_history (
 id uuid PRIMARY KEY,case_ref text NOT NULL REFERENCES wallet_admin_cases(case_ref),operation_id uuid NOT NULL UNIQUE,
 action text NOT NULL,prior_state text NOT NULL,resulting_state text NOT NULL,prior_version bigint NOT NULL,resulting_version bigint NOT NULL,
 staff_ref text NOT NULL,reason_code text NOT NULL,owner_operation_ref text NOT NULL DEFAULT '',occurred_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(case_ref,resulting_version)
);
CREATE TRIGGER wallet_admin_case_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON wallet_admin_case_history FOR EACH STATEMENT EXECUTE FUNCTION reject_wallet_admin_immutable();
CREATE FUNCTION protect_wallet_admin_case_identity() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.case_ref IS DISTINCT FROM OLD.case_ref OR NEW.organization_id IS DISTINCT FROM OLD.organization_id OR NEW.subject_kind IS DISTINCT FROM OLD.subject_kind OR NEW.subject_id IS DISTINCT FROM OLD.subject_id OR NEW.category IS DISTINCT FROM OLD.category OR NEW.severity IS DISTINCT FROM OLD.severity OR NEW.opened_by IS DISTINCT FROM OLD.opened_by OR NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.version<>OLD.version+1 OR (OLD.owner_operation_ref<>'' AND NEW.owner_operation_ref IS DISTINCT FROM OLD.owner_operation_ref) THEN RAISE EXCEPTION 'Wallet case identity is immutable and transitions are versioned'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER wallet_admin_case_identity BEFORE UPDATE ON wallet_admin_cases FOR EACH ROW EXECUTE FUNCTION protect_wallet_admin_case_identity();

-- Admin staff are never granted roles by this migration. The existing Wallet
-- finance RBAC and approval policies remain the independent source authority.
