ALTER TABLE payment_intents DROP CONSTRAINT IF EXISTS chk_payment_intents_settlement_mode;
ALTER TABLE payment_intents DROP COLUMN IF EXISTS settlement_mode;
DROP TABLE IF EXISTS managed_settlements;
