-- Producer-owned authorization allocations over Analytics-local canonical events.
-- No scopes, privacy units, membership rosters or production grants are backfilled.
-- No reporting capability is enabled by installing this migration.
-- Shared template SHA256: f99f214835f16f9bc15336399a7e9cf00be8fea045bef30ff50b5d379dadac41
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
