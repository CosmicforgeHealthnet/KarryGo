DROP TABLE IF EXISTS hauling_service_nonces;
DROP TABLE IF EXISTS hauling_domain_outbox;
DROP TABLE IF EXISTS hauling_command_operations;
DROP TABLE IF EXISTS hauling_organization_reservations;
DROP TABLE IF EXISTS hauling_organization_truck_links;
DROP TABLE IF EXISTS hauling_organization_provider_links;
DROP TABLE IF EXISTS hauling_provider_availability;
DROP INDEX IF EXISTS uq_haulage_fleet_assignment;
ALTER TABLE haulage_bookings
  DROP COLUMN IF EXISTS failure_reason, DROP COLUMN IF EXISTS failed_at,
  DROP COLUMN IF EXISTS unloading_completed_at, DROP COLUMN IF EXISTS unloading_started_at,
  DROP COLUMN IF EXISTS loading_completed_at, DROP COLUMN IF EXISTS loading_started_at,
  DROP COLUMN IF EXISTS assignment_response_deadline, DROP COLUMN IF EXISTS assignment_acknowledged_at,
  DROP COLUMN IF EXISTS assignment_status, DROP COLUMN IF EXISTS acceptance_source, DROP COLUMN IF EXISTS provider_response_id,
  DROP COLUMN IF EXISTS fleet_assignment_id, DROP COLUMN IF EXISTS organization_membership_id,
  DROP COLUMN IF EXISTS settlement_organization_id, DROP COLUMN IF EXISTS winner_organization_id,
  DROP COLUMN IF EXISTS source_version;
