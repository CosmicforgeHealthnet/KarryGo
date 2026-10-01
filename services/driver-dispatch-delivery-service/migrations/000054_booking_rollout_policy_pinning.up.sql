-- Pin security-critical rollout requirements when a Booking is admitted.
-- Environment flags may stop new admission but cannot downgrade these facts.
ALTER TABLE dispatch_bookings
    ADD COLUMN rollout_protection_mode TEXT NOT NULL DEFAULT 'LEGACY'
        CHECK (rollout_protection_mode IN ('LEGACY','PROTECTED')),
    ADD COLUMN rollout_policy_version TEXT NOT NULL DEFAULT 'dispatch_rollout_v1',
    ADD COLUMN handover_required BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN pickup_evidence_required BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN delivery_evidence_required BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN provider_transfer_required BOOLEAN NOT NULL DEFAULT FALSE,
    ADD CONSTRAINT dispatch_booking_rollout_requirements_consistent CHECK (
        (rollout_protection_mode='LEGACY' AND NOT handover_required AND NOT pickup_evidence_required AND NOT delivery_evidence_required AND NOT provider_transfer_required)
        OR
        (rollout_protection_mode='PROTECTED' AND handover_required AND
         (NOT pickup_evidence_required OR handover_required) AND
         (NOT delivery_evidence_required OR handover_required) AND
         (NOT provider_transfer_required OR handover_required))
    );

CREATE INDEX dispatch_bookings_protected_active
    ON dispatch_bookings(id)
    WHERE rollout_protection_mode='PROTECTED' AND status NOT IN ('completed','cancelled','unmatched');

COMMENT ON COLUMN dispatch_bookings.rollout_protection_mode IS
    'Immutable admission snapshot: global feature flags may not downgrade PROTECTED work.';

-- Preserve work that already entered the versioned handover authority before
-- this Booking-level snapshot existed. Work admitted under an old global flag
-- but with no challenge yet must be drained or explicitly reconciled by
-- Operations before rollout; the database cannot infer an unrecorded intent.
UPDATE dispatch_bookings b SET
    rollout_protection_mode='PROTECTED',
    handover_required=true,
    pickup_evidence_required=EXISTS(
        SELECT 1 FROM trips t JOIN dispatch_handover_challenges h ON h.trip_id=t.id
        JOIN dispatch_evidence_requirements e ON e.context_key=h.context_key AND e.stage='PICKUP'
        WHERE t.booking_id=b.id AND (e.photo_required OR e.signature_required OR e.gps_required OR e.min_evidence_count>0)),
    delivery_evidence_required=EXISTS(
        SELECT 1 FROM trips t JOIN dispatch_handover_challenges h ON h.trip_id=t.id
        JOIN dispatch_evidence_requirements e ON e.context_key=h.context_key AND e.stage='DELIVERY'
        WHERE t.booking_id=b.id AND (e.photo_required OR e.signature_required OR e.gps_required OR e.min_evidence_count>0)),
    provider_transfer_required=EXISTS(
        SELECT 1 FROM trips t JOIN dispatch_handover_challenges h ON h.trip_id=t.id
        WHERE t.booking_id=b.id AND h.stage='PROVIDER_TRANSFER')
WHERE EXISTS(SELECT 1 FROM trips t JOIN dispatch_handover_challenges h ON h.trip_id=t.id WHERE t.booking_id=b.id);

CREATE OR REPLACE FUNCTION dispatch_reject_rollout_policy_downgrade()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF ROW(NEW.rollout_protection_mode,NEW.rollout_policy_version,NEW.handover_required,
		NEW.pickup_evidence_required,NEW.delivery_evidence_required,NEW.provider_transfer_required)
		IS DISTINCT FROM ROW(OLD.rollout_protection_mode,OLD.rollout_policy_version,OLD.handover_required,
		OLD.pickup_evidence_required,OLD.delivery_evidence_required,OLD.provider_transfer_required) THEN
		RAISE EXCEPTION 'booking rollout policy snapshot is immutable';
	END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER dispatch_booking_rollout_policy_guard
    BEFORE UPDATE OF rollout_protection_mode, rollout_policy_version,
        handover_required, pickup_evidence_required, delivery_evidence_required,
        provider_transfer_required
    ON dispatch_bookings
    FOR EACH ROW EXECUTE FUNCTION dispatch_reject_rollout_policy_downgrade();
