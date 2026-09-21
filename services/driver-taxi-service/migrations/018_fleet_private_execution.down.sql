-- Reverse of 018. The taxi_bookings rows a private execution created are NOT
-- deleted: they are real operational and financial history, and a rollback of
-- the mapping layer must not erase trips that happened.
DROP INDEX IF EXISTS idx_taxi_private_execution_organization;
DROP INDEX IF EXISTS uq_taxi_private_execution_booking;
DROP INDEX IF EXISTS uq_taxi_private_execution_assignment;
DROP TABLE IF EXISTS taxi_private_execution_mappings;
