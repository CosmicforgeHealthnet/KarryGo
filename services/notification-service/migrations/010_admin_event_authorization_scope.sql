-- Signed source-owned administrative allocations; no business records or grants are seeded.
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

-- Authorization scope comes only from explicit owner allocations, not native geography.
-- SQL selects a candidate; the Go egress guard MUST verify its signature, expiry,
-- current head, canonical record binding and signed feed purpose before disclosure.
-- Metadata corruption or ambiguity must not break a business transaction.
SELECT pg_advisory_xact_lock(hashtextextended('notification-service:migration:010', 0));
CREATE OR REPLACE FUNCTION notification_admin_event_scope(p_ref text) RETURNS jsonb
LANGUAGE plpgsql STABLE AS $fn$
DECLARE candidate jsonb; candidates bigint; native_market text := ''; native_zone text := '';
BEGIN
    SELECT CASE WHEN jsonb_array_length(intent->'markets')=1 THEN intent->'markets'->>0 ELSE '' END,
           CASE WHEN jsonb_typeof(intent->'zones')='array' THEN CASE WHEN jsonb_array_length(intent->'zones')=1 THEN intent->'zones'->>0 ELSE '' END ELSE '' END
      INTO native_market,native_zone FROM campaigns WHERE campaign_id::text||':'||revision::text=p_ref;
    WITH scopes AS (
        SELECT DISTINCT jsonb_build_object('market',h.market,'zone',h.zone,
            'organization',h.organization_ref,'tenant',h.tenant_ref) AS value
        FROM admin_owner_context_heads h JOIN admin_owner_context_history v
          ON v.owner=h.owner AND v.context_id=h.context_id AND v.revision=h.revision AND v.checksum=h.checksum
        WHERE h.owner='notification-service' AND h.resource_kind='campaign-revisions' AND h.resource_ref=p_ref
          AND h.product='karryam' AND 'hq.campaigns.readback'=ANY(h.capabilities)
          AND h.revision=(SELECT max(x.revision) FROM admin_owner_context_history x WHERE x.owner=h.owner AND x.context_id=h.context_id)
          AND (convert_from(v.envelope,'UTF8')::jsonb->'document'->>'state')='APPROVED'
          AND (convert_from(v.envelope,'UTF8')::jsonb->'document'->>'effective_at')::timestamptz<=now()
          AND (convert_from(v.envelope,'UTF8')::jsonb->'document'->>'expires_at')::timestamptz>now()
    ) SELECT count(*),min(value::text)::jsonb INTO candidates,candidate FROM scopes;
    IF candidates=1 THEN RETURN candidate; END IF;
    RETURN jsonb_build_object('market',COALESCE(native_market,''),'zone',COALESCE(native_zone,''),'organization','','tenant','');
EXCEPTION WHEN OTHERS THEN
    RETURN jsonb_build_object('market',COALESCE(native_market,''),'zone',COALESCE(native_zone,''),'organization','','tenant','');
END $fn$;
