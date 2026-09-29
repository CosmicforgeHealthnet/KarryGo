CREATE TABLE IF NOT EXISTS journey_share_sessions (
 id uuid PRIMARY KEY,
 customer_id uuid NOT NULL REFERENCES customers(id),
 authority text NOT NULL CHECK(authority IN ('TAXI','DISPATCH','HAULING','VENDOR_COMMERCE')),
 source_ref text NOT NULL CHECK(length(source_ref) BETWEEN 1 AND 200),
 product_code text NOT NULL,
 token_hash text NOT NULL UNIQUE CHECK(length(token_hash)=64),
 permissions jsonb NOT NULL,
 status text NOT NULL CHECK(status IN ('ACTIVE','REVOKED','EXPIRED','COMPLETED')),
 expires_at timestamptz NOT NULL,
 source_completed_at timestamptz,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 last_access_at timestamptz,
 source_checked_at timestamptz,
 CHECK((authority='TAXI' AND product_code='RIDE') OR (authority='DISPATCH' AND product_code='SEND') OR (authority='HAULING' AND product_code='TRUCK') OR (authority='VENDOR_COMMERCE' AND product_code IN ('FOOD','SHOP')))
);
CREATE INDEX IF NOT EXISTS journey_share_expiry_idx ON journey_share_sessions(expires_at) WHERE status IN ('ACTIVE','COMPLETED');
CREATE INDEX IF NOT EXISTS journey_share_customer_idx ON journey_share_sessions(customer_id,created_at DESC,id);
CREATE INDEX IF NOT EXISTS journey_share_source_idx ON journey_share_sessions(authority,source_ref);
ALTER TABLE journey_share_sessions ADD COLUMN IF NOT EXISTS source_checked_at timestamptz;
CREATE INDEX IF NOT EXISTS journey_share_observation_idx ON journey_share_sessions(source_checked_at NULLS FIRST,created_at) WHERE status='ACTIVE';
CREATE UNIQUE INDEX IF NOT EXISTS journey_share_active_idx ON journey_share_sessions(customer_id,authority,source_ref) WHERE status='ACTIVE';
-- Replay responses contain metadata only, never the one-time capability.
CREATE TABLE IF NOT EXISTS journey_share_commands (
 customer_id uuid NOT NULL REFERENCES customers(id),
 key_hash text NOT NULL,
 fingerprint text NOT NULL,
 session_id uuid NOT NULL REFERENCES journey_share_sessions(id),
 operation text NOT NULL,
 result jsonb NOT NULL,
 created_at timestamptz NOT NULL,
 PRIMARY KEY(customer_id,key_hash)
);
