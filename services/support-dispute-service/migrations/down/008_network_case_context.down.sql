-- Reverses 008_network_case_context.sql.
--
-- Kept in migrations/down/ because the runner applies every file directly in
-- migrations/ on every start. Run this by hand, deliberately.
--
-- The append-only trigger is dropped before its table so the drop is not
-- itself refused. Nothing here touches a column or constraint 008 did not add.
DROP INDEX IF EXISTS idx_support_action_requests_pending;
ALTER TABLE IF EXISTS support_action_requests
    DROP CONSTRAINT IF EXISTS support_action_requests_completion_chk;
DROP INDEX IF EXISTS uq_support_action_requests_key;
DROP TABLE IF EXISTS support_action_requests;

DROP TRIGGER IF EXISTS complaint_context_snapshots_no_mutation ON complaint_context_snapshots;
DROP FUNCTION IF EXISTS complaint_context_snapshots_append_only();
DROP INDEX IF EXISTS idx_complaint_context_snapshots_complaint;
DROP TABLE IF EXISTS complaint_context_snapshots;

DROP INDEX IF EXISTS idx_complaints_recovery_case_ref;
DROP INDEX IF EXISTS idx_complaints_mission_ref;
ALTER TABLE complaints DROP CONSTRAINT IF EXISTS complaints_context_type_chk;
ALTER TABLE complaints DROP COLUMN IF EXISTS recovery_case_ref;
ALTER TABLE complaints DROP COLUMN IF EXISTS guarantee_breach_ref;
ALTER TABLE complaints DROP COLUMN IF EXISTS asset_ref;
ALTER TABLE complaints DROP COLUMN IF EXISTS provider_ref;
ALTER TABLE complaints DROP COLUMN IF EXISTS mission_ref;
ALTER TABLE complaints DROP COLUMN IF EXISTS fulfilment_ref;
ALTER TABLE complaints DROP COLUMN IF EXISTS order_ref;
ALTER TABLE complaints DROP COLUMN IF EXISTS context_type;
