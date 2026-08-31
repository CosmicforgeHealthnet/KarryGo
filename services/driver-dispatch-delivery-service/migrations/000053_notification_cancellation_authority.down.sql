-- Restore the exact pre-correction function for a reversible migration.
-- WARNING: this also restores migration 51's known cancellation-column defect.
-- Roll back only for controlled migration testing/maintenance; reapply before
-- serving canonical Booking/Trip traffic. No business or outbox rows are removed.
DROP TRIGGER IF EXISTS dispatch_cancellation_notification_intent ON cancellations;
CREATE OR REPLACE FUNCTION dispatch_booking_notification_intent() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE kind text; booking_ref uuid; customer_ref uuid; provider_ref uuid; recipient text; payload jsonb;
BEGIN
 IF TG_OP='UPDATE' AND NEW.status IS NOT DISTINCT FROM OLD.status THEN RETURN NEW; END IF;
 IF TG_TABLE_NAME='dispatch_bookings' THEN
  booking_ref:=NEW.id; customer_ref:=NEW.customer_id; provider_ref:=NEW.provider_id;
  kind:=CASE NEW.status WHEN 'accepted' THEN 'delivery.accepted' WHEN 'unmatched' THEN 'delivery.unmatched'
   WHEN 'completed' THEN 'delivery.completed' WHEN 'cancelled' THEN 'delivery.cancelled' END;
  IF kind='delivery.cancelled' AND EXISTS(SELECT 1 FROM trips t WHERE t.booking_id=NEW.id AND t.status='cancelled' AND t.cancelled_by='provider') THEN RETURN NEW; END IF;
 ELSE
  booking_ref:=NEW.booking_id; customer_ref:=NEW.customer_id; provider_ref:=NEW.provider_id;
  kind:=CASE NEW.status WHEN 'assigned' THEN 'delivery.accepted' WHEN 'en_route_pickup' THEN 'delivery.en_route_pickup'
   WHEN 'arrived_pickup' THEN 'delivery.arrived_at_pickup' WHEN 'in_progress' THEN 'package.picked_up'
   WHEN 'proof_submitted' THEN 'package.delivered' WHEN 'completed' THEN 'delivery.completed'
   WHEN 'cancelled' THEN CASE WHEN NEW.cancelled_by='provider' THEN 'delivery.cancelled_by_provider' ELSE 'delivery.cancelled' END END;
 END IF;
 IF kind IS NULL THEN RETURN NEW; END IF;
 recipient:=customer_ref::text;
 IF kind='delivery.cancelled' THEN recipient:=provider_ref::text; END IF;
 IF recipient IS NULL THEN RETURN NEW; END IF;
 payload:=jsonb_build_object('booking_id',booking_ref::text,'recipient_id',recipient,'recipient_type',CASE WHEN kind='delivery.cancelled' THEN 'provider' ELSE 'customer' END);
 INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service)
 VALUES(kind,'booking_notification',booking_ref::text||':'||recipient,1,payload,encode(digest(payload::text,'sha256'),'hex'),'notification-service') ON CONFLICT DO NOTHING;
 IF kind='delivery.arrived_at_pickup' THEN
  INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service)
  VALUES('delivery.pickup_verification_required','booking_notification',booking_ref::text||':'||recipient,1,payload,encode(digest(payload::text,'sha256'),'hex'),'notification-service') ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;

