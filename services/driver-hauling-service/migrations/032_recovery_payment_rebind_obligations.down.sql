-- Reverse of 032.
--
-- An obligation that has not reached a terminal state is evidence that a held
-- payment may still be bound to the wrong provider. Dropping it silently would
-- erase the only record of that, so the rollback refuses while any is open.
DO $$ BEGIN
 IF to_regclass('hauling_payment_rebind_obligations') IS NOT NULL AND EXISTS (
   SELECT 1 FROM hauling_payment_rebind_obligations WHERE state IN ('PENDING', 'UNKNOWN_OUTCOME'))
 THEN RAISE EXCEPTION 'Open payment rebind obligations exist; resolve them before rolling back 032'; END IF;
END $$;
DROP TABLE IF EXISTS hauling_payment_rebind_obligations;
