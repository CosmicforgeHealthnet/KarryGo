ALTER TABLE payment_intents DROP COLUMN IF EXISTS job_completed_at;
DROP TABLE IF EXISTS referral_outbox;
