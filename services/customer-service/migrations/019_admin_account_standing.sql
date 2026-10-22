-- Customer-owned administrative account standing, privacy request disposition
-- and support decision records. Every change arrives only through the closed
-- customer.admin-command.v1 dictionary (shared/go/admincommand) inside the
-- same transaction as its immutable ledger row. This service re-applies every
-- migration on each MIGRATION=true boot, so every statement is idempotent.

-- ─── Account standing aggregate (one row per customer, created lazily) ─────
-- Absence of a row means version 0, no restriction and no remediation.
CREATE TABLE IF NOT EXISTS customer_account_standing (
 customer_id uuid PRIMARY KEY REFERENCES customers(id) ON DELETE RESTRICT,
 version bigint NOT NULL CHECK(version>=1),
 restriction text NOT NULL DEFAULT 'NONE' CHECK(restriction IN ('NONE','LOGIN_BLOCKED','BOOKING_HOLD')),
 remediation text NOT NULL DEFAULT 'NONE' CHECK(remediation IN ('NONE','REVERIFY_CONTACT','RESET_SESSIONS','PROFILE_CORRECTION')),
 remediation_required_at timestamptz,
 state text NOT NULL CHECK(state ~ '^[A-Z][A-Z0-9_]{0,63}$'),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK((remediation='NONE')=(remediation_required_at IS NULL))
);
CREATE TABLE IF NOT EXISTS customer_account_standing_history (
 id bigserial PRIMARY KEY,
 customer_id uuid NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
 decision_id uuid NOT NULL UNIQUE,
 action text NOT NULL,
 reason_code text NOT NULL,
 prior_version bigint NOT NULL CHECK(prior_version>=0),
 recorded_version bigint NOT NULL CHECK(recorded_version=prior_version+1),
 prior_state text NOT NULL,
 resulting_state text NOT NULL,
 restriction text NOT NULL,
 remediation text NOT NULL,
 evidence_ref text,
 sessions_revoked bigint NOT NULL DEFAULT 0 CHECK(sessions_revoked>=0),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(customer_id,recorded_version)
);

-- ─── Privacy request disposition and fulfilment tracking (per case) ────────
-- Fulfilment covers customer-service records only; other owners' data needs
-- each owner's own authorized contract. Nothing is deleted here.
CREATE TABLE IF NOT EXISTS customer_privacy_requests (
 case_ref uuid PRIMARY KEY,
 customer_id uuid NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
 request_type text NOT NULL CHECK(request_type IN ('ACCESS_EXPORT','ERASURE','RECTIFICATION','CONSENT_WITHDRAWAL','RESTRICT_PROCESSING')),
 channel text NOT NULL CHECK(channel IN ('IN_APP','EMAIL','SUPPORT','REGULATOR')),
 received_on date NOT NULL,
 due_on date CHECK(due_on IS NULL OR due_on>=received_on),
 state text NOT NULL CHECK(state IN ('RECEIVED','IN_PROGRESS','FULFILLED','REJECTED')),
 version bigint NOT NULL CHECK(version>=1),
 evidence_ref text,
 rejection text CHECK(rejection IS NULL OR rejection IN ('IDENTITY_UNVERIFIED','LEGAL_HOLD','DUPLICATE','OUT_OF_SCOPE')),
 fulfilment_scope text CHECK(fulfilment_scope IS NULL OR fulfilment_scope='CUSTOMER_SERVICE_RECORDS_ONLY'),
 consents_withdrawn bigint NOT NULL DEFAULT 0 CHECK(consents_withdrawn>=0),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK((state='FULFILLED')=(evidence_ref IS NOT NULL AND fulfilment_scope IS NOT NULL)),
 CHECK((state='REJECTED')=(rejection IS NOT NULL)),
 CHECK(state IN ('RECEIVED','REJECTED') OR due_on IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS customer_privacy_requests_customer_idx ON customer_privacy_requests(customer_id,created_at);
CREATE TABLE IF NOT EXISTS customer_privacy_request_transitions (
 id bigserial PRIMARY KEY,
 case_ref uuid NOT NULL REFERENCES customer_privacy_requests(case_ref) ON DELETE RESTRICT,
 customer_id uuid NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
 decision_id uuid NOT NULL UNIQUE,
 action text NOT NULL,
 reason_code text NOT NULL,
 prior_version bigint NOT NULL CHECK(prior_version>=0),
 recorded_version bigint NOT NULL CHECK(recorded_version=prior_version+1),
 prior_state text NOT NULL,
 resulting_state text NOT NULL,
 detail jsonb NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(detail)='object'),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(case_ref,recorded_version)
);

-- ─── Support-related account decisions (per support case reference) ───────
CREATE TABLE IF NOT EXISTS customer_support_decision_cases (
 case_ref text PRIMARY KEY CHECK(length(case_ref) BETWEEN 1 AND 128),
 customer_id uuid NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
 version bigint NOT NULL CHECK(version>=1),
 decision text NOT NULL CHECK(decision IN ('ACCOUNT_STANDING_CONFIRMED','GOODWILL_ACKNOWLEDGED','ESCALATED_TO_RESTRICTION_REVIEW','NO_ACCOUNT_ACTION')),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS customer_support_decision_cases_customer_idx ON customer_support_decision_cases(customer_id,created_at);
CREATE TABLE IF NOT EXISTS customer_support_decisions (
 id bigserial PRIMARY KEY,
 case_ref text NOT NULL REFERENCES customer_support_decision_cases(case_ref) ON DELETE RESTRICT,
 customer_id uuid NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
 decision_id uuid NOT NULL UNIQUE,
 reason_code text NOT NULL,
 prior_version bigint NOT NULL CHECK(prior_version>=0),
 recorded_version bigint NOT NULL CHECK(recorded_version=prior_version+1),
 prior_decision text NOT NULL,
 decision text NOT NULL,
 note text CHECK(note IS NULL OR length(note)<=280),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(case_ref,recorded_version)
);

-- ─── Immutable decision evidence ───────────────────────────────────────────
CREATE OR REPLACE FUNCTION reject_customer_admin_command_evidence_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Customer administrative decision history is append-only'; END $$;
DROP TRIGGER IF EXISTS customer_account_standing_history_immutable ON customer_account_standing_history;
CREATE TRIGGER customer_account_standing_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON customer_account_standing_history FOR EACH STATEMENT EXECUTE FUNCTION reject_customer_admin_command_evidence_mutation();
DROP TRIGGER IF EXISTS customer_privacy_request_transitions_immutable ON customer_privacy_request_transitions;
CREATE TRIGGER customer_privacy_request_transitions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON customer_privacy_request_transitions FOR EACH STATEMENT EXECUTE FUNCTION reject_customer_admin_command_evidence_mutation();
DROP TRIGGER IF EXISTS customer_support_decisions_immutable ON customer_support_decisions;
CREATE TRIGGER customer_support_decisions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON customer_support_decisions FOR EACH STATEMENT EXECUTE FUNCTION reject_customer_admin_command_evidence_mutation();

-- ─── Login block guard on refresh-session creation ─────────────────────────
-- The auth usecase refuses a login-blocked customer with a clear error. This
-- guard closes the race with a concurrent restriction: the restriction holds
-- the customer row FOR UPDATE while it revokes sessions, so FOR SHARE here
-- waits for it and the following statement sees the committed standing.
CREATE OR REPLACE FUNCTION guard_customer_session_account_standing() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM 1 FROM customers WHERE id=NEW.customer_id FOR SHARE;
 IF EXISTS (SELECT 1 FROM customer_account_standing WHERE customer_id=NEW.customer_id AND restriction='LOGIN_BLOCKED') THEN
  RAISE EXCEPTION 'customer account is login blocked' USING ERRCODE='KG403';
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS customer_sessions_account_standing_guard ON customer_sessions;
CREATE TRIGGER customer_sessions_account_standing_guard BEFORE INSERT ON customer_sessions FOR EACH ROW EXECUTE FUNCTION guard_customer_session_account_standing();
