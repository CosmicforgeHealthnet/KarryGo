-- Rollback is refused while replacement evidence exists: deleting it would
-- destroy the provenance of immutable commercial quotes.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM commerce_checkout_sessions WHERE replacement_source_session_id IS NOT NULL) THEN
        RAISE EXCEPTION 'replacement checkout evidence must be retained';
    END IF;
END $$;
DROP INDEX commerce_checkout_sessions_one_live_idx;
CREATE UNIQUE INDEX commerce_checkout_sessions_one_live_idx ON commerce_checkout_sessions(cart_id)
    WHERE status IN ('VALIDATING','VALIDATED');
DROP INDEX commerce_checkout_replacement_key_idx;
DROP FUNCTION commerce_checkout_evidence_digest(TEXT);
ALTER TABLE commerce_checkout_sessions DROP CONSTRAINT replacement_source_evidence_chk,
    DROP COLUMN replacement_source_session_id, DROP COLUMN replacement_key, DROP COLUMN replacement_source_digest;
