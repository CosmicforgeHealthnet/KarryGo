-- Admin Sections 6 and 9: a return-to-sender is part of the original customer
-- booking, not a second order.
--
-- deliveryrecovery executes a return through its own dispatch_bookings row
-- (the trip, wallet hold and broadcast need one), linked to the original by
-- dispatch_return_workflows (original_trip_id, recovery_action_id,
-- return_mission_id, return_booking_id) and by the custody manifest that moves
-- with the parcels. That execution record must not count as a new customer
-- order in analytics:
--   * no booking lifecycle facts (committed, assigned, unmatched, cancelled)
--     for a return execution booking;
--   * no mission lifecycle copies (trip.completed.v1, trip.delivery_failed.v1,
--     trip.cancelled.v1) for a return trip.
-- Recovery facts and provider activity on a return trip (arrivals, declines,
-- mismatches, incidents) are real and are kept. The original booking's own
-- facts already describe the order: committed, assigned, and the delivery
-- failure or recovery that led to the return.

CREATE OR REPLACE FUNCTION dispatch_is_return_execution(booking uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS (SELECT 1 FROM dispatch_return_workflows rw WHERE rw.return_booking_id = booking)
$$;

CREATE OR REPLACE FUNCTION dispatch_analytics_lifecycle_fact(booking uuid, event text, extra jsonb) RETURNS void LANGUAGE plpgsql AS $$
DECLARE s record; payload jsonb;
BEGIN
 IF dispatch_is_return_execution(booking) THEN
  RETURN;
 END IF;
 s := dispatch_analytics_subject(booking);
 payload := jsonb_build_object('booking_id', booking::text, 'product_code', s.product_code) || coalesce(extra, '{}'::jsonb);
 INSERT INTO dispatch_marketplace_outbox
   (event_type, schema_version, aggregate_type, aggregate_id, aggregate_version,
    payload, payload_hash, target_service)
 VALUES (event, 1, s.aggregate_type, s.aggregate_id, 1, payload,
   encode(digest(payload::text, 'sha256'), 'hex'), 'analytics-service')
 ON CONFLICT DO NOTHING;
END $$;

CREATE OR REPLACE FUNCTION dispatch_analytics_product_attribution() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE entity text; commerce boolean := false;
BEGIN
 IF NEW.target_service <> 'analytics-service' THEN
  RETURN NEW;
 END IF;
 entity := lower(regexp_replace(NEW.aggregate_type, '^analytics:', ''));
 IF entity = 'trip' AND NEW.event_type = 'trip.created.v1' THEN
  RETURN NULL;
 END IF;
 IF entity = 'trip' AND NEW.event_type IN ('trip.completed.v1', 'trip.delivery_failed.v1', 'trip.cancelled.v1')
   AND EXISTS (SELECT 1 FROM trips t WHERE t.id::text = NEW.aggregate_id AND dispatch_is_return_execution(t.booking_id)) THEN
  RETURN NULL;
 END IF;
 IF NEW.payload ? 'product_code' THEN
  RETURN NEW;
 END IF;
 IF entity = 'trip' THEN
  SELECT dispatch_analytics_booking_product(t.booking_id) = 'DISPATCH_COMMERCE' INTO commerce
    FROM trips t WHERE t.id::text = NEW.aggregate_id;
 ELSIF entity = 'delivery_mission' THEN
  SELECT m.source_type = 'VENDOR_COMMERCE'
      OR (m.booking_id IS NOT NULL AND dispatch_analytics_booking_product(m.booking_id) = 'DISPATCH_COMMERCE') INTO commerce
    FROM dispatch_delivery_missions m WHERE m.id::text = NEW.aggregate_id;
 END IF;
 IF coalesce(commerce, false) THEN
  NEW.payload := NEW.payload || jsonb_build_object('product_code', 'DISPATCH_COMMERCE');
  NEW.payload_hash := encode(digest(NEW.payload::text, 'sha256'), 'hex');
 END IF;
 RETURN NEW;
END $$;
