-- Owner administrative command decision ledger (shared/go/admincommand/schema.sql, verbatim).
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
