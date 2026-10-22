-- Owner-signed coverage/membership manifests (shared/go/analyticsadmin).
-- A manifest is an independently signed source-owner context approval whose
-- binding fingerprint is a digest over the exact member facts the owner
-- published for one family, five-dimension scope and window. Heads hold the
-- current monotonic revision; history and members are immutable evidence.
-- Nothing here grants HQ access or declares coverage by itself: every report
-- re-verifies signature, digest and retained native facts before release.
CREATE TABLE IF NOT EXISTS analytics_admin_coverage_heads (
 owner text NOT NULL, context_id text NOT NULL,
 revision bigint NOT NULL CHECK (revision > 0),
 checksum text NOT NULL CHECK (checksum ~ '^[0-9a-f]{64}$'),
 family text NOT NULL CHECK (family IN ('network-events','supply-roster','network-events-global','supply-roster-global')),
 product text NOT NULL, market text NOT NULL, zone text NOT NULL, organization_ref text NOT NULL, tenant_ref text NOT NULL,
 window_start timestamptz NOT NULL, window_end timestamptz NOT NULL CHECK (window_end > window_start),
 digest text NOT NULL CHECK (digest ~ '^[0-9a-f]{64}$'),
 member_count integer NOT NULL CHECK (member_count >= 0),
 state text NOT NULL CHECK (state IN ('APPROVED','REVOKED')),
 recorded_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (owner, context_id)
);
CREATE INDEX IF NOT EXISTS analytics_admin_coverage_scope
 ON analytics_admin_coverage_heads (owner, family, product, market, zone, organization_ref, tenant_ref, window_start);

CREATE TABLE IF NOT EXISTS analytics_admin_coverage_history (
 owner text NOT NULL, context_id text NOT NULL, revision bigint NOT NULL CHECK (revision > 0),
 checksum text NOT NULL CHECK (checksum ~ '^[0-9a-f]{64}$'),
 envelope bytea NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (owner, context_id, revision)
);

CREATE TABLE IF NOT EXISTS analytics_admin_coverage_members (
 owner text NOT NULL, context_id text NOT NULL, revision bigint NOT NULL,
 event_id text NOT NULL, event_name text NOT NULL, entity_type text NOT NULL, entity_id text NOT NULL,
 source_version bigint NOT NULL CHECK (source_version >= 1), occurred_at timestamptz NOT NULL,
 PRIMARY KEY (owner, context_id, revision, event_id),
 FOREIGN KEY (owner, context_id, revision) REFERENCES analytics_admin_coverage_history (owner, context_id, revision)
);
CREATE INDEX IF NOT EXISTS analytics_admin_coverage_member_event ON analytics_admin_coverage_members (event_id);

CREATE OR REPLACE FUNCTION reject_analytics_admin_coverage_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Analytics coverage manifest evidence is append-only'; END $$;
DROP TRIGGER IF EXISTS analytics_admin_coverage_history_immutable ON analytics_admin_coverage_history;
CREATE TRIGGER analytics_admin_coverage_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON analytics_admin_coverage_history
 FOR EACH STATEMENT EXECUTE FUNCTION reject_analytics_admin_coverage_mutation();
DROP TRIGGER IF EXISTS analytics_admin_coverage_members_immutable ON analytics_admin_coverage_members;
CREATE TRIGGER analytics_admin_coverage_members_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON analytics_admin_coverage_members
 FOR EACH STATEMENT EXECUTE FUNCTION reject_analytics_admin_coverage_mutation();

-- Durable publication nonces for manifest publication (replay protection).
CREATE TABLE IF NOT EXISTS analytics_admin_coverage_nonces (
 owner text NOT NULL, nonce_hash text NOT NULL, expires_at timestamptz NOT NULL,
 PRIMARY KEY (owner, nonce_hash)
);
CREATE INDEX IF NOT EXISTS analytics_admin_coverage_nonce_expiry ON analytics_admin_coverage_nonces (expires_at);
