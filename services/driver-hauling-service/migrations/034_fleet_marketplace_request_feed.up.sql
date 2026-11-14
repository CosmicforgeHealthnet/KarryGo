-- Sections 11-12: Hauling publishes its open requests to Fleet as
-- marketplace opportunities (platform.request.offered.v1) and tells Fleet when
-- a request stops being open (taken by an independent provider, cancelled,
-- unmatched), on the EXISTING durable outbox (hauling_domain_outbox, target
-- 'fleet-events', delivered by fleetauthority.OutboxWorker to Fleet's
-- POST /internal/fleet-partner/v1/events).
--
-- Triggers, matching the analytics producer (024/031): every booking code
-- path that changes status -- customer create, matcher, accept, cancel,
-- unmatched sweep -- publishes in the SAME transaction without each of them
-- having to remember to.
--
-- Gated per connection by the custom setting hauling.fleet_marketplace_feed,
-- which the service sets only when HAULING_FLEET_MARKETPLACE_FEED_ENABLED is
-- on. With the setting absent nothing here writes and no version moves, so a
-- deployment that has not switched the feed on behaves byte-for-byte as
-- before.
--
-- Versioning: the request aggregate version is haulage_bookings.source_version
-- (already used by the reservation and trip contract). Each published change
-- advances it by exactly one -- unless the statement already advanced it
-- itself (the Fleet transition endpoint does), so it is never bumped twice.
--
-- The payload is an explicit allowlist: no address, contact, fare or payment
-- field ever leaves Hauling on this feed.

CREATE OR REPLACE FUNCTION hauling_fleet_marketplace_feed_enabled() RETURNS boolean
LANGUAGE sql STABLE AS $$
  SELECT coalesce(current_setting('hauling.fleet_marketplace_feed', true), '') = 'on'
$$;

CREATE OR REPLACE FUNCTION hauling_fleet_marketplace_feed_event(old_status text, new_status text,
  winner uuid) RETURNS text LANGUAGE sql IMMUTABLE AS $$
  SELECT CASE
    WHEN old_status IS NULL AND new_status = 'pending_match' THEN 'platform.request.offered.v1'
    WHEN old_status IS NULL THEN NULL
    WHEN old_status = new_status THEN NULL
    WHEN new_status = 'accepted' AND winner IS NULL THEN 'platform.request.updated.v1'
    WHEN new_status = 'cancelled' THEN 'platform.request.cancelled.v1'
    WHEN new_status = 'unmatched' THEN 'platform.request.expired.v1'
    ELSE NULL END
$$;

CREATE OR REPLACE FUNCTION hauling_fleet_marketplace_feed_version() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF NOT hauling_fleet_marketplace_feed_enabled() THEN
    RETURN NEW;
  END IF;
  IF hauling_fleet_marketplace_feed_event(OLD.status, NEW.status, NEW.winner_organization_id) IS NOT NULL
     AND NEW.source_version = OLD.source_version THEN
    NEW.source_version := OLD.source_version + 1;
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION hauling_fleet_marketplace_feed_publish() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  previous text;
  event text;
BEGIN
  IF NOT hauling_fleet_marketplace_feed_enabled() THEN
    RETURN NEW;
  END IF;
  IF TG_OP = 'UPDATE' THEN
    previous := OLD.status;
  END IF;
  event := hauling_fleet_marketplace_feed_event(previous, NEW.status, NEW.winner_organization_id);
  IF event IS NULL THEN
    RETURN NEW;
  END IF;
  INSERT INTO hauling_domain_outbox
    (aggregate_type, aggregate_id, event_type, event_version, payload, target)
  VALUES ('REQUEST', NEW.id, event, NEW.source_version,
    jsonb_build_object(
      'sourceService', 'driver-hauling-service',
      'requestId', NEW.id::text,
      'bookingId', NEW.id::text,
      'sourceVersion', NEW.source_version,
      'status', NEW.status,
      'serviceType', 'hauling',
      'weightKg', NEW.cargo_weight_kg,
      'helpersRequired', NEW.helper_count),
    'fleet-events')
  ON CONFLICT (aggregate_type, aggregate_id, event_version, event_type) DO NOTHING;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS hauling_fleet_marketplace_feed_version ON haulage_bookings;
CREATE TRIGGER hauling_fleet_marketplace_feed_version
  BEFORE UPDATE OF status ON haulage_bookings
  FOR EACH ROW EXECUTE FUNCTION hauling_fleet_marketplace_feed_version();

DROP TRIGGER IF EXISTS hauling_fleet_marketplace_feed_publish ON haulage_bookings;
CREATE TRIGGER hauling_fleet_marketplace_feed_publish
  AFTER INSERT OR UPDATE OF status ON haulage_bookings
  FOR EACH ROW EXECUTE FUNCTION hauling_fleet_marketplace_feed_publish();
