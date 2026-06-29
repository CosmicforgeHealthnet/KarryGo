CREATE TABLE provider_settings (
    provider_id UUID        PRIMARY KEY REFERENCES providers(id) ON DELETE CASCADE,
    push_enabled BOOLEAN NOT NULL DEFAULT true,
    sms_enabled BOOLEAN NOT NULL DEFAULT true,
    language TEXT NOT NULL DEFAULT 'en',
    dark_mode_enabled BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_provider_settings_language CHECK (language IN ('en','fr','es','it','pt','yo','ig','ha'))
);
