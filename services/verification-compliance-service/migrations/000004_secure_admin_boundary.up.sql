-- Section 18: verification and automated onboarding with limited final review.
--
-- Verification stays the sole authority for a vendor case's formal status.
-- Automation records bounded findings and a recommendation only; it never
-- changes status. Formal decisions are taken by named staff through the
-- Secure Admin command boundary (or the existing signed decision route) and
-- every one of them goes through the same native state machine in Go.
--
-- Nothing is seeded: no staff grant, policy, secret, approval or finding.

-- 1. Formal history is append-only from here on.
ALTER TABLE vendor_verification_case_history
 ADD COLUMN IF NOT EXISTS decided_by_kind TEXT CHECK (decided_by_kind IN ('STAFF','SERVICE','SYSTEM')),
 ADD COLUMN IF NOT EXISTS appeal_ref TEXT;
ALTER TABLE vendor_verification_case_history DROP CONSTRAINT IF EXISTS vendor_case_history_decider_named;
ALTER TABLE vendor_verification_case_history ADD CONSTRAINT vendor_case_history_decider_named
 CHECK (decided_by_kind IS NULL OR (decided_by IS NOT NULL AND btrim(decided_by) <> ''));
CREATE OR REPLACE FUNCTION verification_reject_append_only_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION '% is append-only', TG_TABLE_NAME; END $$;
DROP TRIGGER IF EXISTS vendor_verification_case_history_append_only ON vendor_verification_case_history;
CREATE TRIGGER vendor_verification_case_history_append_only BEFORE UPDATE OR DELETE OR TRUNCATE ON vendor_verification_case_history
 FOR EACH STATEMENT EXECUTE FUNCTION verification_reject_append_only_mutation();

-- 2. Automation recommendation and its lease. The recommendation is advisory;
-- the formal status column is never written by automation.
ALTER TABLE vendor_verification_cases
 ADD COLUMN IF NOT EXISTS automation_recommendation TEXT NOT NULL DEFAULT 'NOT_EVALUATED'
   CHECK (automation_recommendation IN ('NOT_EVALUATED','REVIEW_REQUIRED','SOURCE_UNVERIFIED','AUTO_ELIGIBLE','FORMAT_VALID')),
 ADD COLUMN IF NOT EXISTS automation_evaluated_version BIGINT,
 ADD COLUMN IF NOT EXISTS automation_lease_owner TEXT,
 ADD COLUMN IF NOT EXISTS automation_lease_expires_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS vendor_verification_cases_automation_due_idx
 ON vendor_verification_cases(updated_at) WHERE status IN ('PENDING_REVIEW','IN_PROGRESS');
CREATE INDEX IF NOT EXISTS vendor_verification_cases_expiry_due_idx
 ON vendor_verification_cases(expires_at) WHERE status = 'VERIFIED' AND expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS vendor_verification_cases_document_refs_idx
 ON vendor_verification_cases USING gin (document_refs jsonb_path_ops);

CREATE TABLE IF NOT EXISTS vendor_case_automation_findings (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 case_id UUID NOT NULL REFERENCES vendor_verification_cases(id),
 case_version BIGINT NOT NULL CHECK (case_version > 0),
 check_code TEXT NOT NULL CHECK (check_code IN ('DOCUMENT_REFS_PRESENT','DOCUMENT_REF_FORMAT','DUPLICATE_DOCUMENT_ACROSS_BUSINESSES','MEDIA_OBJECT_STATE','EXTERNAL_AUTHORITY','ADDRESS_CONSISTENCY')),
 outcome TEXT NOT NULL CHECK (outcome IN ('FORMAT_VALID','FAIL','SOURCE_UNVERIFIED','NOT_EVALUATED','PASS')),
 severity TEXT NOT NULL CHECK (severity IN ('INFO','WARN','BLOCKING')),
 evaluated_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (case_id, case_version, check_code)
);
CREATE INDEX IF NOT EXISTS vendor_case_automation_findings_case_idx ON vendor_case_automation_findings(case_id, case_version DESC);
DROP TRIGGER IF EXISTS vendor_case_automation_findings_append_only ON vendor_case_automation_findings;
CREATE TRIGGER vendor_case_automation_findings_append_only BEFORE UPDATE OR DELETE OR TRUNCATE ON vendor_case_automation_findings
 FOR EACH STATEMENT EXECUTE FUNCTION verification_reject_append_only_mutation();

-- 3. Appeals. An appeal is recorded only when a reviewed reopen decision is
-- taken; the row is the immutable decision record for that appeal reference.
CREATE TABLE IF NOT EXISTS vendor_case_appeals (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 case_id UUID NOT NULL REFERENCES vendor_verification_cases(id),
 appeal_ref TEXT NOT NULL CHECK (appeal_ref ~ '^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$'),
 status TEXT NOT NULL CHECK (status IN ('REOPENED')),
 reopened_from TEXT NOT NULL CHECK (reopened_from IN ('REJECTED','EXPIRED','SUSPENDED')),
 case_version BIGINT NOT NULL CHECK (case_version > 0),
 decided_operation_ref TEXT NOT NULL CHECK (btrim(decided_operation_ref) <> ''),
 decided_by TEXT NOT NULL CHECK (btrim(decided_by) <> ''),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE (case_id, appeal_ref)
);
DROP TRIGGER IF EXISTS vendor_case_appeals_append_only ON vendor_case_appeals;
CREATE TRIGGER vendor_case_appeals_append_only BEFORE UPDATE OR DELETE OR TRUNCATE ON vendor_case_appeals
 FOR EACH STATEMENT EXECUTE FUNCTION verification_reject_append_only_mutation();

-- 4. Native staff grants. Issued by an owner process, never seeded.
CREATE TABLE IF NOT EXISTS verification_admin_staff_grants (
 id TEXT PRIMARY KEY,
 staff_ref TEXT NOT NULL,
 permission TEXT NOT NULL CHECK (permission IN (
  'verification.vendor_cases.read','verification.findings.read','verification.asset_cases.read',
  'verification.organization_submissions.read','verification.case.review','verification.case.decide')),
 granted_by TEXT NOT NULL,
 approval_ref TEXT NOT NULL,
 expires_at TIMESTAMPTZ,
 revoked_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK (granted_by <> staff_ref),
 UNIQUE (staff_ref, permission)
);
CREATE INDEX IF NOT EXISTS verification_admin_staff_grants_current ON verification_admin_staff_grants(staff_ref, permission) WHERE revoked_at IS NULL;

-- 5. Secure Admin owner context, reads and command decisions (shared
-- templates, embedded verbatim; see internal/features/adminresources tests).
-- Install only through the consuming source owner's normal migration runner.
-- Each owner keeps these records in its own database; Admin never connects.
CREATE TABLE IF NOT EXISTS admin_owner_context_heads (
 owner text NOT NULL,context_id text NOT NULL,revision bigint NOT NULL CHECK(revision>0),
 checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),resource_kind text NOT NULL,resource_ref text NOT NULL,
 product text NOT NULL,market text NOT NULL,zone text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL,
 capabilities text[] NOT NULL,
 PRIMARY KEY(owner,context_id)
);
CREATE INDEX IF NOT EXISTS admin_owner_context_resource ON admin_owner_context_heads(owner,resource_kind,resource_ref);
CREATE INDEX IF NOT EXISTS admin_owner_context_scope ON admin_owner_context_heads(owner,resource_kind,product,market,zone,organization_ref,tenant_ref,resource_ref);
CREATE TABLE IF NOT EXISTS admin_owner_context_history (
 owner text NOT NULL,context_id text NOT NULL,revision bigint NOT NULL CHECK(revision>0),
 checksum text NOT NULL,envelope bytea NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(owner,context_id,revision)
);
CREATE OR REPLACE FUNCTION reject_admin_owner_context_history_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Source-owner authorization history is append-only'; END $$;
DROP TRIGGER IF EXISTS admin_owner_context_history_immutable ON admin_owner_context_history;
CREATE TRIGGER admin_owner_context_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_context_history FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_context_history_mutation();

CREATE TABLE IF NOT EXISTS admin_owner_policy_revisions (
 policy_id text PRIMARY KEY,version bigint NOT NULL CHECK(version>0),checksum text NOT NULL CHECK(checksum ~ '^[0-9a-f]{64}$'),accepted_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS admin_owner_read_nonces (
 owner text NOT NULL,nonce_hash text NOT NULL,expires_at timestamptz NOT NULL,
 PRIMARY KEY(owner,nonce_hash)
);
CREATE INDEX IF NOT EXISTS admin_owner_read_nonce_expiry ON admin_owner_read_nonces(expires_at);
CREATE TABLE IF NOT EXISTS admin_owner_context_preparation_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner text NOT NULL,resource_kind text NOT NULL,target_ref text NOT NULL,
 workload_principal text NOT NULL,purpose_code text NOT NULL,binding_fingerprint text NOT NULL,source_contract text NOT NULL,
 known_source_dimensions jsonb NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now()
);
DROP TRIGGER IF EXISTS admin_owner_context_preparation_immutable ON admin_owner_context_preparation_audit;
CREATE TRIGGER admin_owner_context_preparation_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_context_preparation_audit FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_context_history_mutation();
CREATE TABLE IF NOT EXISTS admin_owner_read_audit (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),owner text NOT NULL,resource_kind text NOT NULL,target_ref text NOT NULL,
 staff_ref text NOT NULL,purpose_code text NOT NULL,market text NOT NULL,zone text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL,
 policy_id text NOT NULL,policy_version bigint NOT NULL,policy_checksum text NOT NULL,as_of timestamptz NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now(),outcome text NOT NULL DEFAULT 'AUTHORIZED'
);
CREATE OR REPLACE FUNCTION reject_admin_owner_read_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Owner administrative read audit is append-only'; END $$;
DROP TRIGGER IF EXISTS admin_owner_read_audit_immutable ON admin_owner_read_audit;
CREATE TRIGGER admin_owner_read_audit_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_read_audit FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_read_audit_mutation();

-- shared-template-sha256: 6b244aa0dce15e2f7300c9b461c4393b0dc0d81d7c25cb9f24d3030e09bd0f29
-- Source-owned administrative command decision record. Install only through
-- the consuming owner's normal migration runner; each owner keeps its own
-- immutable copy in its own database. The Admin Bridge never connects here.
CREATE TABLE IF NOT EXISTS admin_owner_command_decisions (
 idempotency_key text PRIMARY KEY,
 operation_id uuid NOT NULL UNIQUE,
 decision_id uuid NOT NULL UNIQUE,
 owner text NOT NULL,
 contract text NOT NULL,
 subject_kind text NOT NULL,
 subject_id text NOT NULL,
 case_ref text NOT NULL DEFAULT '',
 action text NOT NULL,
 version_scope text NOT NULL CHECK (version_scope IN ('SUBJECT','CASE')),
 version_key text NOT NULL,
 recorded_version bigint NOT NULL CHECK (recorded_version > 0),
 operation_digest text NOT NULL CHECK (operation_digest ~ '^[0-9a-f]{64}$'),
 intent_digest text NOT NULL CHECK (intent_digest ~ '^[0-9a-f]{64}$'),
 approval_id uuid UNIQUE,
 approver_ref text,
 staff_ref text NOT NULL,
 purpose_code text NOT NULL,
 product text NOT NULL,
 market text NOT NULL,
 zone text NOT NULL,
 organization_ref text NOT NULL,
 tenant_ref text NOT NULL,
 policy_id text NOT NULL,
 policy_version bigint NOT NULL CHECK (policy_version > 0),
 policy_checksum text NOT NULL CHECK (policy_checksum ~ '^[0-9a-f]{64}$'),
 receipt jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((approval_id IS NULL) = (approver_ref IS NULL)),
 CHECK (approver_ref IS NULL OR approver_ref <> staff_ref),
 UNIQUE (owner, subject_kind, subject_id, version_scope, version_key, recorded_version)
);
CREATE INDEX IF NOT EXISTS admin_owner_command_decisions_subject ON admin_owner_command_decisions(owner, subject_kind, subject_id, created_at DESC);
CREATE OR REPLACE FUNCTION reject_admin_owner_command_decision_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Owner administrative command decisions are append-only'; END $$;
DROP TRIGGER IF EXISTS admin_owner_command_decisions_immutable ON admin_owner_command_decisions;
CREATE TRIGGER admin_owner_command_decisions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_command_decisions FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_command_decision_mutation();

-- 6. Admin event outbox (adminevents.OutboxSchema, verbatim).
CREATE TABLE IF NOT EXISTS admin_event_outbox (
 id text PRIMARY KEY,
 event_type text NOT NULL,
 schema_version int NOT NULL CHECK (schema_version > 0),
 aggregate_type text NOT NULL,
 aggregate_id text NOT NULL,
 payload jsonb NOT NULL,
 status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PUBLISHED','DEAD_LETTERED')),
 available_at timestamptz NOT NULL DEFAULT now(),
 attempts int NOT NULL DEFAULT 0 CHECK (attempts >= 0),
 lease_owner text,
 lease_expires_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 published_at timestamptz,
 last_error_redacted text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS admin_event_outbox_due_idx ON admin_event_outbox (status, available_at) WHERE status = 'PENDING';

-- 7. Admin event emission. Every insert or exported-field update of a vendor
-- case appends the exact adminevents SourceEvent in the same transaction.
-- Document references, subject references, remediation text and decider
-- identities are never part of the payload. business_ref is the Vendor's
-- opaque business identifier; a non-opaque identifier is not exported at all
-- rather than exported in a form the Bridge would have to quarantine.
CREATE OR REPLACE FUNCTION verification_emit_vendor_case_admin_event() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
 opaque CONSTANT text := '^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,127}$';
 fmt CONSTANT text := 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"';
 event_id text;
 body jsonb;
BEGIN
 IF TG_OP = 'UPDATE' AND (OLD.status, OLD.source_version, OLD.reason_code, OLD.expires_at, OLD.automation_recommendation, OLD.dimension, OLD.subject_type, OLD.business_id, OLD.updated_at)
    IS NOT DISTINCT FROM (NEW.status, NEW.source_version, NEW.reason_code, NEW.expires_at, NEW.automation_recommendation, NEW.dimension, NEW.subject_type, NEW.business_id, NEW.updated_at) THEN
  RETURN NEW;
 END IF;
 IF NEW.business_id !~ opaque OR NEW.subject_type !~ opaque THEN
  RETURN NEW;
 END IF;
 event_id := gen_random_uuid()::text;
 body := jsonb_build_object(
  'schema', 'karryam.owner-admin-event.v1',
  'event_id', event_id,
  'event_type', 'verification.vendor_case.changed.v1',
  'source_service', 'verification-compliance-service',
  'product', 'karryam',
  'entity_type', 'vendor_verification_case',
  'entity_ref', NEW.id::text,
  'entity_version', NEW.source_version,
  'deleted', false,
  'occurred_at', to_char(now() AT TIME ZONE 'UTC', fmt),
  'emitted_at', to_char(clock_timestamp() AT TIME ZONE 'UTC', fmt),
  'scope', jsonb_build_object('market', '', 'zone', '', 'organization', '', 'tenant', ''),
  'correlation_id', '',
  'payload', jsonb_build_object(
    'status', NEW.status,
    'dimension', NEW.dimension,
    'subject_type', NEW.subject_type,
    'business_ref', NEW.business_id,
    'automation_recommendation', NEW.automation_recommendation,
    'reason_code', CASE WHEN NEW.reason_code ~ opaque THEN NEW.reason_code END,
    'expires_at', CASE WHEN NEW.expires_at IS NOT NULL THEN to_char(NEW.expires_at AT TIME ZONE 'UTC', fmt) END,
    'updated_at', to_char(NEW.updated_at AT TIME ZONE 'UTC', fmt)));
 INSERT INTO admin_event_outbox(id,event_type,schema_version,aggregate_type,aggregate_id,payload,status,available_at,attempts,created_at)
 VALUES (event_id, 'verification.vendor_case.changed.v1', 1, 'vendor_verification_case', NEW.id::text, body, 'PENDING', now(), 0, now());
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS vendor_verification_cases_admin_event ON vendor_verification_cases;
CREATE TRIGGER vendor_verification_cases_admin_event AFTER INSERT OR UPDATE ON vendor_verification_cases
 FOR EACH ROW EXECUTE FUNCTION verification_emit_vendor_case_admin_event();
