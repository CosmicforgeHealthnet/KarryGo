DELETE FROM fleet_partner_role_permissions WHERE permission_code IN('profitability.read','profitability.snapshot','cost_allocations.read','cost_allocations.manage');
DELETE FROM fleet_partner_permissions WHERE code IN('profitability.read','profitability.snapshot','cost_allocations.read','cost_allocations.manage');
DROP TRIGGER IF EXISTS trg_fleet_profitability_snapshots_append_only ON fleet_partner_profitability_snapshots;
DROP TABLE IF EXISTS fleet_partner_profitability_snapshots;
DROP TRIGGER IF EXISTS trg_fleet_cost_allocations_append_only ON fleet_partner_cost_allocations;
DROP TABLE IF EXISTS fleet_partner_cost_allocations;
ALTER TABLE fleet_partner_payroll_periods DROP COLUMN IF EXISTS calculation_hash;
