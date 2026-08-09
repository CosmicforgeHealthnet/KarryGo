-- Reverse of 000012.

DROP INDEX IF EXISTS idx_fleet_trip_projection_request;
DROP INDEX IF EXISTS idx_fleet_trip_projection_source;
DROP INDEX IF EXISTS idx_fleet_trip_projection_freshness;
DROP INDEX IF EXISTS uq_fleet_reconciliation_open_aggregate;
DROP INDEX IF EXISTS idx_fleet_reconciliation_dead_letter;
DROP INDEX IF EXISTS idx_fleet_reconciliation_lease;

ALTER TABLE fleet_partner_projection_reconciliation_cases
    DROP COLUMN IF EXISTS lease_expires_at;

ALTER TABLE fleet_partner_projection_reconciliation_cases
    DROP COLUMN IF EXISTS lease_owner;

ALTER TABLE fleet_partner_trip_projections
    DROP COLUMN IF EXISTS last_payload_hash;
