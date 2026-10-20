CREATE TABLE IF NOT EXISTS analytics_admin_http_nonces (
 caller text NOT NULL CHECK(caller='admin-backoffice-service'),
 nonce_hash text NOT NULL CHECK(nonce_hash ~ '^[0-9a-f]{64}$'),
 expires_at timestamptz NOT NULL,
 PRIMARY KEY(caller,nonce_hash)
);
CREATE INDEX IF NOT EXISTS analytics_admin_http_nonces_expiry ON analytics_admin_http_nonces(expires_at);
