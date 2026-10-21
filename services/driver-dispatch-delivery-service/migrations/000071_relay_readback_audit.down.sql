DO $$ BEGIN RAISE EXCEPTION 'Read-back audit retention requires an approved forward migration'; END $$;
