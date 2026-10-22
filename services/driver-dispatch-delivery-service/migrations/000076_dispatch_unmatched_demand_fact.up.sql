-- Admin Section 6: Dispatch mission lifecycle facts for analytics, exactly
-- once per mission and kind, with the product taken from Dispatch's own rows.
--
-- The mission (the subject every lifecycle fact is recorded against) is:
--   * a Vendor commerce mission, for every booking it ever used (current
--     booking or any assignment attempt): analytics:COMMERCE_MISSION/<canonical_ref>;
--   * otherwise a customer booking: analytics:dispatch_booking/<booking id>;
--   * otherwise (no dispatch_bookings row) a partner-API request, identified by
--     its broadcast's booking id: analytics:partner_request/<booking id>.
--
-- Authoritative fact per kind (aggregate_version 1, so the outbox's
-- uq_dispatch_outbox_aggregate_event_version makes every repeat, retry,
-- reassignment or concurrent writer a no-op):
--   committed  customer booking insert (deferred to commit, so a commerce
--              booking is recognised and skipped: commerce.mission.created.v1
--              is that request); partner broadcast insert with no booking row.
--   assigned   the first trip row of the mission (every execution path, both
--              independent and organization-managed, inserts exactly one trip
--              per booking). Booking acceptance and trip.created.v1 no longer
--              reach analytics: they described the same assignment.
--   unmatched  the booking's CAS into 'unmatched'; for a partner request, its
--              broadcast ending no_provider_found/expired.
--   cancelled  a booking or partner request cancelled before any trip existed;
--              after that trip.cancelled.v1 (unchanged) is the fact.
--   completed / delivery failed: trip.completed.v1 / trip.delivery_failed.v1,
--              unchanged, one per trip.
-- Trip and delivery-mission copies of commerce work carry DISPATCH_COMMERCE.
-- Only analytical fields travel: never a coordinate, address, receiver or
-- customer.

CREATE OR REPLACE FUNCTION dispatch_analytics_subject(booking uuid, OUT aggregate_type text, OUT aggregate_id text, OUT product_code text) LANGUAGE plpgsql STABLE AS $$
BEGIN
 SELECT m.canonical_ref INTO aggregate_id FROM commerce_missions m
  WHERE m.booking_id = booking
     OR EXISTS (SELECT 1 FROM commerce_mission_assignment_attempts a WHERE a.commerce_mission_id = m.id AND a.booking_id = booking)
  ORDER BY m.created_at LIMIT 1;
 IF aggregate_id IS NOT NULL THEN
  aggregate_type := 'analytics:COMMERCE_MISSION'; product_code := 'DISPATCH_COMMERCE'; RETURN;
 END IF;
 aggregate_id := booking::text; product_code := 'DISPATCH_PARCEL';
 IF EXISTS (SELECT 1 FROM dispatch_bookings b WHERE b.id = booking) THEN
  aggregate_type := 'analytics:dispatch_booking';
 ELSE
  aggregate_type := 'analytics:partner_request';
 END IF;
END $$;

CREATE OR REPLACE FUNCTION dispatch_analytics_booking_product(booking uuid) RETURNS text LANGUAGE sql STABLE AS $$
 SELECT (dispatch_analytics_subject(booking)).product_code
$$;

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

-- 000064's booking function is superseded by the triggers below.
CREATE OR REPLACE FUNCTION dispatch_booking_network_analytics() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION dispatch_booking_committed_network_analytics() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (dispatch_analytics_subject(NEW.id)).aggregate_type = 'analytics:dispatch_booking' THEN
  PERFORM dispatch_analytics_lifecycle_fact(NEW.id, 'dispatch.booking.committed.v1', jsonb_build_object('service_tier', NEW.service_tier));
 END IF;
 RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS dispatch_booking_committed_network_analytics ON dispatch_bookings;
CREATE CONSTRAINT TRIGGER dispatch_booking_committed_network_analytics AFTER INSERT ON dispatch_bookings
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION dispatch_booking_committed_network_analytics();

CREATE OR REPLACE FUNCTION dispatch_booking_outcome_network_analytics() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status = 'unmatched' AND OLD.status IS DISTINCT FROM 'unmatched' THEN
  PERFORM dispatch_analytics_lifecycle_fact(NEW.id, 'dispatch.booking.unmatched.v1', '{}'::jsonb);
 ELSIF NEW.status = 'cancelled' AND OLD.status IS DISTINCT FROM 'cancelled'
   AND NOT EXISTS (SELECT 1 FROM trips t WHERE t.booking_id = NEW.id) THEN
  PERFORM dispatch_analytics_lifecycle_fact(NEW.id, 'dispatch.booking.cancelled.v1', '{}'::jsonb);
 END IF;
 RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS dispatch_booking_outcome_network_analytics ON dispatch_bookings;
CREATE TRIGGER dispatch_booking_outcome_network_analytics AFTER UPDATE OF status ON dispatch_bookings
 FOR EACH ROW EXECUTE FUNCTION dispatch_booking_outcome_network_analytics();

CREATE OR REPLACE FUNCTION dispatch_trip_assigned_network_analytics() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM dispatch_analytics_lifecycle_fact(NEW.booking_id, 'dispatch.booking.assigned.v1',
   jsonb_build_object('trip_id', NEW.id::text, 'provider_id', NEW.provider_id::text));
 RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS dispatch_trip_assigned_network_analytics ON trips;
CREATE TRIGGER dispatch_trip_assigned_network_analytics AFTER INSERT ON trips
 FOR EACH ROW EXECUTE FUNCTION dispatch_trip_assigned_network_analytics();

-- Partner-API requests have no dispatch_bookings row; their broadcast is
-- Dispatch's own record of the request.
CREATE OR REPLACE FUNCTION dispatch_partner_request_committed_network_analytics() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (dispatch_analytics_subject(NEW.booking_id)).aggregate_type = 'analytics:partner_request' THEN
  PERFORM dispatch_analytics_lifecycle_fact(NEW.booking_id, 'dispatch.booking.committed.v1', jsonb_build_object('service_tier', coalesce(NEW.booking_payload->>'service_tier', 'standard')));
 END IF;
 RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS dispatch_partner_request_committed_network_analytics ON request_broadcasts;
CREATE CONSTRAINT TRIGGER dispatch_partner_request_committed_network_analytics AFTER INSERT ON request_broadcasts
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION dispatch_partner_request_committed_network_analytics();

CREATE OR REPLACE FUNCTION dispatch_partner_request_outcome_network_analytics() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status IS NOT DISTINCT FROM OLD.status
   OR (dispatch_analytics_subject(NEW.booking_id)).aggregate_type <> 'analytics:partner_request' THEN
  RETURN NEW;
 END IF;
 IF NEW.status IN ('no_provider_found', 'expired') AND OLD.status NOT IN ('no_provider_found', 'expired') THEN
  PERFORM dispatch_analytics_lifecycle_fact(NEW.booking_id, 'dispatch.booking.unmatched.v1', '{}'::jsonb);
 ELSIF NEW.status = 'cancelled' AND NOT EXISTS (SELECT 1 FROM trips t WHERE t.booking_id = NEW.booking_id) THEN
  PERFORM dispatch_analytics_lifecycle_fact(NEW.booking_id, 'dispatch.booking.cancelled.v1', '{}'::jsonb);
 END IF;
 RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS dispatch_partner_request_outcome_network_analytics ON request_broadcasts;
CREATE TRIGGER dispatch_partner_request_outcome_network_analytics AFTER UPDATE OF status ON request_broadcasts
 FOR EACH ROW EXECUTE FUNCTION dispatch_partner_request_outcome_network_analytics();

-- Analytics copies: trip.created.v1 duplicates the trip-row assignment fact
-- and is not copied; commerce trip and delivery-mission facts carry the
-- commerce product. Parcel facts are otherwise byte-for-byte unchanged.
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

DROP TRIGGER IF EXISTS dispatch_analytics_product_attribution ON dispatch_marketplace_outbox;
CREATE TRIGGER dispatch_analytics_product_attribution BEFORE INSERT ON dispatch_marketplace_outbox
 FOR EACH ROW EXECUTE FUNCTION dispatch_analytics_product_attribution();
