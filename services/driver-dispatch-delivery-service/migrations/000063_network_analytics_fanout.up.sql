-- Section 13: send the network facts Dispatch already records to analytics.
--
-- Trip outcomes go to Fleet and recovery facts to notification-service; none
-- of them reached analytics, so mission success and recovery performance for
-- bike delivery were invisible. Rather than teach every call site a second
-- target, the outbox copies each fact once more, addressed to analytics.
--
-- The copy's aggregate_type is prefixed so it does not collide with the
-- original on uq_dispatch_outbox_aggregate_event_version; the deliverer strips
-- the prefix. Only trip and mission aggregates are copied: the existing
-- vendor_delivery_projection fan-out re-emits the same recovery facts, and
-- copying those too would count every recovery twice.
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

DROP TRIGGER IF EXISTS dispatch_network_analytics_fanout ON dispatch_marketplace_outbox;
CREATE TRIGGER dispatch_network_analytics_fanout AFTER INSERT ON dispatch_marketplace_outbox
 FOR EACH ROW EXECUTE FUNCTION dispatch_network_analytics_fanout();
