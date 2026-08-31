-- Additive closure only; reuse canonical outbox and domain rows.
CREATE FUNCTION dispatch_booking_notification_intent() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER dispatch_booking_notification_intent AFTER INSERT OR UPDATE OF status ON dispatch_bookings FOR EACH ROW EXECUTE FUNCTION dispatch_booking_notification_intent();
CREATE TRIGGER dispatch_trip_notification_intent AFTER INSERT OR UPDATE OF status ON trips FOR EACH ROW EXECUTE FUNCTION dispatch_booking_notification_intent();

ALTER TABLE dispatch_delivery_incidents
 ADD COLUMN recovery_attempts integer NOT NULL DEFAULT 0 CHECK(recovery_attempts>=0),
 ADD COLUMN recovery_next_at timestamptz NOT NULL DEFAULT now(),
 ADD COLUMN recovery_deadline timestamptz NOT NULL DEFAULT (now()+interval '30 minutes'),
 ADD COLUMN recovery_lease_owner text,
 ADD COLUMN recovery_lease_expires_at timestamptz,
 ADD COLUMN recovery_result text NOT NULL DEFAULT 'PENDING' CHECK(recovery_result IN ('PENDING','UNKNOWN','READY','MANUAL_REVIEW','CANCELLED')),
 ADD COLUMN replacement_provider_id uuid REFERENCES providers(id),
 ADD COLUMN recovery_error_code text NOT NULL DEFAULT '';
CREATE INDEX dispatch_incident_recovery_due ON dispatch_delivery_incidents(recovery_next_at) WHERE recovery_result IN ('PENDING','UNKNOWN');

-- Replacement offers reuse the provider offer inbox but do not create another
-- Trip or silently transfer custody. Historical initial winners stay immutable.
ALTER TABLE dispatch_assignment_offers ADD COLUMN incident_id uuid REFERENCES dispatch_delivery_incidents(id);
DROP INDEX dispatch_assignment_one_winner;
CREATE UNIQUE INDEX dispatch_assignment_one_winner ON dispatch_assignment_offers(mission_id) WHERE status='ACCEPTED' AND incident_id IS NULL;
CREATE UNIQUE INDEX dispatch_assignment_one_replacement_winner ON dispatch_assignment_offers(incident_id) WHERE status='ACCEPTED' AND incident_id IS NOT NULL;
CREATE UNIQUE INDEX dispatch_incident_one_active_recovery ON dispatch_delivery_incidents(trip_id) WHERE recovery_result IN ('PENDING','UNKNOWN','READY') AND state='REASSIGNING';

-- Immutable, transaction-time source correlation. Existing domain schemas keep
-- their fields; only the allowlisted projection is delivered to Vendor.
CREATE FUNCTION dispatch_vendor_lifecycle_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE group_id uuid; mission_id uuid; projection jsonb;
BEGIN
 IF NEW.target_service<>'vendor-service' OR NEW.event_type NOT IN ('dispatch.delivery_plan.created.v1','dispatch.delivery_plan.superseded.v1','dispatch.mission.assigned.v1','dispatch.pickup.verified.v1','dispatch.receiver.verified.v1','dispatch.return.completed.v1','dispatch.return.started.v1','dispatch.delivery_recovery.selected.v1','dispatch.provider_replacement.started.v1') THEN RETURN NEW; END IF;
 IF NEW.aggregate_type='delivery_group' THEN group_id:=NEW.aggregate_id::uuid;
 ELSIF NEW.aggregate_type='delivery_mission' THEN mission_id:=NEW.aggregate_id::uuid;
 ELSIF NEW.aggregate_type='handover_challenge' THEN SELECT c.mission_id INTO mission_id FROM dispatch_handover_challenges c WHERE c.challenge_ref=NEW.aggregate_id;
 ELSIF NEW.aggregate_type='vendor_delivery_projection' THEN SELECT m.id INTO mission_id FROM dispatch_delivery_missions m WHERE m.trip_id=(NEW.payload->>'trip_id')::uuid ORDER BY m.updated_at DESC LIMIT 1;
 END IF;
 IF group_id IS NULL THEN SELECT m.delivery_group_id INTO group_id FROM dispatch_delivery_missions m WHERE m.id=mission_id; END IF;
 SELECT jsonb_build_object('source_ref',g.source_ref,'source_version',g.source_version,'group_ref',g.id::text,'missions',
  (SELECT jsonb_agg(jsonb_build_object('mission_ref',m.id::text,'version',m.version,'state',m.state,'parcel_refs',
   (SELECT COALESCE(jsonb_agg(p.parcel_ref ORDER BY p.parcel_ref),'[]') FROM dispatch_parcels p JOIN dispatch_manifest_parcels mp ON mp.parcel_id=p.id JOIN dispatch_parcel_manifests pm ON pm.id=mp.manifest_id WHERE pm.mission_id=m.id AND pm.version=(SELECT MAX(last.version) FROM dispatch_parcel_manifests last WHERE last.mission_id=m.id))) ORDER BY m.id)
   FROM dispatch_delivery_missions m WHERE m.delivery_group_id=g.id AND (mission_id IS NULL OR m.id=mission_id)))
 INTO projection FROM dispatch_delivery_groups g WHERE g.id=group_id AND g.source_type='VENDOR_COMMERCE';
 IF projection IS NOT NULL THEN NEW.payload:=NEW.payload||jsonb_build_object('delivery_projection',projection);NEW.payload_hash:=encode(digest(NEW.payload::text,'sha256'),'hex'); END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER dispatch_vendor_lifecycle_snapshot BEFORE INSERT OR UPDATE OF payload ON dispatch_marketplace_outbox FOR EACH ROW EXECUTE FUNCTION dispatch_vendor_lifecycle_snapshot();
-- Capture an explicit current-state read-back for undelivered pre-adapter work.
-- Published history is never rewritten; dead letters still need operator replay.
UPDATE dispatch_marketplace_outbox SET payload=payload WHERE target_service='vendor-service' AND status IN ('PENDING','FAILED') AND event_type LIKE 'dispatch.%' AND NOT payload ? 'delivery_projection';

CREATE FUNCTION dispatch_recovery_vendor_fanout() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.target_service='notification-service' AND NEW.event_type IN ('dispatch.return.started.v1','dispatch.delivery_recovery.selected.v1','dispatch.provider_replacement.started.v1') AND NEW.aggregate_type='trip' AND EXISTS(SELECT 1 FROM dispatch_delivery_missions m JOIN dispatch_delivery_groups g ON g.id=m.delivery_group_id WHERE m.trip_id=NEW.aggregate_id::uuid AND g.source_type='VENDOR_COMMERCE') THEN
  INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service)
  VALUES(NEW.event_type,'vendor_delivery_projection',NEW.aggregate_id,NEW.aggregate_version,NEW.payload||jsonb_build_object('trip_id',NEW.aggregate_id),NEW.payload_hash,'vendor-service') ON CONFLICT DO NOTHING;
 END IF;RETURN NEW;
END $$;
CREATE TRIGGER dispatch_recovery_vendor_fanout AFTER INSERT ON dispatch_marketplace_outbox FOR EACH ROW EXECUTE FUNCTION dispatch_recovery_vendor_fanout();

-- Each alert has its own identity. Multiple alerts on one route version must
-- not collide, and scheduler-created stale alerts need the same atomic intent.
CREATE FUNCTION dispatch_route_alert_analytics_intent() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE payload jsonb;
BEGIN
 payload:=jsonb_build_object('route_version',NEW.route_version,'alert_type',NEW.alert_type,'severity',NEW.severity,'state',NEW.state,'policy_version',NEW.policy_version);
 INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service)
 VALUES('dispatch.route_alert.opened.v1','route_alert',NEW.id::text,1,payload,encode(digest(payload::text,'sha256'),'hex'),'analytics-service') ON CONFLICT DO NOTHING;
 RETURN NEW;
END $$;
CREATE TRIGGER dispatch_route_alert_analytics_intent AFTER INSERT ON dispatch_route_alerts FOR EACH ROW EXECUTE FUNCTION dispatch_route_alert_analytics_intent();
