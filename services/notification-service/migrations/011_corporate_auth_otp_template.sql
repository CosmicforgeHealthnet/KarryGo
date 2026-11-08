-- Ensure Fleet corporate email OTP has a production/staging template.
-- Previously this existed only in dev_templates.sql. Deployment environments
-- may not run dev seeds, causing subject-only emails when the producer used
-- the legacy corporate_auth_otp key.
-- Preserve any customized non-empty template already provisioned by an operator.
INSERT INTO notification_templates
    (key, locale, title, body, default_channels, active)
VALUES
    ('corporate.auth.otp', 'en-NG',
     'Your Karryam verification code',
     'Your verification code is {{code}}. It expires in {{expires_in_minutes}} minutes. Never share this code.',
     '["email"]'::jsonb, true)
ON CONFLICT (key, locale) DO NOTHING;
