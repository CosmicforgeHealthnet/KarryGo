DROP TRIGGER IF EXISTS dispatch_booking_network_analytics ON dispatch_bookings;
DROP FUNCTION IF EXISTS dispatch_booking_network_analytics();
-- Restore 000063's function.
CREATE OR REPLACE FUNCTION dispatch_network_analytics_fanout() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.target_service <> 'analytics-service'
    AND NEW.aggregate_type IN ('trip','delivery_mission')
    AND NEW.event_type IN (
      'trip.completed.v1','trip.cancelled.v1','trip.delivery_failed.v1',
      'dispatch.delivery_recovery.selected.v1','dispatch.provider_replacement.started.v1',
      'dispatch.offer.declined.v1')
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
