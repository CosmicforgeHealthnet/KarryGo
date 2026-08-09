-- Reverses WP8 finance operator access.

DELETE FROM fleet_partner_role_permissions
WHERE permission_code IN ('finance.operations.read', 'finance.operations.replay',
                          'finance.reconciliation.read', 'finance.reconciliation.manage',
                          'finance.settlements.request', 'finance.refunds.request',
                          'finance.reversals.request');

DELETE FROM fleet_partner_permissions
WHERE code IN ('finance.operations.read', 'finance.operations.replay',
               'finance.reconciliation.read', 'finance.reconciliation.manage',
               'finance.settlements.request', 'finance.refunds.request',
               'finance.reversals.request');

DROP INDEX IF EXISTS idx_fleet_finance_projections_branch;
DROP INDEX IF EXISTS idx_fleet_finance_operations_requester;
DROP INDEX IF EXISTS idx_fleet_finance_operations_branch;

ALTER TABLE fleet_partner_finance_projections
    DROP COLUMN IF EXISTS branch_id;

ALTER TABLE fleet_partner_finance_operations
    DROP COLUMN IF EXISTS branch_id,
    DROP COLUMN IF EXISTS requested_by_actor_id,
    DROP COLUMN IF EXISTS requested_by_actor_type;
