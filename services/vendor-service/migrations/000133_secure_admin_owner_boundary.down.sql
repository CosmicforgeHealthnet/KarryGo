-- Empty boundary rollback only. Historical disclosure/action evidence is never erased.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM admin_owner_command_decisions) OR EXISTS(SELECT 1 FROM vendor_admin_case_history) OR EXISTS(SELECT 1 FROM admin_owner_context_history) OR EXISTS(SELECT 1 FROM admin_owner_read_audit) OR EXISTS(SELECT 1 FROM admin_owner_context_preparation_audit) THEN RAISE EXCEPTION 'Secure Admin owner evidence cannot be rolled back'; END IF;
END $$;
DROP TABLE vendor_admin_case_history;
DROP TABLE vendor_admin_cases;
DROP FUNCTION protect_vendor_admin_case_identity();
DROP FUNCTION reject_vendor_admin_case_history_change();
DROP TABLE IF EXISTS admin_owner_command_decisions;
DROP TABLE IF EXISTS admin_owner_context_history;
DROP TABLE IF EXISTS admin_owner_context_heads;
DROP TABLE IF EXISTS admin_owner_read_audit;
DROP TABLE IF EXISTS admin_owner_context_preparation_audit;
DROP TABLE IF EXISTS admin_owner_read_nonces;
DROP TABLE IF EXISTS admin_owner_policy_revisions;
DROP FUNCTION IF EXISTS reject_admin_owner_command_decision_mutation();
DROP FUNCTION IF EXISTS reject_admin_owner_context_history_mutation();
DROP FUNCTION IF EXISTS reject_admin_owner_read_audit_mutation();
