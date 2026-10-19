-- Owner-owned review intake, not an account status override or privacy fulfillment.
-- Initial version 1 describes this NEW review aggregate only.
CREATE TABLE IF NOT EXISTS customer_admin_review_state (
 customer_id uuid PRIMARY KEY REFERENCES customers(id) ON DELETE CASCADE,
 version bigint NOT NULL DEFAULT 1 CHECK(version>=1),
 status text NOT NULL DEFAULT 'IDLE' CHECK(status IN ('IDLE','PENDING_REVIEW')),
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO customer_admin_review_state(customer_id) SELECT id FROM customers ON CONFLICT DO NOTHING;
CREATE OR REPLACE FUNCTION initialize_customer_admin_review_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN INSERT INTO customer_admin_review_state(customer_id) VALUES(NEW.id) ON CONFLICT DO NOTHING; RETURN NEW; END $$;
DROP TRIGGER IF EXISTS customer_admin_review_initial_state ON customers;
CREATE TRIGGER customer_admin_review_initial_state AFTER INSERT ON customers FOR EACH ROW EXECUTE FUNCTION initialize_customer_admin_review_state();

CREATE TABLE IF NOT EXISTS customer_admin_review_requests (
 id uuid PRIMARY KEY, customer_id uuid NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
 request_id uuid NOT NULL UNIQUE,idempotency_key text NOT NULL UNIQUE,
 intent_digest text NOT NULL CHECK(intent_digest ~ '^[0-9a-f]{64}$'),
 requested_revision bigint NOT NULL CHECK(requested_revision>=1),recorded_revision bigint NOT NULL CHECK(recorded_revision=requested_revision+1),
 staff_ref text NOT NULL,purpose_code text NOT NULL,reason_code text NOT NULL,
 product text NOT NULL,market text NOT NULL,zone text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL,
 policy_id text NOT NULL,policy_version bigint NOT NULL,policy_checksum text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(customer_id,id)
);
CREATE TABLE IF NOT EXISTS customer_admin_review_work_items (
 request_id uuid PRIMARY KEY REFERENCES customer_admin_review_requests(id) ON DELETE RESTRICT,
 customer_id uuid NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
 status text NOT NULL DEFAULT 'REQUESTED' CHECK(status IN ('REQUESTED','ACKNOWLEDGED','CLOSED')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>=1),
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(customer_id,request_id) REFERENCES customer_admin_review_requests(customer_id,id)
);
CREATE UNIQUE INDEX IF NOT EXISTS customer_admin_review_one_open ON customer_admin_review_work_items(customer_id) WHERE status<>'CLOSED';
CREATE TABLE IF NOT EXISTS customer_admin_review_audit (
 id bigserial PRIMARY KEY,request_id uuid NOT NULL REFERENCES customer_admin_review_requests(id) ON DELETE RESTRICT,
 staff_ref text NOT NULL,purpose_code text NOT NULL,outcome text NOT NULL CHECK(outcome='REVIEW_REQUEST_RECORDED'),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE OR REPLACE FUNCTION reject_customer_admin_review_evidence_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Customer review request evidence is immutable'; END $$;
DROP TRIGGER IF EXISTS customer_admin_review_request_immutable ON customer_admin_review_requests;
CREATE TRIGGER customer_admin_review_request_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON customer_admin_review_requests FOR EACH STATEMENT EXECUTE FUNCTION reject_customer_admin_review_evidence_mutation();
DROP TRIGGER IF EXISTS customer_admin_review_audit_immutable ON customer_admin_review_audit;
CREATE TRIGGER customer_admin_review_audit_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON customer_admin_review_audit FOR EACH STATEMENT EXECUTE FUNCTION reject_customer_admin_review_evidence_mutation();
