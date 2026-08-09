-- Section 22.2: operational expenses, budgets and external claim references.
CREATE TABLE fleet_partner_expenses (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 branch_id text REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 category text NOT NULL, cost_centre text NOT NULL, asset_id text, provider_membership_id text,
 order_id text, shipment_id text, work_order_id text, incident_id text,
 amount_minor bigint NOT NULL CHECK(amount_minor > 0), currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 receipt_media_id text NOT NULL DEFAULT '', description text NOT NULL,
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','SUBMITTED','PENDING_APPROVAL','APPROVED','REJECTED','POSTING','POSTED','REVERSED','CANCELLED')),
 submitted_by text NOT NULL, approved_by text NOT NULL DEFAULT '', approved_at timestamptz, rejected_at timestamptz,
 posting_reference text NOT NULL DEFAULT '', version bigint NOT NULL DEFAULT 1 CHECK(version > 0),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fleet_expenses_scope ON fleet_partner_expenses(organization_id,branch_id,status,cost_centre,category);

CREATE TABLE fleet_partner_expense_history (
 id text PRIMARY KEY, organization_id text NOT NULL, expense_id text NOT NULL REFERENCES fleet_partner_expenses(id) ON DELETE RESTRICT,
 previous_status text NOT NULL, new_status text NOT NULL, snapshot jsonb NOT NULL, actor_reference text NOT NULL,
 reason text NOT NULL DEFAULT '', version bigint NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_expense_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_expense_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_expense_reversals (
 id text PRIMARY KEY, organization_id text NOT NULL, expense_id text NOT NULL REFERENCES fleet_partner_expenses(id) ON DELETE RESTRICT,
 amount_minor bigint NOT NULL CHECK(amount_minor > 0), currency text NOT NULL, reason text NOT NULL, evidence_media_id text NOT NULL DEFAULT '',
 posting_operation_id text REFERENCES fleet_partner_financial_posting_operations(id) ON DELETE RESTRICT,
 created_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(expense_id)
);
CREATE TRIGGER trg_fleet_expense_reversals_append_only BEFORE UPDATE OR DELETE ON fleet_partner_expense_reversals FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_budgets (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 cost_centre text NOT NULL, category text NOT NULL, period_start date NOT NULL, period_end date NOT NULL,
 currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'), approved_amount_minor bigint NOT NULL CHECK(approved_amount_minor >= 0),
 committed_amount_projection bigint NOT NULL DEFAULT 0 CHECK(committed_amount_projection >= 0),
 actual_amount_projection bigint NOT NULL DEFAULT 0 CHECK(actual_amount_projection >= 0),
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','PENDING_APPROVAL','APPROVED','ACTIVE','CLOSED','CANCELLED')),
 approved_by text NOT NULL DEFAULT '', approved_at timestamptz, version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(period_end >= period_start), UNIQUE(organization_id,cost_centre,category,period_start,period_end,currency)
);
CREATE INDEX idx_fleet_budgets_scope ON fleet_partner_budgets(organization_id,status,period_start,period_end);

CREATE TABLE fleet_partner_budget_history (
 id text PRIMARY KEY, organization_id text NOT NULL, budget_id text NOT NULL REFERENCES fleet_partner_budgets(id) ON DELETE RESTRICT,
 previous_status text NOT NULL, new_status text NOT NULL, snapshot jsonb NOT NULL, actor_reference text NOT NULL,
 version bigint NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_budget_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_budget_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_claims (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 claim_type text NOT NULL CHECK(claim_type IN('INSURANCE','CARGO','VEHICLE','PROPERTY','PERSONAL_INJURY','SECURITY','OTHER')),
 incident_id text REFERENCES fleet_partner_incidents(id) ON DELETE RESTRICT, asset_id text, shipment_id text, package_id text,
 policy_reference text NOT NULL DEFAULT '', amount_reference_minor bigint CHECK(amount_reference_minor IS NULL OR amount_reference_minor >= 0),
 currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','SUBMITTED','UNDER_REVIEW','MORE_INFORMATION_REQUIRED','APPROVED','PARTIALLY_APPROVED','REJECTED','SETTLED','CLOSED','CANCELLED')),
 external_claim_reference text NOT NULL DEFAULT '', evidence_media_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
 submitted_at timestamptz, approved_amount_minor bigint CHECK(approved_amount_minor IS NULL OR approved_amount_minor >= 0),
 settlement_reference text NOT NULL DEFAULT '', owner_user_id text NOT NULL DEFAULT '', version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fleet_claims_scope ON fleet_partner_claims(organization_id,status,claim_type);

CREATE TABLE fleet_partner_claim_history (
 id text PRIMARY KEY, organization_id text NOT NULL, claim_id text NOT NULL REFERENCES fleet_partner_claims(id) ON DELETE RESTRICT,
 previous_status text NOT NULL, new_status text NOT NULL, snapshot jsonb NOT NULL, actor_reference text NOT NULL,
 source_type text NOT NULL DEFAULT 'FLEET', source_reference text NOT NULL DEFAULT '', source_version bigint NOT NULL DEFAULT 0,
 occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_claim_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_claim_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('expenses.read','section22','Read operational expenses',true),('expenses.manage','section22','Manage operational expenses',true),
('expenses.approve','section22','Approve operational expenses',true),('expenses.reverse','section22','Reverse posted expenses',true),
('budgets.read','section22','Read company budgets',true),('budgets.manage','section22','Manage draft company budgets',true),
('budgets.approve','section22','Approve company budgets',true),('claims.read','section22','Read claim records',true),
('claims.manage','section22','Manage claim records',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code IN('expenses.read','expenses.manage','expenses.approve','expenses.reverse','budgets.read','budgets.manage','budgets.approve','claims.read','claims.manage')
WHERE r.id IN('rol_system_owner','rol_system_operations','rol_system_finance','rol_system_compliance') ON CONFLICT DO NOTHING;
