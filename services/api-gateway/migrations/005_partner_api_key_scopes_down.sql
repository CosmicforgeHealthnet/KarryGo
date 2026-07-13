-- 005_partner_api_key_scopes_down.sql
ALTER TABLE partner_api_keys
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS permissions;