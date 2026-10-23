-- Restores the 000076 definitions; the return-execution exclusion is removed.
CREATE OR REPLACE FUNCTION dispatch_analytics_lifecycle_fact(booking uuid, event text, extra jsonb) RETURNS void LANGUAGE plpgsql AS $$
DECLARE s record; payload jsonb;
BEGIN
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

DROP FUNCTION IF EXISTS dispatch_is_return_execution(uuid);
