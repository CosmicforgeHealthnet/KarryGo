-- 009_partner_otp_onboarding_down.sql
-- Rollback note: PostgreSQL cannot restore NOT NULL while OTP-created rows hold
-- NULL business_name/password_hash values. To keep rollback clean, NULL values
-- are mapped to empty strings before restoring the old constraints.

DROP TABLE IF EXISTS partner_otp_challenges;

ALTER TABLE partner_accounts
    DROP COLUMN IF EXISTS profile_photo_url;

UPDATE partner_accounts SET password_hash = '' WHERE password_hash IS NULL;
ALTER TABLE partner_accounts
    ALTER COLUMN password_hash SET DEFAULT '',
    ALTER COLUMN password_hash SET NOT NULL;

UPDATE partner_accounts SET business_name = '' WHERE business_name IS NULL;
ALTER TABLE partner_accounts
    ALTER COLUMN business_name SET NOT NULL;