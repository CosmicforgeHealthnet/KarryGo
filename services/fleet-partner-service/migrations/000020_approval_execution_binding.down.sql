-- Reverses WP9.2 approval execution binding.

DROP TRIGGER IF EXISTS trg_fleet_partner_approval_executions_append_only
    ON fleet_partner_approval_executions;
DROP TABLE IF EXISTS fleet_partner_approval_executions;

DROP INDEX IF EXISTS uq_fleet_partner_decision_per_request;
DROP INDEX IF EXISTS idx_fleet_partner_approval_decisions_request;

ALTER TABLE fleet_partner_approval_decisions
    DROP COLUMN IF EXISTS correlation_id,
    DROP COLUMN IF EXISTS actor_role_id,
    DROP COLUMN IF EXISTS approved_fingerprint,
    DROP COLUMN IF EXISTS request_version;

DROP INDEX IF EXISTS idx_fleet_partner_approval_requests_fingerprint;
DROP INDEX IF EXISTS idx_fleet_partner_approval_requests_subject;
DROP INDEX IF EXISTS idx_fleet_partner_approval_requests_branch;
DROP INDEX IF EXISTS uq_fleet_partner_approval_request_idempotency;
DROP INDEX IF EXISTS uq_fleet_partner_approval_execution_reference;

ALTER TABLE fleet_partner_approval_requests
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_approval_invalidation_reason,
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_approval_execution_evidence,
    DROP CONSTRAINT IF EXISTS chk_fleet_partner_approval_request_status;

-- Rows in states 000006 did not know about cannot survive the constraint being
-- narrowed again, so they are returned to CANCELLED with the reason preserved
-- in the append-only history rather than silently deleted.
UPDATE fleet_partner_approval_requests
SET status = 'CANCELLED'
WHERE status = 'INVALIDATED';

ALTER TABLE fleet_partner_approval_requests
    ADD CONSTRAINT fleet_partner_approval_requests_status_check
    CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'EXPIRED', 'CANCELLED', 'EXECUTED'));

ALTER TABLE fleet_partner_approval_requests
    DROP COLUMN IF EXISTS causation_id,
    DROP COLUMN IF EXISTS idempotency_key,
    DROP COLUMN IF EXISTS branch_id,
    DROP COLUMN IF EXISTS invalidation_reason,
    DROP COLUMN IF EXISTS invalidated_at,
    DROP COLUMN IF EXISTS execution_reference,
    DROP COLUMN IF EXISTS eligibility_decision_id,
    DROP COLUMN IF EXISTS separation_of_duties,
    DROP COLUMN IF EXISTS self_approval_allowed,
    DROP COLUMN IF EXISTS eligible_role_ids,
    DROP COLUMN IF EXISTS minimum_approvals,
    DROP COLUMN IF EXISTS policy_version,
    DROP COLUMN IF EXISTS payload_version,
    DROP COLUMN IF EXISTS expected_subject_version;

ALTER TABLE fleet_partner_approval_policies
    DROP COLUMN IF EXISTS requires_current_eligibility,
    DROP COLUMN IF EXISTS eligibility_policy_key,
    DROP COLUMN IF EXISTS separation_of_duties;
