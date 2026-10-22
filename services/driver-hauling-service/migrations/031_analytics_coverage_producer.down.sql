-- Roster and publication evidence may be removed only while empty.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM analytics_supply_roster_runs) OR EXISTS(SELECT 1 FROM analytics_coverage_publications)
 THEN RAISE EXCEPTION 'Analytics coverage producer evidence requires an approved forward retention migration'; END IF;
END $$;
DROP TABLE IF EXISTS analytics_coverage_publications;
DROP TABLE IF EXISTS analytics_supply_roster_members;
DROP TABLE IF EXISTS analytics_supply_roster_runs;
DROP FUNCTION IF EXISTS reject_analytics_supply_roster_mutation();
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
   jsonb_build_object('booking_id', NEW.id::text, 'product_code', 'HAULAGE'), 'analytics-service')
 ON CONFLICT (aggregate_type, aggregate_id, event_version, event_type) DO NOTHING;
 RETURN NEW;
END $$;
