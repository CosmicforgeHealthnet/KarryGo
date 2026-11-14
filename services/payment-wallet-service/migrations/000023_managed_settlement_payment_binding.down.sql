DROP INDEX IF EXISTS idx_managed_settlements_payment_intent;
DROP INDEX IF EXISTS uq_managed_settlements_posted_payment;
ALTER TABLE managed_settlements
    DROP COLUMN IF EXISTS payment_source_reference,
    DROP COLUMN IF EXISTS payment_source_service,
    DROP COLUMN IF EXISTS payment_intent_id;
