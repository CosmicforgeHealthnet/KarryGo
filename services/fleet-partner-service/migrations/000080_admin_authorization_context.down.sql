DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM admin_owner_context_history)
 OR EXISTS(SELECT 1 FROM admin_owner_read_audit)
 OR EXISTS(SELECT 1 FROM admin_owner_context_preparation_audit)
 OR EXISTS(SELECT 1 FROM admin_owner_policy_revisions)
 THEN RAISE EXCEPTION 'Owner approval and disclosure evidence requires approved forward retention migration'; END IF;
END $$;
DROP TABLE admin_owner_context_preparation_audit,admin_owner_read_nonces,admin_owner_read_audit,admin_owner_context_heads,admin_owner_context_history,admin_owner_policy_revisions;
DROP FUNCTION reject_admin_owner_context_history_mutation();
DROP FUNCTION reject_admin_owner_read_audit_mutation();
