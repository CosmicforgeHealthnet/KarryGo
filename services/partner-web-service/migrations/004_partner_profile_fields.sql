-- 004_partner_profile_fields.sql
-- Adds the Settings > Profile fields the dashboard needs (business phone,
-- location, logo) that were missing from 001_partner_schema.sql - only
-- business_name/business_email/status/plan/created_at existed before this.
--
-- business_logo_url stores a URL/path to an already-uploaded file (e.g. via
-- media-file-service), not the file itself - this table is not a file store.

ALTER TABLE partner_accounts
    ADD COLUMN IF NOT EXISTS business_phone text,
    ADD COLUMN IF NOT EXISTS state text,
    ADD COLUMN IF NOT EXISTS city text,
    ADD COLUMN IF NOT EXISTS business_logo_url text;