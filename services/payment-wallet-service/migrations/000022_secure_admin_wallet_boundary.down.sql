-- Evidence-preserving rollback: only an unused boundary can be removed.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM admin_owner_command_decisions) OR EXISTS(SELECT 1 FROM wallet_admin_case_history) OR EXISTS(SELECT 1 FROM admin_owner_context_history) OR EXISTS(SELECT 1 FROM admin_owner_read_audit) OR EXISTS(SELECT 1 FROM admin_owner_context_preparation_audit) THEN RAISE EXCEPTION 'Secure Admin Wallet evidence cannot be rolled back'; END IF;
END $$;
DROP TRIGGER wallet_admin_case_identity ON wallet_admin_cases;
DROP FUNCTION protect_wallet_admin_case_identity();
DROP TRIGGER wallet_admin_case_history_immutable ON wallet_admin_case_history;
DROP TABLE wallet_admin_case_history;
DROP TABLE wallet_admin_cases;
DROP TRIGGER admin_owner_command_decisions_immutable ON admin_owner_command_decisions;
DROP TABLE admin_owner_command_decisions;
DROP TRIGGER admin_owner_read_audit_immutable ON admin_owner_read_audit;
DROP TABLE admin_owner_read_audit;
DROP TRIGGER admin_owner_context_preparation_immutable ON admin_owner_context_preparation_audit;
DROP TABLE admin_owner_context_preparation_audit;
DROP TABLE admin_owner_read_nonces;
DROP TABLE admin_owner_policy_revisions;
DROP TRIGGER admin_owner_context_history_immutable ON admin_owner_context_history;
DROP TABLE admin_owner_context_history;
DROP TABLE admin_owner_context_heads;
DROP FUNCTION reject_wallet_admin_immutable();
