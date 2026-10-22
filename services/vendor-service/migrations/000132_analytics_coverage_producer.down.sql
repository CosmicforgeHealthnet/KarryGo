-- Publication evidence may be removed only while empty.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM analytics_supply_roster_runs) OR EXISTS(SELECT 1 FROM analytics_coverage_publications)
 THEN RAISE EXCEPTION 'Analytics coverage producer evidence requires an approved forward retention migration'; END IF;
END $$;
DROP TABLE IF EXISTS analytics_coverage_publications;
DROP TABLE IF EXISTS analytics_supply_roster_members;
DROP TABLE IF EXISTS analytics_supply_roster_runs;
DROP FUNCTION IF EXISTS reject_analytics_supply_roster_mutation();
