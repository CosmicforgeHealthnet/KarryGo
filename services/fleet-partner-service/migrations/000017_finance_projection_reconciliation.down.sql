DROP TABLE IF EXISTS fleet_partner_finance_projection_history;
DROP INDEX IF EXISTS idx_fleet_finance_projection_reconciliation_lease;
DROP INDEX IF EXISTS idx_fleet_finance_projection_reconciliation_due;
ALTER TABLE fleet_partner_finance_projections
 DROP COLUMN IF EXISTS reconciliation_safe_error_code,
 DROP COLUMN IF EXISTS reconciliation_lease_expires_at,
 DROP COLUMN IF EXISTS reconciliation_lease_owner,
 DROP COLUMN IF EXISTS reconciliation_next_attempt_at,
 DROP COLUMN IF EXISTS reconciliation_attempts,
 DROP COLUMN IF EXISTS reconciliation_status;
