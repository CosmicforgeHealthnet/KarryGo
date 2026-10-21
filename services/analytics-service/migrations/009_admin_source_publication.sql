-- Nonces for independently signed producer-context publication only.
CREATE TABLE IF NOT EXISTS admin_owner_read_nonces (
 owner text NOT NULL, nonce_hash text NOT NULL, expires_at timestamptz NOT NULL,
 PRIMARY KEY(owner,nonce_hash)
);
CREATE INDEX IF NOT EXISTS admin_owner_read_nonce_expiry ON admin_owner_read_nonces(expires_at);
