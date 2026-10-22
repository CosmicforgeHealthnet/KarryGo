-- The command ledger is immutable decision evidence. Rollback is refused once
-- any decision exists; an empty ledger can be removed.
DO $$ BEGIN
 IF to_regclass('admin_owner_command_ledger') IS NOT NULL AND EXISTS(SELECT 1 FROM admin_owner_command_ledger) THEN
  RAISE EXCEPTION 'Owner administrative command evidence requires an approved forward retention migration';
 END IF;
END $$;
DROP TABLE IF EXISTS admin_owner_command_ledger;
DROP FUNCTION IF EXISTS reject_admin_owner_command_ledger_mutation();
