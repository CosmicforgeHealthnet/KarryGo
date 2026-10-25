-- Secure Admin owner context, privacy-minimized reads and immutable command
-- decisions. No staff grant, scope mapping or production policy is seeded.
ALTER TABLE referral_persons ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1 CHECK (version > 0);
ALTER TABLE referrals ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1 CHECK (version > 0);
ALTER TABLE reward_ledger ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1 CHECK (version > 0);
ALTER TABLE fraud_flags ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1 CHECK (version > 0);

CREATE TABLE IF NOT EXISTS referral_admin_staff_grants (
 id text PRIMARY KEY,
 staff_ref text NOT NULL,
 permission text NOT NULL,
 granted_by text NOT NULL,
 approval_ref text NOT NULL,
 expires_at timestamptz,
 revoked_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(staff_ref,permission)
);
CREATE INDEX IF NOT EXISTS referral_admin_staff_grants_current ON referral_admin_staff_grants(staff_ref,permission) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS admin_owner_context_heads (
 owner text NOT NULL,context_id text NOT NULL,revision bigint NOT NULL CHECK(revision>0),
 checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),resource_kind text NOT NULL,resource_ref text NOT NULL,
 product text NOT NULL,market text NOT NULL,zone text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL,
 capabilities text[] NOT NULL,PRIMARY KEY(owner,context_id));
CREATE INDEX IF NOT EXISTS admin_owner_context_resource ON admin_owner_context_heads(owner,resource_kind,resource_ref);
CREATE INDEX IF NOT EXISTS admin_owner_context_scope ON admin_owner_context_heads(owner,resource_kind,product,market,zone,organization_ref,tenant_ref,resource_ref);
CREATE TABLE IF NOT EXISTS admin_owner_context_history (
 owner text NOT NULL,context_id text NOT NULL,revision bigint NOT NULL CHECK(revision>0),checksum text NOT NULL,envelope bytea NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(owner,context_id,revision));
CREATE OR REPLACE FUNCTION reject_admin_owner_context_history_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Source-owner authorization history is append-only'; END $$;
DROP TRIGGER IF EXISTS admin_owner_context_history_immutable ON admin_owner_context_history;
CREATE TRIGGER admin_owner_context_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_context_history FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_context_history_mutation();
CREATE TABLE IF NOT EXISTS admin_owner_policy_revisions (
 policy_id text PRIMARY KEY,version bigint NOT NULL CHECK(version>0),checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),accepted_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS admin_owner_read_nonces (owner text NOT NULL,nonce_hash text NOT NULL,expires_at timestamptz NOT NULL,PRIMARY KEY(owner,nonce_hash));
CREATE INDEX IF NOT EXISTS admin_owner_read_nonce_expiry ON admin_owner_read_nonces(expires_at);
CREATE TABLE IF NOT EXISTS admin_owner_context_preparation_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner text NOT NULL,resource_kind text NOT NULL,target_ref text NOT NULL,workload_principal text NOT NULL,
 purpose_code text NOT NULL,binding_fingerprint text NOT NULL,source_contract text NOT NULL,known_source_dimensions jsonb NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now());
DROP TRIGGER IF EXISTS admin_owner_context_preparation_immutable ON admin_owner_context_preparation_audit;
CREATE TRIGGER admin_owner_context_preparation_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_context_preparation_audit FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_context_history_mutation();
CREATE TABLE IF NOT EXISTS admin_owner_read_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner text NOT NULL,resource_kind text NOT NULL,target_ref text NOT NULL,staff_ref text NOT NULL,purpose_code text NOT NULL,
 market text NOT NULL,zone text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL,policy_id text NOT NULL,policy_version bigint NOT NULL,
 policy_checksum text NOT NULL,as_of timestamptz NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now(),outcome text NOT NULL DEFAULT 'AUTHORIZED');
CREATE OR REPLACE FUNCTION reject_admin_owner_read_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Owner administrative read audit is append-only'; END $$;
DROP TRIGGER IF EXISTS admin_owner_read_audit_immutable ON admin_owner_read_audit;
CREATE TRIGGER admin_owner_read_audit_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_read_audit FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_read_audit_mutation();

CREATE TABLE IF NOT EXISTS admin_owner_command_decisions (
 idempotency_key text PRIMARY KEY,operation_id uuid NOT NULL UNIQUE,decision_id uuid NOT NULL UNIQUE,owner text NOT NULL,contract text NOT NULL,
 subject_kind text NOT NULL,subject_id text NOT NULL,case_ref text NOT NULL DEFAULT '',action text NOT NULL,version_scope text NOT NULL CHECK (version_scope IN ('SUBJECT','CASE')),
 version_key text NOT NULL,recorded_version bigint NOT NULL CHECK (recorded_version > 0),operation_digest text NOT NULL CHECK (operation_digest ~ '^[0-9a-f]{64}$'),
 intent_digest text NOT NULL CHECK (intent_digest ~ '^[0-9a-f]{64}$'),approval_id uuid UNIQUE,approver_ref text,staff_ref text NOT NULL,purpose_code text NOT NULL,
 product text NOT NULL,market text NOT NULL,zone text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL,policy_id text NOT NULL,
 policy_version bigint NOT NULL CHECK (policy_version > 0),policy_checksum text NOT NULL CHECK (policy_checksum ~ '^[0-9a-f]{64}$'),receipt jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),CHECK ((approval_id IS NULL) = (approver_ref IS NULL)),CHECK (approver_ref IS NULL OR approver_ref <> staff_ref),
 UNIQUE (owner,subject_kind,subject_id,version_scope,version_key,recorded_version));
CREATE INDEX IF NOT EXISTS admin_owner_command_decisions_subject ON admin_owner_command_decisions(owner,subject_kind,subject_id,created_at DESC);
CREATE OR REPLACE FUNCTION reject_admin_owner_command_decision_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Owner administrative command decisions are append-only'; END $$;
DROP TRIGGER IF EXISTS admin_owner_command_decisions_immutable ON admin_owner_command_decisions;
CREATE TRIGGER admin_owner_command_decisions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_command_decisions FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_command_decision_mutation();
