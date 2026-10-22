-- Bridge orchestration metadata only. Customer remains the sole business writer.
CREATE TABLE admin_customer_work_item_operations (
 id uuid PRIMARY KEY,
 principal text NOT NULL CHECK(principal='cosmicforge-hq-karryam-admin'),
 staff_ref text NOT NULL,
 purpose_code text NOT NULL,
 product text NOT NULL CHECK(product='karryam'),
 market text NOT NULL,
 zone text NOT NULL,
 organization_ref text NOT NULL,
 tenant_ref text NOT NULL,
 customer_id uuid NOT NULL,
 review_request_id uuid NOT NULL,
 action text NOT NULL CHECK(action IN ('ACKNOWLEDGE','CLOSE')),
 idempotency_key text NOT NULL UNIQUE,
 intent_digest text NOT NULL CHECK(intent_digest ~ '^[0-9a-f]{64}$'),
 expected_work_item_version bigint NOT NULL CHECK(expected_work_item_version>0),
 status text NOT NULL CHECK(status IN ('EXECUTING','UNKNOWN_OUTCOME','CONFIRMED')),
 claim_token uuid NOT NULL,
 receipt jsonb,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK ((status='CONFIRMED')=(receipt IS NOT NULL)),
 UNIQUE(customer_id,review_request_id,expected_work_item_version)
);
CREATE TABLE admin_customer_work_item_operation_audit (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 operation_id uuid NOT NULL REFERENCES admin_customer_work_item_operations(id),
 staff_ref text NOT NULL,
 purpose_code text NOT NULL,
 product text NOT NULL CHECK(product='karryam'),
 market text NOT NULL,
 zone text NOT NULL,
 organization_ref text NOT NULL,
 tenant_ref text NOT NULL,
 policy_id text NOT NULL,
 policy_version bigint NOT NULL CHECK(policy_version>0),
 policy_checksum text NOT NULL CHECK(policy_checksum ~ '^[0-9a-f]{64}$'),
 outcome text NOT NULL CHECK(outcome IN ('EXECUTING','UNKNOWN_OUTCOME','CONFIRMED')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER admin_customer_work_item_audit_immutable BEFORE UPDATE OR DELETE
 ON admin_customer_work_item_operation_audit FOR EACH ROW EXECUTE FUNCTION admin_immutable_evidence();
CREATE TRIGGER admin_customer_work_item_audit_no_truncate BEFORE TRUNCATE
 ON admin_customer_work_item_operation_audit FOR EACH STATEMENT EXECUTE FUNCTION admin_immutable_evidence();
CREATE FUNCTION admin_customer_work_item_operation_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP <> 'UPDATE' THEN RAISE EXCEPTION 'review operation identity is durable'; END IF;
 IF (to_jsonb(OLD)-ARRAY['status','receipt','updated_at']) IS DISTINCT FROM
    (to_jsonb(NEW)-ARRAY['status','receipt','updated_at']) THEN
  RAISE EXCEPTION 'review operation identity is immutable';
 END IF;
 IF OLD.status='CONFIRMED' AND (NEW.status<>OLD.status OR NEW.receipt IS DISTINCT FROM OLD.receipt) THEN
  RAISE EXCEPTION 'confirmed review operation is immutable';
 END IF;
 IF OLD.status='UNKNOWN_OUTCOME' AND NEW.status='EXECUTING' THEN
  RAISE EXCEPTION 'uncertain review operation cannot be retried';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER admin_customer_work_item_operation_guard BEFORE UPDATE OR DELETE
 ON admin_customer_work_item_operations FOR EACH ROW EXECUTE FUNCTION admin_customer_work_item_operation_guard();
CREATE TRIGGER admin_customer_work_item_operation_no_truncate BEFORE TRUNCATE
 ON admin_customer_work_item_operations FOR EACH STATEMENT EXECUTE FUNCTION admin_customer_work_item_operation_guard();
REVOKE ALL ON admin_customer_work_item_operations,admin_customer_work_item_operation_audit FROM PUBLIC;
