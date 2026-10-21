-- Source-owned Admin authorization context/read schema. No production grants.
-- shared-template-sha256: d4d870f4c55730d53cc4a01f344bfd505f902d3685222355056f86f1d961aa40
-- Install only through the consuming source owner's normal migration runner.
-- Each owner keeps these records in its own database; Admin never connects.
CREATE TABLE IF NOT EXISTS admin_owner_context_heads (
 owner text NOT NULL,context_id text NOT NULL,revision bigint NOT NULL CHECK(revision>0),
 checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),resource_kind text NOT NULL,resource_ref text NOT NULL,
 product text NOT NULL,market text NOT NULL,zone text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL,
 capabilities text[] NOT NULL,
 PRIMARY KEY(owner,context_id)
);
CREATE INDEX IF NOT EXISTS admin_owner_context_resource ON admin_owner_context_heads(owner,resource_kind,resource_ref);
CREATE INDEX IF NOT EXISTS admin_owner_context_scope ON admin_owner_context_heads(owner,resource_kind,product,market,zone,organization_ref,tenant_ref,resource_ref);
CREATE TABLE IF NOT EXISTS admin_owner_context_history (
 owner text NOT NULL,context_id text NOT NULL,revision bigint NOT NULL CHECK(revision>0),
 checksum text NOT NULL,envelope bytea NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(owner,context_id,revision)
);
CREATE OR REPLACE FUNCTION reject_admin_owner_context_history_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Source-owner authorization history is append-only'; END $$;
DROP TRIGGER IF EXISTS admin_owner_context_history_immutable ON admin_owner_context_history;
CREATE TRIGGER admin_owner_context_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_context_history FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_context_history_mutation();

CREATE TABLE IF NOT EXISTS admin_owner_policy_revisions (
 policy_id text PRIMARY KEY,version bigint NOT NULL CHECK(version>0),checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),accepted_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS admin_owner_read_nonces (
 owner text NOT NULL,nonce_hash text NOT NULL,expires_at timestamptz NOT NULL,
 PRIMARY KEY(owner,nonce_hash)
);
CREATE INDEX IF NOT EXISTS admin_owner_read_nonce_expiry ON admin_owner_read_nonces(expires_at);
CREATE TABLE IF NOT EXISTS admin_owner_context_preparation_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner text NOT NULL,resource_kind text NOT NULL,target_ref text NOT NULL,
 workload_principal text NOT NULL,purpose_code text NOT NULL,binding_fingerprint text NOT NULL,source_contract text NOT NULL,
 known_source_dimensions jsonb NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now()
);
DROP TRIGGER IF EXISTS admin_owner_context_preparation_immutable ON admin_owner_context_preparation_audit;
CREATE TRIGGER admin_owner_context_preparation_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_context_preparation_audit FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_context_history_mutation();
CREATE TABLE IF NOT EXISTS admin_owner_read_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner text NOT NULL,resource_kind text NOT NULL,target_ref text NOT NULL,
 staff_ref text NOT NULL,purpose_code text NOT NULL,market text NOT NULL,zone text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL,
 policy_id text NOT NULL,policy_version bigint NOT NULL,policy_checksum text NOT NULL,as_of timestamptz NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now(),outcome text NOT NULL DEFAULT 'AUTHORIZED'
);
CREATE OR REPLACE FUNCTION reject_admin_owner_read_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Owner administrative read audit is append-only'; END $$;
DROP TRIGGER IF EXISTS admin_owner_read_audit_immutable ON admin_owner_read_audit;
CREATE TRIGGER admin_owner_read_audit_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_read_audit FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_read_audit_mutation();
