-- Dispatch administrative intervention heads and append-only intervention
-- records. A head is the per-subject version fence the command contract
-- compares with expected_version; every accepted command appends exactly one
-- record bound to the ledger's decision identity. The domain effect itself
-- lives in the rows the recovery engines already consult (incidents, return
-- workflows, recovery actions); these tables never drive an engine.
CREATE TABLE IF NOT EXISTS dispatch_admin_intervention_heads (
 subject_kind text NOT NULL CHECK (subject_kind IN ('incidents','missions','returns','delivery-recoveries')),
 subject_id text NOT NULL,
 version bigint NOT NULL CHECK (version > 0),
 state text NOT NULL CHECK (state ~ '^[A-Z][A-Z0-9_]{0,63}$'),
 last_action text NOT NULL,
 last_decision_id uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (subject_kind, subject_id)
);
CREATE OR REPLACE FUNCTION dispatch_admin_intervention_head_fence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP <> 'UPDATE' THEN RAISE EXCEPTION 'Dispatch intervention heads cannot be removed'; END IF;
 IF NEW.subject_kind <> OLD.subject_kind OR NEW.subject_id <> OLD.subject_id OR NEW.version <> OLD.version + 1 OR NEW.created_at <> OLD.created_at THEN
  RAISE EXCEPTION 'Dispatch intervention head may only advance by exactly one version';
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS dispatch_admin_intervention_head_fence ON dispatch_admin_intervention_heads;
CREATE TRIGGER dispatch_admin_intervention_head_fence BEFORE UPDATE OR DELETE ON dispatch_admin_intervention_heads FOR EACH ROW EXECUTE FUNCTION dispatch_admin_intervention_head_fence();
CREATE OR REPLACE FUNCTION reject_dispatch_admin_intervention_truncate() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Dispatch intervention evidence cannot be truncated'; END $$;
DROP TRIGGER IF EXISTS dispatch_admin_intervention_heads_truncate ON dispatch_admin_intervention_heads;
CREATE TRIGGER dispatch_admin_intervention_heads_truncate BEFORE TRUNCATE ON dispatch_admin_intervention_heads FOR EACH STATEMENT EXECUTE FUNCTION reject_dispatch_admin_intervention_truncate();

CREATE TABLE IF NOT EXISTS dispatch_admin_intervention_records (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 decision_id uuid NOT NULL UNIQUE,
 subject_kind text NOT NULL,
 subject_id text NOT NULL,
 action text NOT NULL CHECK (action IN ('RETRY_INCIDENT_RECOVERY','ESCALATE_INCIDENT_RECOVERY','REQUEST_REASSIGNMENT','RETRY_RETURN_ASSIGNMENT','REQUEST_DELIVERY_REPLAN')),
 recorded_version bigint NOT NULL CHECK (recorded_version > 0),
 prior_state text NOT NULL CHECK (prior_state ~ '^[A-Z][A-Z0-9_]{0,63}$'),
 resulting_state text NOT NULL CHECK (resulting_state ~ '^[A-Z][A-Z0-9_]{0,63}$'),
 outcome text NOT NULL CHECK (outcome ~ '^[A-Z][A-Z0-9_]{0,63}$'),
 gate text NOT NULL DEFAULT 'NONE' CHECK (gate IN ('NONE','AWAITING_CUSTOMER_APPROVAL')),
 reason_code text NOT NULL,
 params jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(params)='object'),
 effect_ref text NOT NULL DEFAULT '',
 effect jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(effect)='object'),
 recorded_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (subject_kind, subject_id, recorded_version),
 FOREIGN KEY (subject_kind, subject_id) REFERENCES dispatch_admin_intervention_heads(subject_kind, subject_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS dispatch_admin_intervention_records_subject ON dispatch_admin_intervention_records(subject_kind, subject_id, recorded_version DESC);
CREATE OR REPLACE FUNCTION reject_dispatch_admin_intervention_record_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Dispatch intervention records are append-only'; END $$;
DROP TRIGGER IF EXISTS dispatch_admin_intervention_records_immutable ON dispatch_admin_intervention_records;
CREATE TRIGGER dispatch_admin_intervention_records_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON dispatch_admin_intervention_records FOR EACH STATEMENT EXECUTE FUNCTION reject_dispatch_admin_intervention_record_mutation();
