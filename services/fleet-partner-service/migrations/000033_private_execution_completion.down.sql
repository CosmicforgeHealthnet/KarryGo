DELETE FROM fleet_partner_role_permissions WHERE permission_code='private_execution.replay';
DELETE FROM fleet_partner_permissions WHERE code='private_execution.replay';
DELETE FROM fleet_partner_role_permissions WHERE permission_code='manifests.seal_amend';
DELETE FROM fleet_partner_permissions WHERE code='manifests.seal_amend';
DROP INDEX IF EXISTS uq_fleet_bulk_import_media_once;
DROP TABLE IF EXISTS fleet_partner_private_execution_reconciliation_attempts;
DROP TABLE IF EXISTS fleet_partner_private_execution_reconciliation_cases;
DROP TABLE IF EXISTS fleet_partner_private_execution_projections;
ALTER TABLE fleet_partner_private_execution_operations
  DROP COLUMN IF EXISTS dead_lettered_at,
  DROP COLUMN IF EXISTS completed_at,
  DROP COLUMN IF EXISTS lease_heartbeat_at,
  DROP COLUMN IF EXISTS causation_id,
  DROP COLUMN IF EXISTS correlation_id,
  DROP COLUMN IF EXISTS execution_payload,
  DROP COLUMN IF EXISTS vehicle_or_truck_reference,
  DROP COLUMN IF EXISTS provider_response_id;
ALTER TABLE fleet_partner_private_execution_operations
  DROP CONSTRAINT IF EXISTS fleet_partner_private_execution_operations_status_check;
ALTER TABLE fleet_partner_private_execution_operations
  ADD CONSTRAINT fleet_partner_private_execution_operations_status_check CHECK(status IN(
    'PENDING','SUBMITTING','OUTCOME_UNKNOWN','CONFIRMED','FAILED',
    'CANCELLATION_PENDING','CANCELLED','RECONCILIATION_REQUIRED'));
