-- Restore the exact fanout contract from migration 020.
CREATE OR REPLACE FUNCTION taxi_network_analytics_fanout() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.target NOT IN ('analytics-service','notification-service') AND NEW.event_type IN (
      'taxi.recovery.started','taxi.recovery.started.v1',
      'taxi.recovery.provider_replaced','taxi.recovery.provider_replaced.v1',
      'taxi.recovery.resolved','taxi.recovery.resolved.v1',
      'taxi.recovery.failed','taxi.recovery.failed.v1',
      'taxi.guarantee.breached','taxi.guarantee.breached.v1',
      'taxi.booking.arrived_at_pickup','taxi.goods.pickup.arrived.v1',
      'taxi.goods.completed.v1','taxi.goods.mission.cancelled.v1',
      'taxi.goods.mission.committed.v1','taxi.goods.assigned.v1',
      'taxi.booking.created','taxi.booking.accepted','taxi.booking.completed',
      'taxi.booking.auto_completed','taxi.booking.cancelled','taxi.booking.admin_cancelled',
      'taxi.booking.unmatched')
 THEN
  INSERT INTO taxi_outbox (aggregate_type, aggregate_id, event_type, payload, occurred_at, target, idempotency_key)
  VALUES (NEW.aggregate_type, NEW.aggregate_id, NEW.event_type, NEW.payload, NEW.occurred_at,
          'analytics-service', 'analytics:' || NEW.id::text)
  ON CONFLICT DO NOTHING;
 END IF;
 IF NEW.target NOT IN ('analytics-service','notification-service') AND NEW.event_type IN (
      'taxi.recovery.started','taxi.recovery.started.v1',
      'taxi.recovery.provider_replaced','taxi.recovery.provider_replaced.v1',
      'taxi.recovery.failed','taxi.recovery.failed.v1',
      'taxi.recovery.custody_transfer_required.v1',
      'taxi.guarantee.breached','taxi.guarantee.breached.v1')
 THEN
  INSERT INTO taxi_outbox (aggregate_type, aggregate_id, event_type, payload, occurred_at, target, idempotency_key)
  VALUES (NEW.aggregate_type, NEW.aggregate_id, NEW.event_type, NEW.payload, NEW.occurred_at,
          'notification-service', 'notify:' || NEW.id::text)
  ON CONFLICT DO NOTHING;
 END IF;
 RETURN NEW;
END $$;
