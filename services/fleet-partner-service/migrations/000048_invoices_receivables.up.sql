-- Section 22.1: company billing and receivable projections. Fleet owns no wallet or ledger.
CREATE TABLE fleet_partner_invoices (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 branch_id text REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 customer_id text NOT NULL REFERENCES fleet_partner_company_customers(id) ON DELETE RESTRICT,
 contract_id text REFERENCES fleet_partner_contracts(id) ON DELETE RESTRICT,
 quotation_id text REFERENCES fleet_partner_quotations(id) ON DELETE RESTRICT,
 currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 subtotal_minor bigint NOT NULL DEFAULT 0 CHECK(subtotal_minor >= 0),
 tax_total_minor bigint NOT NULL DEFAULT 0 CHECK(tax_total_minor >= 0),
 adjustment_total_minor bigint NOT NULL DEFAULT 0,
 total_minor bigint NOT NULL DEFAULT 0 CHECK(total_minor >= 0),
 due_date date NOT NULL,
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','PENDING_APPROVAL','APPROVED','ISSUED','PARTIALLY_PAID','PAID','OVERDUE','CANCELLED','CREDITED','WRITTEN_OFF')),
 issued_at timestamptz, paid_at timestamptz, cancelled_at timestamptz,
 version bigint NOT NULL DEFAULT 1 CHECK(version > 0),
 created_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(total_minor = subtotal_minor + tax_total_minor + adjustment_total_minor)
);
CREATE INDEX idx_fleet_invoices_scope ON fleet_partner_invoices(organization_id,branch_id,status,due_date);

CREATE TABLE fleet_partner_invoice_lines (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 invoice_id text NOT NULL REFERENCES fleet_partner_invoices(id) ON DELETE RESTRICT,
 source_type text NOT NULL, source_reference text NOT NULL,
 description text NOT NULL,
 quantity numeric(18,4) NOT NULL CHECK(quantity > 0),
 unit_amount_minor bigint NOT NULL CHECK(unit_amount_minor >= 0),
 line_total_minor bigint NOT NULL CHECK(line_total_minor >= 0),
 tax_reference text NOT NULL DEFAULT '', tax_amount_minor bigint NOT NULL DEFAULT 0 CHECK(tax_amount_minor >= 0),
 adjustment_reference text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(invoice_id,source_type,source_reference)
);
CREATE INDEX idx_fleet_invoice_lines_invoice ON fleet_partner_invoice_lines(organization_id,invoice_id);

CREATE TABLE fleet_partner_invoice_snapshots (
 id text PRIMARY KEY, organization_id text NOT NULL, invoice_id text NOT NULL REFERENCES fleet_partner_invoices(id) ON DELETE RESTRICT,
 invoice_version bigint NOT NULL, snapshot jsonb NOT NULL, issued_by text NOT NULL, issued_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(invoice_id,invoice_version)
);
CREATE TRIGGER trg_fleet_invoice_snapshots_append_only BEFORE UPDATE OR DELETE ON fleet_partner_invoice_snapshots FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_credit_notes (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 invoice_id text NOT NULL REFERENCES fleet_partner_invoices(id) ON DELETE RESTRICT,
 currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'), amount_minor bigint NOT NULL CHECK(amount_minor > 0),
 affected_lines jsonb NOT NULL DEFAULT '[]'::jsonb, reason text NOT NULL,
 status text NOT NULL DEFAULT 'ISSUED' CHECK(status IN('ISSUED','POSTING','POSTED','REVERSED')),
 created_by text NOT NULL, version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), posting_reference text NOT NULL DEFAULT ''
);
CREATE TRIGGER trg_fleet_credit_notes_append_only BEFORE UPDATE OR DELETE ON fleet_partner_credit_notes FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_receivable_entries (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 invoice_id text NOT NULL UNIQUE REFERENCES fleet_partner_invoices(id) ON DELETE RESTRICT,
 entry_type text NOT NULL DEFAULT 'INVOICE', original_amount_minor bigint NOT NULL CHECK(original_amount_minor >= 0),
 paid_amount_minor bigint NOT NULL DEFAULT 0 CHECK(paid_amount_minor >= 0), outstanding_amount_minor bigint NOT NULL CHECK(outstanding_amount_minor >= 0),
 currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'), due_at timestamptz NOT NULL,
 status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','PARTIALLY_PAID','PAID','OVERDUE','DISPUTED','WRITTEN_OFF','REVERSED')),
 payment_reference text NOT NULL DEFAULT '', wallet_operation_reference text NOT NULL DEFAULT '', accounting_reference text NOT NULL DEFAULT '',
 source_version bigint NOT NULL DEFAULT 0 CHECK(source_version >= 0),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(paid_amount_minor + outstanding_amount_minor = original_amount_minor)
);
CREATE INDEX idx_fleet_receivables_scope ON fleet_partner_receivable_entries(organization_id,status,due_at);

CREATE TABLE fleet_partner_receivable_payment_history (
 id text PRIMARY KEY, organization_id text NOT NULL, receivable_id text NOT NULL REFERENCES fleet_partner_receivable_entries(id) ON DELETE RESTRICT,
 source_event_id text NOT NULL, source_version bigint NOT NULL, amount_minor bigint NOT NULL CHECK(amount_minor > 0), currency text NOT NULL,
 payment_reference text NOT NULL, wallet_operation_reference text NOT NULL DEFAULT '', received_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,source_event_id)
);
CREATE TRIGGER trg_fleet_receivable_payment_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_receivable_payment_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_financial_posting_operations (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 record_type text NOT NULL, record_id text NOT NULL, operation_type text NOT NULL,
 idempotency_key text NOT NULL, request_fingerprint text NOT NULL,
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','RUNNING','OUTCOME_UNKNOWN','CONFIRMED','RETRY_SCHEDULED','FAILED','DEAD_LETTERED','REVERSED')),
 external_operation_reference text NOT NULL DEFAULT '', source_version bigint NOT NULL DEFAULT 0,
 attempts integer NOT NULL DEFAULT 0, next_attempt_at timestamptz NOT NULL DEFAULT now(), last_error_code text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,operation_type,idempotency_key)
);
CREATE INDEX idx_fleet_financial_posting_due ON fleet_partner_financial_posting_operations(status,next_attempt_at) WHERE status IN('PENDING','OUTCOME_UNKNOWN','RETRY_SCHEDULED');

CREATE TABLE fleet_partner_financial_posting_attempts (
 id text PRIMARY KEY, organization_id text NOT NULL, operation_id text NOT NULL REFERENCES fleet_partner_financial_posting_operations(id) ON DELETE RESTRICT,
 attempt_number integer NOT NULL CHECK(attempt_number > 0), outcome text NOT NULL, external_reference text NOT NULL DEFAULT '',
 error_code text NOT NULL DEFAULT '', started_at timestamptz NOT NULL, completed_at timestamptz NOT NULL, UNIQUE(operation_id,attempt_number)
);
CREATE TRIGGER trg_fleet_financial_posting_attempts_append_only BEFORE UPDATE OR DELETE ON fleet_partner_financial_posting_attempts FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('invoices.read','section22','Read company invoices',true),('invoices.manage','section22','Manage draft company invoices',true),
('invoices.approve','section22','Approve company invoices',true),('invoices.issue','section22','Issue immutable company invoices',true),
('invoices.credit','section22','Issue invoice credit notes',true),('receivables.read','section22','Read receivable projections',true),
('receivables.reconcile','section22','Reconcile authoritative receivable payments',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code IN('invoices.read','invoices.manage','invoices.approve','invoices.issue','invoices.credit','receivables.read','receivables.reconcile')
WHERE r.id IN('rol_system_owner','rol_system_operations','rol_system_finance') ON CONFLICT DO NOTHING;
