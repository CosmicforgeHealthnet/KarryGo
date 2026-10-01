-- Section 13 closure: Haulage booking outcomes for analytics.
--
-- Haulage bookings recorded their lifecycle only in their own status, so
-- mission success could not see haulage at all. This records each outcome,
-- once, from the booking row itself, into the outbox analytics already reads.
-- Only the booking id and product travel: no customer, provider, address or
-- coordinate.
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
 -- A booking reaches each outcome once; the event name is the version's
 -- partner in the outbox key, so version 1 per outcome is exact.
 INSERT INTO hauling_domain_outbox (aggregate_type, aggregate_id, event_type, event_version, payload, target)
 VALUES ('haulage_booking', NEW.id, event, 1,
   jsonb_build_object('booking_id', NEW.id::text, 'product_code', 'HAULAGE'), 'analytics-service')
 ON CONFLICT (aggregate_type, aggregate_id, event_version, event_type) DO NOTHING;
 RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS hauling_booking_network_analytics ON haulage_bookings;
CREATE TRIGGER hauling_booking_network_analytics AFTER INSERT OR UPDATE OF status ON haulage_bookings
 FOR EACH ROW EXECUTE FUNCTION hauling_booking_network_analytics();
