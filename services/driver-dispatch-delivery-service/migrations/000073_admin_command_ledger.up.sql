-- Source-owned administrative command decision ledger (Dispatch). No production grants.
-- shared-template-sha256: 7d18d98288fbcca513090f63fbd9c2adb0245afc849c89a978911666d04ceb55
-- Source-owned administrative command decision ledger. Install only through
-- the consuming owner's normal migration runner; each owner keeps its own
-- immutable copy in its own database. The Admin Bridge never connects here.
CREATE TABLE IF NOT EXISTS admin_owner_command_ledger (
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
CREATE INDEX IF NOT EXISTS admin_owner_command_ledger_subject ON admin_owner_command_ledger(owner, subject_kind, subject_id, created_at DESC);
CREATE OR REPLACE FUNCTION reject_admin_owner_command_ledger_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Owner administrative command ledger is append-only'; END $$;
DROP TRIGGER IF EXISTS admin_owner_command_ledger_immutable ON admin_owner_command_ledger;
CREATE TRIGGER admin_owner_command_ledger_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON admin_owner_command_ledger FOR EACH STATEMENT EXECUTE FUNCTION reject_admin_owner_command_ledger_mutation();
