-- 009_partner_otp_onboarding.sql
-- Partner web app OTP signup + onboarding profile fields.

CREATE TABLE IF NOT EXISTS partner_otp_challenges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_email text NOT NULL,
    otp_code_hash text NOT NULL,
    attempts int NOT NULL DEFAULT 0,
    consumed_at timestamptz,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_otp_email ON partner_otp_challenges (business_email);

ALTER TABLE partner_accounts
    ALTER COLUMN password_hash DROP NOT NULL,
    ALTER COLUMN business_name DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS profile_photo_url text;