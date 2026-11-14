-- Reverses 000090 exactly.

DROP TABLE IF EXISTS fleet_partner_operating_locations;
DROP TABLE IF EXISTS fleet_partner_teams;
ALTER TABLE fleet_partner_departments
    DROP CONSTRAINT IF EXISTS fk_fleet_partner_department_cost_centre;
DROP TABLE IF EXISTS fleet_partner_cost_centres;

DELETE FROM fleet_partner_role_permissions
WHERE role_id = 'rol_system_admin' AND permission_code = 'approval_policies.manage';

DROP INDEX IF EXISTS idx_fleet_partner_delegations_delegator;
DROP INDEX IF EXISTS uq_fleet_partner_delegation_idempotency;
ALTER TABLE fleet_partner_approval_delegations
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_delegation_revocation,
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_delegation_window,
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS revocation_reason,
    DROP COLUMN IF EXISTS revoked_by,
    DROP COLUMN IF EXISTS reason,
    DROP COLUMN IF EXISTS branch_id;

DROP INDEX IF EXISTS uq_fleet_partner_decision_on_behalf_per_step;
-- Decisions are append-only; dropping columns is a schema change, not a row
-- mutation, so the append-only trigger does not fire.
ALTER TABLE fleet_partner_approval_decisions
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_approval_decision_delegation,
    DROP COLUMN IF EXISTS on_behalf_of,
    DROP COLUMN IF EXISTS delegation_id;

ALTER TABLE fleet_partner_approval_requests
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_approval_request_amount,
    DROP COLUMN IF EXISTS amount_currency,
    DROP COLUMN IF EXISTS amount_minor;

DROP TABLE IF EXISTS fleet_partner_approval_policy_versions;

DROP INDEX IF EXISTS uq_fleet_partner_approval_policy_idempotency;
DROP INDEX IF EXISTS uq_fleet_partner_approval_policy_code;
ALTER TABLE fleet_partner_approval_policies
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_approval_policy_threshold,
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_approval_policy_scope,
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_approval_policy_code,
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_approval_policy_origin,
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS origin,
    DROP COLUMN IF EXISTS code;
