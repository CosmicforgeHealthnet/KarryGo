DO $$ BEGIN
  RAISE EXCEPTION 'Relay uncertainty fence is irreversible: resolve outstanding decisions before an operator-managed rollback';
END $$;
