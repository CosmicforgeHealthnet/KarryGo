-- Bridge-owned security metadata only. No domain tables or domain SQL access.
-- Applied in one transaction under the application's migration advisory lock.
CREATE TABLE admin_remote_principals (
 principal text NOT NULL CHECK (principal = 'cosmicforge-hq-karryam-admin'),
 certificate_sha256 text NOT NULL CHECK (certificate_sha256 ~ '^[0-9a-f]{64}$'),
 workload_uri text NOT NULL,
 product text NOT NULL CHECK (product = 'karryam'),
 audience text NOT NULL,
 expires_at timestamptz NOT NULL,
 revoked_at timestamptz,
 PRIMARY KEY (principal, certificate_sha256)
);
CREATE TABLE admin_delegated_operations (
 id uuid PRIMARY KEY,
 principal text NOT NULL,
 staff_ref text NOT NULL,
 market text NOT NULL,
 organization text NOT NULL,
 idempotency_key text NOT NULL,
 request_digest text NOT NULL CHECK (length(request_digest)=64),
 action text NOT NULL,
 target_type text NOT NULL,
 target_ref text NOT NULL,
 expected_version bigint NOT NULL CHECK (expected_version > 0),
 source_version bigint CHECK (source_version > 0),
 status text NOT NULL CHECK (status IN ('PENDING','EXECUTING','CONFIRMED','REJECTED','FAILED_RETRYABLE','FAILED_TERMINAL','UNKNOWN_OUTCOME','CANCELLED_WHEN_SAFE')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (principal, market, organization, idempotency_key)
);
CREATE INDEX admin_operations_staff_scope ON admin_delegated_operations (staff_ref,market,organization,created_at,id);
CREATE TABLE admin_approvals (
 id text PRIMARY KEY,
 operator_ref text NOT NULL,
 approver_ref text NOT NULL CHECK (approver_ref <> operator_ref),
 action text NOT NULL,
 request_digest text NOT NULL CHECK (length(request_digest)=64),
 market text NOT NULL,
 organization text NOT NULL,
 policy_version text NOT NULL,
 expires_at timestamptz NOT NULL,
 evidence_ref text NOT NULL,
 incident_ref text,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX admin_approvals_expiry ON admin_approvals(expires_at);
CREATE TABLE admin_audit_entries (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 correlation_id uuid NOT NULL,
 product text NOT NULL CHECK (product='karryam'),
 principal text NOT NULL,
 staff_ref text NOT NULL,
 permission text NOT NULL,
 policy_version text NOT NULL,
 target_service text NOT NULL,
 target_ref text NOT NULL,
 request_type text NOT NULL,
 market text NOT NULL,
 organization text NOT NULL,
 reason_code text NOT NULL,
 outcome text NOT NULL,
 source_version bigint NOT NULL DEFAULT 0,
 operation_status text NOT NULL,
 evidence_ref text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX admin_audit_scope_time ON admin_audit_entries(market,organization,created_at,id);
CREATE INDEX admin_audit_correlation ON admin_audit_entries(correlation_id);
CREATE TABLE admin_security_claims (
 kind text NOT NULL CHECK (kind IN ('nonce','jti')),
 claim_hash text NOT NULL,
 expires_at timestamptz NOT NULL,
 PRIMARY KEY(kind,claim_hash)
);
CREATE INDEX admin_claims_expiry ON admin_security_claims(expires_at);
CREATE TABLE admin_revocations (
 kind text NOT NULL CHECK (kind IN ('staff','session','approval')),
 reference text NOT NULL,
 revoked_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(kind,reference)
);
CREATE TABLE admin_rate_limits (
 key_hash text NOT NULL,
 window_start timestamptz NOT NULL,
 count integer NOT NULL CHECK(count > 0),
 PRIMARY KEY(key_hash,window_start)
);
CREATE FUNCTION admin_immutable_evidence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'admin security evidence is append only'; END $$;
CREATE TRIGGER admin_audit_immutable BEFORE UPDATE OR DELETE ON admin_audit_entries
 FOR EACH ROW EXECUTE FUNCTION admin_immutable_evidence();
CREATE TRIGGER admin_approvals_immutable BEFORE UPDATE OR DELETE ON admin_approvals
 FOR EACH ROW EXECUTE FUNCTION admin_immutable_evidence();
REVOKE ALL ON admin_audit_entries,admin_approvals FROM PUBLIC;
