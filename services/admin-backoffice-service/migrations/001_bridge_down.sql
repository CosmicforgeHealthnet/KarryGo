-- Intentionally irreversible: rollback binaries, retain audit/approval evidence.
-- Destructive schema removal requires a separately reviewed retention procedure.
DO $$ BEGIN RAISE EXCEPTION 'bridge migration is irreversible; retain security evidence'; END $$;
