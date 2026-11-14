-- Sections 11-12: Taxi tells Fleet about (a) its open ride requests, as
-- marketplace opportunities, and (b) the lifecycle of the canonical trips it
-- runs for organizations, so Fleet can move the assignment to IN_PROGRESS and
-- COMPLETED. Before this, Taxi sent Fleet only completed-work.
--
-- Rows go onto the EXISTING durable outbox (taxi_outbox) under a new target,
-- 'fleet-marketplace', delivered by the existing publisher through a
-- FleetMarketplaceEventDestination to Fleet's POST
-- /internal/fleet-partner/v1/events. Same leasing, backoff and dead-lettering
-- as every other taxi outbox target.
--
-- Triggers, so every booking path that changes status (customer create,
-- matcher, accept, ride lifecycle, cancel, unmatched sweep, Fleet binding)
-- publishes in the SAME transaction.
--
-- Gated per connection by the custom setting taxi.fleet_marketplace_feed,
-- which the service sets only when TAXI_FLEET_MARKETPLACE_FEED_ENABLED is on
-- and the Fleet integration is not killed. With it absent nothing here writes
-- and no version moves.
--
-- Versioning: taxi_bookings.source_version is the aggregate version (already
-- used by the managed-execution reservation contract). Each published change
-- advances it by exactly one, unless the statement itself already did (the
-- Fleet binding does), so it is never advanced twice.
--
-- Payloads are explicit allowlists: no address, contact, fare or payment
-- field ever reaches Fleet on this feed.

CREATE OR REPLACE FUNCTION taxi_fleet_marketplace_feed_enabled() RETURNS boolean
LANGUAGE sql STABLE AS $$
  SELECT coalesce(current_setting('taxi.fleet_marketplace_feed', true), '') = 'on'
$$;

-- taxi_fleet_marketplace_feed_event names the event a change publishes, or
-- NULL. Organization-run trips (bound to a Fleet marketplace assignment)
-- publish TRIP events; open requests publish REQUEST events. Private
-- executions are Fleet's own private orders, projected elsewhere, and are not
-- part of this feed.
CREATE OR REPLACE FUNCTION taxi_fleet_marketplace_feed_event(
  old_status text, new_status text, old_managed boolean, new_managed boolean,
  source_context text) RETURNS text LANGUAGE sql IMMUTABLE AS $$
  SELECT CASE
    WHEN source_context = 'FLEET_PRIVATE_EXECUTION' THEN NULL
    -- A request is opened.
    WHEN old_status IS NULL AND new_status = 'pending_match' AND NOT new_managed
      THEN 'platform.request.offered.v1'
    WHEN old_status IS NULL THEN NULL
    -- An organization's provider was bound to it: the canonical trip exists.
    WHEN new_managed AND NOT old_managed THEN 'trip.created.v1'
    WHEN old_status = new_status THEN NULL
    -- Organization-run trip lifecycle.
    WHEN new_managed AND new_status = 'completed' THEN 'trip.completed.v1'
    WHEN new_managed AND new_status IN ('cancelled', 'unmatched') THEN 'trip.cancelled.v1'
    WHEN new_managed THEN 'trip.status_changed.v1'
    -- Open request outcomes.
    WHEN new_status = 'accepted' THEN 'platform.request.updated.v1'
    WHEN new_status = 'cancelled' THEN 'platform.request.cancelled.v1'
    WHEN new_status = 'unmatched' THEN 'platform.request.expired.v1'
    ELSE NULL END
$$;

CREATE OR REPLACE FUNCTION taxi_fleet_marketplace_feed_version() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF NOT taxi_fleet_marketplace_feed_enabled() THEN
    RETURN NEW;
  END IF;
  IF taxi_fleet_marketplace_feed_event(OLD.status, NEW.status, OLD.is_corporate_managed,
       NEW.is_corporate_managed, NEW.source_context) IS NOT NULL
     AND NEW.source_version = OLD.source_version THEN
    NEW.source_version := OLD.source_version + 1;
  END IF;
  RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION taxi_fleet_marketplace_feed_publish() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  previous_status text;
  previous_managed boolean := false;
  event text;
  aggregate text := 'REQUEST';
  body jsonb;
BEGIN
  IF NOT taxi_fleet_marketplace_feed_enabled() THEN
    RETURN NEW;
  END IF;
  IF TG_OP = 'UPDATE' THEN
    previous_status := OLD.status;
    previous_managed := OLD.is_corporate_managed;
  END IF;
  event := taxi_fleet_marketplace_feed_event(previous_status, NEW.status, previous_managed,
    NEW.is_corporate_managed, NEW.source_context);
  IF event IS NULL THEN
    RETURN NEW;
  END IF;
  body := jsonb_build_object(
    'sourceService', 'driver-taxi-service',
    'requestId', NEW.id::text,
    'bookingId', NEW.id::text,
    'sourceVersion', NEW.source_version,
    'status', NEW.status,
    'serviceType', 'taxi',
    'vehicleClass', NEW.vehicle_class,
    'passengerCount', NEW.passenger_count);
  IF event LIKE 'trip.%' THEN
    aggregate := 'TRIP';
    body := body || jsonb_build_object(
      'tripId', NEW.id::text,
      'fleetAssignmentId', NEW.fleet_assignment_id,
      'executingProviderId', NEW.provider_id::text,
      'organizationId', NEW.organization_id,
      'settlementOrganizationId', NEW.organization_id);
    IF NEW.completed_at IS NOT NULL THEN
      body := body || jsonb_build_object('completedAt',
        to_char(NEW.completed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'));
    END IF;
    IF NEW.cancelled_at IS NOT NULL THEN
      body := body || jsonb_build_object('cancelledAt',
        to_char(NEW.cancelled_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'));
    END IF;
  END IF;
  INSERT INTO taxi_outbox (aggregate_type, aggregate_id, event_type, payload, target, idempotency_key)
  VALUES (aggregate, NEW.id, event, jsonb_strip_nulls(body), 'fleet-marketplace',
    event || ':' || NEW.id::text || ':' || NEW.source_version::text)
  ON CONFLICT (target, idempotency_key) WHERE idempotency_key <> '' DO NOTHING;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS taxi_fleet_marketplace_feed_version ON taxi_bookings;
CREATE TRIGGER taxi_fleet_marketplace_feed_version
  BEFORE UPDATE ON taxi_bookings
  FOR EACH ROW EXECUTE FUNCTION taxi_fleet_marketplace_feed_version();

DROP TRIGGER IF EXISTS taxi_fleet_marketplace_feed_publish ON taxi_bookings;
CREATE TRIGGER taxi_fleet_marketplace_feed_publish
  AFTER INSERT OR UPDATE ON taxi_bookings
  FOR EACH ROW EXECUTE FUNCTION taxi_fleet_marketplace_feed_publish();
