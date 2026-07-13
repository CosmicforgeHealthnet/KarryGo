-- 005_partner_api_key_scopes.sql
-- Adds the Generate API Key form fields (description, expiration, permissions)
-- that were missing from 001_partner_schema.sql's partner_api_keys table.
--
-- permissions defaults to '{}' (empty) rather than allowing NULL, so reading it
-- back is always a plain []string with no null-slice special-casing needed.
-- NOTE: the Figma form does not mark Permissions as required, so a key CAN be
-- created with zero permissions - which means it will be rejected by every
-- permission-gated route (see partnerauth.RequirePermission). That is a real
-- design gap worth revisiting on the frontend, not something this migration
-- can fix by itself.

ALTER TABLE partner_api_keys
    ADD COLUMN IF NOT EXISTS description text,
    ADD COLUMN IF NOT EXISTS expires_at timestamptz,
    ADD COLUMN IF NOT EXISTS permissions text[] NOT NULL DEFAULT '{}';