-- Queue lifecycle only. No account, privacy, consent or financial decision.
CREATE TABLE IF NOT EXISTS customer_admin_review_transitions (
 idempotency_key text PRIMARY KEY,
 operation_id uuid NOT NULL UNIQUE,
 request_id uuid NOT NULL REFERENCES customer_admin_review_work_items(request_id) ON DELETE RESTRICT,
 customer_id uuid NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
 intent_digest text NOT NULL CHECK(intent_digest ~ '^[0-9a-f]{64}$'),
 staff_ref text NOT NULL,purpose_code text NOT NULL,
 product text NOT NULL,market text NOT NULL,zone text NOT NULL,organization_ref text NOT NULL,tenant_ref text NOT NULL,
 policy_id text NOT NULL,policy_version bigint NOT NULL,policy_checksum text NOT NULL,
 receipt jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(customer_id,request_id) REFERENCES customer_admin_review_requests(customer_id,id)
);
DROP TRIGGER IF EXISTS customer_admin_review_transitions_immutable ON customer_admin_review_transitions;
CREATE TRIGGER customer_admin_review_transitions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON customer_admin_review_transitions FOR EACH STATEMENT EXECUTE FUNCTION reject_customer_admin_review_evidence_mutation();
