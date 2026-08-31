DROP TRIGGER IF EXISTS dispatch_booking_notification_intent ON dispatch_bookings;
DROP TRIGGER IF EXISTS dispatch_route_alert_analytics_intent ON dispatch_route_alerts;
DROP FUNCTION IF EXISTS dispatch_route_alert_analytics_intent();
DROP TRIGGER IF EXISTS dispatch_recovery_vendor_fanout ON dispatch_marketplace_outbox;
DROP FUNCTION IF EXISTS dispatch_recovery_vendor_fanout();
DROP TRIGGER IF EXISTS dispatch_vendor_lifecycle_snapshot ON dispatch_marketplace_outbox;
DROP FUNCTION IF EXISTS dispatch_vendor_lifecycle_snapshot();
DROP TRIGGER IF EXISTS dispatch_trip_notification_intent ON trips;
DROP FUNCTION IF EXISTS dispatch_booking_notification_intent();
-- Rollback refuses to destroy historical replacement winners. An applied
-- deployment with such history must retain this additive schema.
DO $$ BEGIN IF EXISTS(SELECT 1 FROM dispatch_assignment_offers WHERE incident_id IS NOT NULL) THEN RAISE EXCEPTION 'replacement history exists; rollback requires retention-safe maintenance'; END IF; END $$;
DROP INDEX dispatch_incident_one_active_recovery;
DROP INDEX dispatch_assignment_one_replacement_winner;
DROP INDEX dispatch_assignment_one_winner;
CREATE UNIQUE INDEX dispatch_assignment_one_winner ON dispatch_assignment_offers(mission_id) WHERE status='ACCEPTED';
ALTER TABLE dispatch_assignment_offers DROP COLUMN incident_id;
DROP INDEX IF EXISTS dispatch_incident_recovery_due;
ALTER TABLE dispatch_delivery_incidents DROP COLUMN recovery_attempts,DROP COLUMN recovery_next_at,DROP COLUMN recovery_deadline,DROP COLUMN recovery_lease_owner,DROP COLUMN recovery_lease_expires_at,DROP COLUMN recovery_result,DROP COLUMN replacement_provider_id,DROP COLUMN recovery_error_code;
