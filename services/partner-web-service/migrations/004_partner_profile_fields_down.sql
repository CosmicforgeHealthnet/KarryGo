-- 004_partner_profile_fields_down.sql
ALTER TABLE partner_accounts
    DROP COLUMN IF EXISTS business_phone,
    DROP COLUMN IF EXISTS state,
    DROP COLUMN IF EXISTS city,
    DROP COLUMN IF EXISTS business_logo_url;