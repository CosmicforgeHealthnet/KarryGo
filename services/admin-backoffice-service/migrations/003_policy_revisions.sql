CREATE TABLE admin_policy_revisions (
 policy_id text PRIMARY KEY,
 version bigint NOT NULL CHECK(version>0),
 checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),
 accepted_at timestamptz NOT NULL DEFAULT now()
);
