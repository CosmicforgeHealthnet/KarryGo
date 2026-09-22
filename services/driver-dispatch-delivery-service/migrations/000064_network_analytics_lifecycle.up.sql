-- Section 13 closure.
--
-- 1. Fix 000063: the trip lifecycle is emitted with aggregate_type 'TRIP'
--    (and commerce missions with 'COMMERCE_MISSION'), but 000063 matched
--    lower-case 'trip' only, so real trip outcomes were never copied. Match
--    case-insensitively.
-- 2. Customer bookings emitted nothing when committed or assigned. Record
--    both, from the booking row itself, for analytics only.
CREATE OR REPLACE FUNCTION dispatch_network_analytics_fanout() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.target_service <> 'analytics-service'
    AND lower(NEW.aggregate_type) IN ('trip','delivery_mission','commerce_mission')
    AND NEW.event_type IN (
      'trip.created.v1','trip.completed.v1','trip.cancelled.v1','trip.delivery_failed.v1',
      'dispatch.delivery_recovery.selected.v1','dispatch.provider_replacement.started.v1',
      'dispatch.offer.declined.v1','commerce.mission.created.v1')
 THEN
  INSERT INTO dispatch_marketplace_outbox
    (event_type, schema_version, aggregate_type, aggregate_id, aggregate_version,
     payload, payload_hash, correlation_id, causation_id, target_service)
  VALUES
    (NEW.event_type, NEW.schema_version, 'analytics:' || NEW.aggregate_type, NEW.aggregate_id,
     NEW.aggregate_version, NEW.payload, NEW.payload_hash, NEW.correlation_id, NEW.causation_id,
     'analytics-service')
  ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;

-- Committed on insert; assigned on the move to 'accepted'. Only analytical
-- fields travel: the booking id and product, never a coordinate or customer.
CREATE OR REPLACE FUNCTION dispatch_booking_network_analytics() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE event text; payload jsonb;
BEGIN
 IF TG_OP = 'INSERT' THEN
  event := 'dispatch.booking.committed.v1';
 ELSIF NEW.status = 'accepted' AND OLD.status IS DISTINCT FROM 'accepted' THEN
  event := 'dispatch.booking.assigned.v1';
 ELSE
  RETURN NEW;
 END IF;
 payload := jsonb_build_object('booking_id', NEW.id::text, 'service_tier', NEW.service_tier,
   'product_code', 'DISPATCH_PARCEL');
 INSERT INTO dispatch_marketplace_outbox
   (event_type, schema_version, aggregate_type, aggregate_id, aggregate_version,
    payload, payload_hash, target_service)
 VALUES (event, 1, 'analytics:dispatch_booking', NEW.id::text, 1, payload,
   encode(digest(payload::text, 'sha256'), 'hex'), 'analytics-service')
 ON CONFLICT DO NOTHING;
 RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS dispatch_booking_network_analytics ON dispatch_bookings;
CREATE TRIGGER dispatch_booking_network_analytics AFTER INSERT OR UPDATE OF status ON dispatch_bookings
 FOR EACH ROW EXECUTE FUNCTION dispatch_booking_network_analytics();
