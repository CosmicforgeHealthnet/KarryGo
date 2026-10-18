CREATE TABLE taxi_admin_policy_revisions (
 policy_id text PRIMARY KEY,
 version bigint NOT NULL CHECK(version>0),
 checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),
 accepted_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE taxi_admin_read_audit
 ADD COLUMN tenant_ref text NOT NULL DEFAULT '',
 ADD COLUMN approved_role text NOT NULL DEFAULT '',
 ADD COLUMN policy_id text NOT NULL DEFAULT '',
 ADD COLUMN policy_version bigint,
 ADD COLUMN policy_checksum text NOT NULL DEFAULT '';
