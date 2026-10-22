-- Owner-side Analytics coverage producer state (shared/go/analyticsadmin).
-- Installed verbatim by each producing owner's normal migration runner.
-- analytics_supply_roster_runs/members: the owner's authoritative, append-only
-- record of who was in its supply roster at each roster bucket. A run row is
-- written only when the owner actually observed its availability records for
-- that bucket, so a missing run is an unobserved bucket, never an empty one.
-- analytics_coverage_publications: the producer's last signed manifest per
-- window/scope/family, kept so an unknown publication outcome is retried with
-- the identical signed body (idempotent) rather than a new revision.
CREATE TABLE IF NOT EXISTS analytics_supply_roster_runs (
 bucket_start timestamptz PRIMARY KEY,
 member_count integer NOT NULL CHECK (member_count >= 0),
 policy_version text NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS analytics_supply_roster_members (
 bucket_start timestamptz NOT NULL REFERENCES analytics_supply_roster_runs(bucket_start),
 provider_id uuid NOT NULL,
 product_code text NOT NULL,
 eligible boolean,
 occupied boolean,
 event_id text NOT NULL UNIQUE,
 PRIMARY KEY (bucket_start, provider_id, product_code)
);
CREATE OR REPLACE FUNCTION reject_analytics_supply_roster_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Analytics supply roster evidence is append-only'; END $$;
DROP TRIGGER IF EXISTS analytics_supply_roster_runs_immutable ON analytics_supply_roster_runs;
CREATE TRIGGER analytics_supply_roster_runs_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON analytics_supply_roster_runs
 FOR EACH STATEMENT EXECUTE FUNCTION reject_analytics_supply_roster_mutation();
DROP TRIGGER IF EXISTS analytics_supply_roster_members_immutable ON analytics_supply_roster_members;
CREATE TRIGGER analytics_supply_roster_members_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON analytics_supply_roster_members
 FOR EACH STATEMENT EXECUTE FUNCTION reject_analytics_supply_roster_mutation();
CREATE TABLE IF NOT EXISTS analytics_coverage_publications (
 context_id text PRIMARY KEY,
 family text NOT NULL,
 window_start timestamptz NOT NULL,
 window_end timestamptz NOT NULL CHECK (window_end > window_start),
 digest text NOT NULL CHECK (digest ~ '^[0-9a-f]{64}$'),
 member_count integer NOT NULL CHECK (member_count >= 0),
 revision bigint NOT NULL CHECK (revision > 0),
 body bytea NOT NULL,
 state text NOT NULL CHECK (state IN ('PENDING','PUBLISHED')),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS analytics_coverage_publications_window ON analytics_coverage_publications (window_start);
