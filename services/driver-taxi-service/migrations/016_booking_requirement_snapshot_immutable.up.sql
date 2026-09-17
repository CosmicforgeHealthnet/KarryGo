-- A passenger booking's product and requirement snapshot is frozen at
-- creation. Matching, recovery and reassignment read this snapshot, so it
-- must never change afterwards. Legacy bookings without a snapshot (NULL)
-- stay NULL: a snapshot is never backfilled onto a committed booking.
CREATE OR REPLACE FUNCTION guard_taxi_booking_requirement_snapshot() RETURNS trigger AS $$
BEGIN
  IF NEW.service_product_code IS DISTINCT FROM OLD.service_product_code
     OR NEW.service_product_version IS DISTINCT FROM OLD.service_product_version
     OR NEW.eligibility_policy_version IS DISTINCT FROM OLD.eligibility_policy_version
     OR NEW.guarantee_profile_version IS DISTINCT FROM OLD.guarantee_profile_version
     OR NEW.requirement_snapshot_json IS DISTINCT FROM OLD.requirement_snapshot_json THEN
    RAISE EXCEPTION 'taxi booking % requirement snapshot is immutable', OLD.id USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS taxi_booking_requirement_snapshot_immutable ON taxi_bookings;
CREATE TRIGGER taxi_booking_requirement_snapshot_immutable
  BEFORE UPDATE OF service_product_code, service_product_version, eligibility_policy_version, guarantee_profile_version, requirement_snapshot_json
  ON taxi_bookings FOR EACH ROW EXECUTE FUNCTION guard_taxi_booking_requirement_snapshot();

ALTER TABLE taxi_bookings DROP CONSTRAINT IF EXISTS taxi_bookings_product_snapshot_complete;
ALTER TABLE taxi_bookings ADD CONSTRAINT taxi_bookings_product_snapshot_complete CHECK (
  (service_product_code IS NULL AND requirement_snapshot_json IS NULL)
  OR (service_product_code IS NOT NULL AND service_product_version IS NOT NULL AND eligibility_policy_version IS NOT NULL
      AND guarantee_profile_version IS NOT NULL AND requirement_snapshot_json IS NOT NULL)
) NOT VALID;
