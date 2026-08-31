-- Removing a guard cannot and must not restore erased historical credentials.
ALTER TABLE trips DROP CONSTRAINT IF EXISTS trips_pickup_reference_only;
ALTER TABLE trips DROP CONSTRAINT IF EXISTS trips_delivery_reference_only;
ALTER TABLE commerce_mission_stops DROP CONSTRAINT IF EXISTS commerce_stop_reference_only;
ALTER TABLE commerce_mission_operations DROP CONSTRAINT IF EXISTS commerce_operation_reference_only;
