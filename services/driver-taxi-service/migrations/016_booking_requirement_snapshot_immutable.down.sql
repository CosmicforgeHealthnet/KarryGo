ALTER TABLE taxi_bookings DROP CONSTRAINT IF EXISTS taxi_bookings_product_snapshot_complete;
DROP TRIGGER IF EXISTS taxi_booking_requirement_snapshot_immutable ON taxi_bookings;
DROP FUNCTION IF EXISTS guard_taxi_booking_requirement_snapshot();
