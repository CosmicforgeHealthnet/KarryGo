-- Already-recorded facts stay in the outbox: they describe outcomes that
-- happened and may already have been delivered. Restores 000064's booking
-- function (committed on insert, assigned on acceptance).
DROP TRIGGER IF EXISTS dispatch_analytics_product_attribution ON dispatch_marketplace_outbox;
DROP FUNCTION IF EXISTS dispatch_analytics_product_attribution();
DROP TRIGGER IF EXISTS dispatch_partner_request_outcome_network_analytics ON request_broadcasts;
DROP FUNCTION IF EXISTS dispatch_partner_request_outcome_network_analytics();
DROP TRIGGER IF EXISTS dispatch_partner_request_committed_network_analytics ON request_broadcasts;
DROP FUNCTION IF EXISTS dispatch_partner_request_committed_network_analytics();
DROP TRIGGER IF EXISTS dispatch_trip_assigned_network_analytics ON trips;
DROP FUNCTION IF EXISTS dispatch_trip_assigned_network_analytics();
DROP TRIGGER IF EXISTS dispatch_booking_outcome_network_analytics ON dispatch_bookings;
DROP FUNCTION IF EXISTS dispatch_booking_outcome_network_analytics();
DROP TRIGGER IF EXISTS dispatch_booking_committed_network_analytics ON dispatch_bookings;
DROP FUNCTION IF EXISTS dispatch_booking_committed_network_analytics();

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

DROP FUNCTION IF EXISTS dispatch_analytics_lifecycle_fact(uuid, text, jsonb);
DROP FUNCTION IF EXISTS dispatch_analytics_booking_product(uuid);
DROP FUNCTION IF EXISTS dispatch_analytics_subject(uuid);
