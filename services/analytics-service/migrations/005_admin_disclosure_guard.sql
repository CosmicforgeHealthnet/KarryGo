-- Governance state and disclosure evidence belong to Analytics. There is no
-- source-domain backfill or administrative mutation in this migration.
CREATE TABLE IF NOT EXISTS analytics_admin_policy_revisions (
 policy_id text PRIMARY KEY, version bigint NOT NULL CHECK(version>0),
 checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),
 accepted_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS analytics_admin_query_budgets (
 staff_hash text NOT NULL, window_start timestamptz NOT NULL,
 query_count integer NOT NULL CHECK(query_count>0),
 PRIMARY KEY(staff_hash,window_start)
);
CREATE TABLE IF NOT EXISTS analytics_admin_disclosures (
 release_id text PRIMARY KEY, query_hash text NOT NULL,
 policy_checksum text NOT NULL, released_payload jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS analytics_admin_disclosure_partitions (
 partition_hash text PRIMARY KEY,
 release_id text NOT NULL REFERENCES analytics_admin_disclosures(release_id),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS analytics_admin_query_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), staff_ref text NOT NULL,
 policy_id text NOT NULL, policy_version bigint NOT NULL,
 query_hash text NOT NULL, outcome text NOT NULL,
 purpose_code text NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS analytics_admin_query_audit_time ON analytics_admin_query_audit(recorded_at);
CREATE OR REPLACE FUNCTION reject_analytics_admin_evidence_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Analytics disclosure evidence is append-only'; END $$;
DROP TRIGGER IF EXISTS analytics_admin_disclosures_immutable ON analytics_admin_disclosures;
CREATE TRIGGER analytics_admin_disclosures_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON analytics_admin_disclosures FOR EACH STATEMENT EXECUTE FUNCTION reject_analytics_admin_evidence_mutation();
DROP TRIGGER IF EXISTS analytics_admin_partitions_immutable ON analytics_admin_disclosure_partitions;
CREATE TRIGGER analytics_admin_partitions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON analytics_admin_disclosure_partitions FOR EACH STATEMENT EXECUTE FUNCTION reject_analytics_admin_evidence_mutation();
DROP TRIGGER IF EXISTS analytics_admin_query_audit_immutable ON analytics_admin_query_audit;
CREATE TRIGGER analytics_admin_query_audit_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON analytics_admin_query_audit FOR EACH STATEMENT EXECUTE FUNCTION reject_analytics_admin_evidence_mutation();
