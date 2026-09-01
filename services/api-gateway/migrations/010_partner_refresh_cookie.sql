-- Extend the canonical Partner dashboard session; no second session authority.
ALTER TABLE partner_sessions ADD COLUMN refresh_token_hash TEXT;
CREATE UNIQUE INDEX partner_sessions_refresh_hash_unique ON partner_sessions(refresh_token_hash) WHERE refresh_token_hash IS NOT NULL;
ALTER TABLE partner_sessions ADD CONSTRAINT partner_sessions_refresh_hash_shape CHECK (refresh_token_hash IS NULL OR refresh_token_hash ~ '^[0-9a-f]{64}$');
