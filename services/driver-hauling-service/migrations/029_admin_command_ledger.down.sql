-- The command ledger is immutable decision evidence. It may be removed only
-- while empty; a populated ledger needs an approved forward retention migration.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM admin_owner_command_decisions)
 THEN RAISE EXCEPTION 'Owner administrative command evidence requires approved forward retention migration'; END IF;
END $$;
DROP TABLE admin_owner_command_decisions;
DROP FUNCTION reject_admin_owner_command_decision_mutation();
