-- Reverse of 000031. Drops only what 000031 added; the pre-existing customer
-- booking and independent-provider columns are untouched.

DROP TABLE IF EXISTS dispatch_marketplace_outbox_deliveries;
DROP TABLE IF EXISTS dispatch_marketplace_outbox;
DROP TABLE IF EXISTS dispatch_marketplace_command_operations;
DROP TABLE IF EXISTS dispatch_organization_reservations;

DROP INDEX IF EXISTS uq_trips_fleet_assignment;
DROP INDEX IF EXISTS idx_trips_organization;

ALTER TABLE trips DROP CONSTRAINT IF EXISTS chk_trip_organization_managed_complete;
ALTER TABLE trips DROP CONSTRAINT IF EXISTS chk_trip_acceptance_source;
ALTER TABLE trips DROP CONSTRAINT IF EXISTS chk_trip_execution_mode;

ALTER TABLE trips DROP COLUMN IF EXISTS source_version;
ALTER TABLE trips DROP COLUMN IF EXISTS fleet_correlation_id;
ALTER TABLE trips DROP COLUMN IF EXISTS execution_mode;
ALTER TABLE trips DROP COLUMN IF EXISTS execution_source;
ALTER TABLE trips DROP COLUMN IF EXISTS acceptance_source;
ALTER TABLE trips DROP COLUMN IF EXISTS provider_response_id;
ALTER TABLE trips DROP COLUMN IF EXISTS fleet_assignment_id;
ALTER TABLE trips DROP COLUMN IF EXISTS organization_membership_id;
ALTER TABLE trips DROP COLUMN IF EXISTS settlement_organization_id;
ALTER TABLE trips DROP COLUMN IF EXISTS organization_id;

DROP INDEX IF EXISTS idx_broadcasts_reserved_org;
ALTER TABLE request_broadcasts DROP CONSTRAINT IF EXISTS chk_broadcast_reservation_status;
ALTER TABLE request_broadcasts DROP COLUMN IF EXISTS reservation_status;
ALTER TABLE request_broadcasts DROP COLUMN IF EXISTS reserved_organization_id;
ALTER TABLE request_broadcasts DROP COLUMN IF EXISTS source_version;
