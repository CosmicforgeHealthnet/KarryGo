-- 002_partner_auth_down.sql
-- Rollback for 002_partner_auth.sql.

DROP TABLE IF EXISTS partner_sessions;

ALTER TABLE partner_accounts DROP CONSTRAINT IF EXISTS partner_accounts_status_check;
ALTER TABLE partner_accounts ADD CONSTRAINT partner_accounts_status_check
    CHECK (status IN ('active', 'suspended', 'pending'));

ALTER TABLE partner_accounts DROP COLUMN IF EXISTS password_hash;
