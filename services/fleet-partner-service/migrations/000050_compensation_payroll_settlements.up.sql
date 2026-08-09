-- Section 22.3: explainable compensation, payroll read models and subcontractor settlements.
CREATE TABLE fleet_partner_compensation_rules (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 name text NOT NULL, rule_type text NOT NULL CHECK(rule_type IN('SALARY','PER_JOB','PER_SHIFT','COMMISSION','ALLOWANCE','OVERTIME','BONUS','MANUAL_ADJUSTMENT','DEDUCTION')),
 applicable_roles jsonb NOT NULL DEFAULT '[]'::jsonb, applicable_services jsonb NOT NULL DEFAULT '[]'::jsonb,
 calculation_configuration jsonb NOT NULL, currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 effective_from date NOT NULL, effective_until date,
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','PENDING_APPROVAL','APPROVED','ACTIVE','INACTIVE','ARCHIVED')),
 version bigint NOT NULL DEFAULT 1, approved_by text NOT NULL DEFAULT '', created_by text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(effective_until IS NULL OR effective_until >= effective_from),
 CHECK(rule_type <> 'DEDUCTION' OR (calculation_configuration ? 'approvedBasis' AND length(calculation_configuration->>'approvedBasis') > 0))
);
CREATE INDEX idx_fleet_compensation_rules_scope ON fleet_partner_compensation_rules(organization_id,status,effective_from,effective_until);

CREATE TABLE fleet_partner_compensation_rule_versions (
 id text PRIMARY KEY, organization_id text NOT NULL, rule_id text NOT NULL REFERENCES fleet_partner_compensation_rules(id) ON DELETE RESTRICT,
 rule_version bigint NOT NULL, snapshot jsonb NOT NULL, actor_reference text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(rule_id,rule_version)
);
CREATE TRIGGER trg_fleet_comp_rule_versions_append_only BEFORE UPDATE OR DELETE ON fleet_partner_compensation_rule_versions FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_payroll_periods (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 branch_id text REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 period_start date NOT NULL, period_end date NOT NULL, currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','CALCULATING','CALCULATED','PENDING_APPROVAL','APPROVED','POSTING','POSTED','PARTIALLY_POSTED','FAILED','REVERSED','CLOSED')),
 calculation_version bigint NOT NULL DEFAULT 0, calculated_at timestamptz, approved_by text NOT NULL DEFAULT '', approved_at timestamptz,
 posted_at timestamptz, export_reference text NOT NULL DEFAULT '', wallet_operation_reference text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1, created_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(period_end >= period_start), UNIQUE(organization_id,branch_id,period_start,period_end,currency)
);
CREATE INDEX idx_fleet_payroll_periods_scope ON fleet_partner_payroll_periods(organization_id,branch_id,status,period_start);

CREATE TABLE fleet_partner_provider_compensation_items (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 payroll_period_id text NOT NULL REFERENCES fleet_partner_payroll_periods(id) ON DELETE RESTRICT,
 membership_id text NOT NULL REFERENCES fleet_partner_organization_provider_memberships(id) ON DELETE RESTRICT,
 source_type text NOT NULL, source_reference text NOT NULL,
 rule_id text NOT NULL REFERENCES fleet_partner_compensation_rules(id) ON DELETE RESTRICT, rule_version bigint NOT NULL,
 calculation_version bigint NOT NULL, amount_minor bigint NOT NULL CHECK(amount_minor <> 0), currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 reason_code text NOT NULL, explanation text NOT NULL, source_inputs jsonb NOT NULL,
 status text NOT NULL DEFAULT 'CALCULATED' CHECK(status IN('CALCULATED','REVIEW_REQUIRED','APPROVED','POSTING','POSTED','REVERSED')),
 adjustment_of text REFERENCES fleet_partner_provider_compensation_items(id) ON DELETE RESTRICT,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(payroll_period_id,membership_id,source_type,source_reference,rule_id,calculation_version),
 CHECK(amount_minor > 0 OR source_type NOT IN('DECLINE','DECLINE_COUNT','DECLINE_RATE','SAFETY_REPORT','SAFETY_DECLINE','PERFORMANCE_SCORE'))
);
CREATE INDEX idx_fleet_compensation_items_statement ON fleet_partner_provider_compensation_items(organization_id,membership_id,payroll_period_id,status);

CREATE TABLE fleet_partner_payroll_history (
 id text PRIMARY KEY, organization_id text NOT NULL, payroll_period_id text NOT NULL REFERENCES fleet_partner_payroll_periods(id) ON DELETE RESTRICT,
 previous_status text NOT NULL, new_status text NOT NULL, calculation_version bigint NOT NULL,
 snapshot jsonb NOT NULL, actor_reference text NOT NULL, reason text NOT NULL DEFAULT '', occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_payroll_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_payroll_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_subcontractor_settlements (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 supplier_reference text NOT NULL, provider_or_carrier_reference text NOT NULL,
 period_start date NOT NULL, period_end date NOT NULL, currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'), amount_minor bigint NOT NULL CHECK(amount_minor >= 0),
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','CALCULATED','PENDING_APPROVAL','APPROVED','POSTING','POSTED','DISPUTED','REVERSED','CANCELLED')),
 approved_by text NOT NULL DEFAULT '', posting_reference text NOT NULL DEFAULT '', payment_reference text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1, created_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(period_end >= period_start)
);
CREATE INDEX idx_fleet_subcontractor_settlements_scope ON fleet_partner_subcontractor_settlements(organization_id,status,period_start);

CREATE TABLE fleet_partner_subcontractor_settlement_jobs (
 settlement_id text NOT NULL REFERENCES fleet_partner_subcontractor_settlements(id) ON DELETE RESTRICT,
 organization_id text NOT NULL, job_reference text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(settlement_id,job_reference), UNIQUE(organization_id,job_reference)
);
CREATE TRIGGER trg_fleet_settlement_jobs_append_only BEFORE UPDATE OR DELETE ON fleet_partner_subcontractor_settlement_jobs FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('compensation.read','section22','Read compensation rules and items',true),('compensation.manage','section22','Manage compensation rules',true),
('compensation.approve','section22','Approve compensation policies',true),('payroll.read','section22','Read payroll periods and statements',true),
('payroll.manage','section22','Manage payroll periods',true),('payroll.calculate','section22','Calculate payroll periods',true),
('payroll.approve','section22','Approve payroll periods',true),('payroll.post','section22','Post approved payroll',true),
('payroll.reverse','section22','Reverse posted payroll',true),('subcontractor_settlements.read','section22','Read subcontractor settlements',true),
('subcontractor_settlements.manage','section22','Manage subcontractor settlements',true),('subcontractor_settlements.approve','section22','Approve subcontractor settlements',true),
('subcontractor_settlements.post','section22','Post subcontractor settlements',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code IN('compensation.read','compensation.manage','compensation.approve','payroll.read','payroll.manage','payroll.calculate','payroll.approve','payroll.post','payroll.reverse','subcontractor_settlements.read','subcontractor_settlements.manage','subcontractor_settlements.approve','subcontractor_settlements.post')
WHERE r.id IN('rol_system_owner','rol_system_operations','rol_system_finance') ON CONFLICT DO NOTHING;
