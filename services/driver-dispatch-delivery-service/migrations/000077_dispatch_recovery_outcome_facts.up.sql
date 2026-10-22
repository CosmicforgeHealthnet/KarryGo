-- Admin Section 6: Dispatch recovery outcomes for analytics, one fact per
-- recovery per outcome, written in the same transaction as the engine's own
-- state change. Dispatch has two recovery engines; their own states define
-- the outcome (nothing is inferred from timing or other tables):
--
-- Provider-replacement recovery (dispatch_delivery_incidents), one per
-- incident. Every incident is inserted REASSIGNING, i.e. recovery started.
--   started   incident insert                         dispatch.incident_recovery.started.v1
--   resolved  recovery_result -> READY (a replacement provider accepted;
--             the engine treats READY as final: it cannot be escalated)
--                                                     dispatch.incident_recovery.resolved.v1
--   failed    recovery_result -> MANUAL_REVIEW (deadline or attempts
--             exhausted, or administrative escalation), or the incident
--             handed to operations control (EMERGENCY_RECOVERY) while still
--             unresolved                              dispatch.incident_recovery.failed.v1
--   not counted: recovery_result -> CANCELLED. The engine sets it only when
--             the trip already ended (completed/cancelled/failed) or the
--             mission is no longer being reassigned: the recovery became moot
--             and the mission's own lifecycle fact records what happened.
--             Counting it as failed would also count a delivery the original
--             rider completed as a failed recovery.
--
-- Delivery recovery (dispatch_delivery_recovery_actions), one per recovery
-- action. Its start is the existing dispatch.delivery_recovery.selected.v1.
--   resolved  the action takes effect: state -> READY, at selection for an
--             immediate action or on activation of a scheduled retry or
--             reschedule (when the attempt is RESOLVED)
--                                                     dispatch.delivery_recovery.resolved.v1
--   failed    an OPERATIONS_REVIEW action taking effect: the recovery is
--             handed to operations                    dispatch.delivery_recovery.failed.v1
--   not counted: CANCELLED (the trip ended before a scheduled action took
--             effect) and COMPLETED (an action superseded by the customer's
--             new selection, which is its own recovery).
--
-- Idempotent: each fact is keyed by its recovery's own id at version 1, so
-- uq_dispatch_outbox_aggregate_event_version turns retries, replays and
-- concurrent writers into no-ops; READY and MANUAL_REVIEW are reached once.
-- Product follows the mission (commerce vs parcel), as in 000076.

CREATE OR REPLACE FUNCTION dispatch_analytics_recovery_fact(aggregate text, ref text, event text, payload jsonb) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO dispatch_marketplace_outbox
   (event_type, schema_version, aggregate_type, aggregate_id, aggregate_version,
    payload, payload_hash, target_service)
 VALUES (event, 1, 'analytics:' || aggregate, ref, 1, payload,
   encode(digest(payload::text, 'sha256'), 'hex'), 'analytics-service')
 ON CONFLICT DO NOTHING;
END $$;

CREATE OR REPLACE FUNCTION dispatch_incident_recovery_network_analytics() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE base jsonb; product text;
BEGIN
 SELECT dispatch_analytics_booking_product(t.booking_id) INTO product FROM trips t WHERE t.id = NEW.trip_id;
 base := jsonb_build_object('incident_id', NEW.id::text, 'trip_id', NEW.trip_id::text, 'reason_code', NEW.reason_code,
   'provider_id', NEW.provider_id::text, 'original_provider_id', NEW.provider_id::text,
   'product_code', coalesce(product, 'DISPATCH_PARCEL'));
 IF TG_OP = 'INSERT' THEN
  PERFORM dispatch_analytics_recovery_fact('delivery_incident', NEW.id::text, 'dispatch.incident_recovery.started.v1', base);
 ELSIF NEW.recovery_result = 'READY' AND OLD.recovery_result IS DISTINCT FROM 'READY' AND NEW.state = 'REASSIGNING' THEN
  PERFORM dispatch_analytics_recovery_fact('delivery_incident', NEW.id::text, 'dispatch.incident_recovery.resolved.v1',
    base || jsonb_build_object('replacement_provider_id', NEW.replacement_provider_id::text));
 ELSIF (NEW.recovery_result = 'MANUAL_REVIEW' AND OLD.recovery_result IN ('PENDING', 'UNKNOWN'))
    OR (NEW.state = 'EMERGENCY_RECOVERY' AND OLD.state IS DISTINCT FROM 'EMERGENCY_RECOVERY' AND NEW.recovery_result IN ('PENDING', 'UNKNOWN')) THEN
  PERFORM dispatch_analytics_recovery_fact('delivery_incident', NEW.id::text, 'dispatch.incident_recovery.failed.v1', base);
 END IF;
 RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS dispatch_incident_recovery_network_analytics ON dispatch_delivery_incidents;
CREATE TRIGGER dispatch_incident_recovery_network_analytics AFTER INSERT OR UPDATE OF recovery_result, state ON dispatch_delivery_incidents
 FOR EACH ROW EXECUTE FUNCTION dispatch_incident_recovery_network_analytics();

CREATE OR REPLACE FUNCTION dispatch_delivery_recovery_network_analytics() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE provider text;
BEGIN
 IF NEW.state <> 'READY' OR (TG_OP = 'UPDATE' AND OLD.state = 'READY') THEN
  RETURN NULL;
 END IF;
 SELECT t.provider_id::text INTO provider FROM trips t WHERE t.id = NEW.trip_id;
 PERFORM dispatch_analytics_recovery_fact('delivery_recovery_action', NEW.id::text,
   CASE WHEN NEW.action = 'OPERATIONS_REVIEW' THEN 'dispatch.delivery_recovery.failed.v1' ELSE 'dispatch.delivery_recovery.resolved.v1' END,
   jsonb_build_object('recovery_action_id', NEW.id::text, 'trip_id', NEW.trip_id::text, 'action', NEW.action,
     'provider_id', provider, 'product_code', dispatch_analytics_booking_product(NEW.booking_id)));
 RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS dispatch_delivery_recovery_network_analytics ON dispatch_delivery_recovery_actions;
CREATE TRIGGER dispatch_delivery_recovery_network_analytics AFTER INSERT OR UPDATE OF state ON dispatch_delivery_recovery_actions
 FOR EACH ROW EXECUTE FUNCTION dispatch_delivery_recovery_network_analytics();
