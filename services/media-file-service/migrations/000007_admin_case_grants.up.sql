-- A distinct case-bound ledger, never an ambient owner download token.
CREATE TABLE IF NOT EXISTS media_admin_case_grants (
 id uuid PRIMARY KEY, token_hash text NOT NULL CHECK(token_hash ~ '^[0-9a-f]{64}$'),
 media_object_id text NOT NULL REFERENCES media_objects(id), staff_ref text NOT NULL,
 request jsonb NOT NULL, binding_hash text NOT NULL CHECK(binding_hash ~ '^[0-9a-f]{64}$'),
 policy_id text NOT NULL, policy_version bigint NOT NULL, policy_checksum text NOT NULL,
 object_version bigint NOT NULL CHECK(object_version>0),
 created_at timestamptz NOT NULL, expires_at timestamptz NOT NULL CHECK(expires_at>created_at)
);
CREATE TABLE IF NOT EXISTS media_admin_case_revocations (
 grant_id uuid PRIMARY KEY REFERENCES media_admin_case_grants(id),staff_ref text NOT NULL,
 reason_code text NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE OR REPLACE FUNCTION reject_media_admin_case_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Media administrative case grant evidence is immutable'; END $$;
CREATE TRIGGER media_admin_case_grant_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON media_admin_case_grants FOR EACH STATEMENT EXECUTE FUNCTION reject_media_admin_case_mutation();
CREATE TRIGGER media_admin_case_revocation_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON media_admin_case_revocations FOR EACH STATEMENT EXECUTE FUNCTION reject_media_admin_case_mutation();

CREATE TABLE IF NOT EXISTS media_admin_case_policy_heads (
 policy_id text PRIMARY KEY, version bigint NOT NULL CHECK(version>0),
 checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$')
);
