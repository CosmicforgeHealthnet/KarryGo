ALTER TABLE partner_sessions DROP CONSTRAINT IF EXISTS partner_sessions_refresh_hash_shape;
DROP INDEX IF EXISTS partner_sessions_refresh_hash_unique;
ALTER TABLE partner_sessions DROP COLUMN IF EXISTS refresh_token_hash;
