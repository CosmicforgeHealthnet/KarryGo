-- Reverse of 033.
DROP TABLE IF EXISTS hauling_booking_waiting;
DROP TRIGGER IF EXISTS hauling_waiting_policies_append_only ON hauling_waiting_policies;
DROP TABLE IF EXISTS hauling_waiting_policies;
DROP FUNCTION IF EXISTS hauling_waiting_policies_append_only();
