-- Admin Section 6: owner-signed Analytics coverage manifests and supply roster.
-- shared-template-sha256: ee2345d63bbe50fe9533f1857a89e22f71f698b48cb82cc15f1deb7452249dd0
-- The block below is shared/go/analyticsadmin/producer_schema.sql verbatim.
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
-- Owner-owned producer contract: the haulage outcome facts (assigned,
-- completed, cancelled, unmatched) now name the provider that held the booking
-- (when one did); the committed/demand fact stays provider-free, so provider-level Analytics
-- reports can attribute each fact to a verified roster member. No customer,
-- address, coordinate or money is added. Unchanged rows keep their payloads.
CREATE OR REPLACE FUNCTION hauling_booking_network_analytics() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE event text;
BEGIN
 IF TG_OP = 'INSERT' THEN
  event := 'hauling.booking.committed.v1';
 ELSIF NEW.status IS DISTINCT FROM OLD.status THEN
  event := CASE NEW.status
    WHEN 'accepted'  THEN 'hauling.booking.assigned.v1'
    WHEN 'completed' THEN 'hauling.booking.completed.v1'
    WHEN 'cancelled' THEN 'hauling.booking.cancelled.v1'
    WHEN 'unmatched' THEN 'hauling.booking.unmatched.v1'
    ELSE NULL END;
 END IF;
 IF event IS NULL THEN
  RETURN NEW;
 END IF;
 INSERT INTO hauling_domain_outbox (aggregate_type, aggregate_id, event_type, event_version, payload, target)
 VALUES ('haulage_booking', NEW.id, event, 1,
   jsonb_strip_nulls(jsonb_build_object('booking_id', NEW.id::text, 'product_code', 'HAULAGE',
     'provider_id', CASE WHEN TG_OP = 'INSERT' THEN NULL ELSE NEW.provider_id::text END)), 'analytics-service')
 ON CONFLICT (aggregate_type, aggregate_id, event_version, event_type) DO NOTHING;
 RETURN NEW;
END $$;
