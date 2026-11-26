-- Sections 22 and 52: payroll calculation fingerprint, append-only cost
-- allocations and append-only profitability snapshots. Every amount is an
-- integer in minor units with an explicit currency; nothing here aggregates
-- across currencies.

-- The fingerprint of the compensation items a calculation produced. Approval
-- recomputes it from current inputs and refuses on mismatch (FIN-003).
ALTER TABLE fleet_partner_payroll_periods ADD COLUMN calculation_hash text NOT NULL DEFAULT '';

CREATE TABLE fleet_partner_cost_allocations (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 allocation_set_id text NOT NULL,
 source_type text NOT NULL CHECK(source_type IN('EXPENSE')),
 source_id text NOT NULL,
 target_type text NOT NULL CHECK(target_type IN('BRANCH','COST_CENTRE','ASSET','PROVIDER','CUSTOMER','ORDER')),
 target_id text NOT NULL,
 amount_minor bigint NOT NULL CHECK(amount_minor > 0),
 currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 method text NOT NULL CHECK(method IN('MANUAL','EVEN')),
 reason text NOT NULL,
 created_by text NOT NULL,
 occurred_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,source_type,source_id,target_type,target_id)
);
-- One allocation set per source is enforced by the use case under the
-- source expense's row lock; a correction is a reversal of the source, never
-- an edit of its allocation.
CREATE INDEX idx_fleet_cost_allocations_target ON fleet_partner_cost_allocations(organization_id,target_type,target_id,currency);
CREATE INDEX idx_fleet_cost_allocations_source ON fleet_partner_cost_allocations(organization_id,source_type,source_id);
CREATE TRIGGER trg_fleet_cost_allocations_append_only BEFORE UPDATE OR DELETE ON fleet_partner_cost_allocations FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_profitability_snapshots (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 snapshot_set_id text NOT NULL,
 dimension_type text NOT NULL CHECK(dimension_type IN('CUSTOMER','BRANCH','PROVIDER','ASSET','COST_CENTRE')),
 dimension_id text NOT NULL,
 period_start date NOT NULL, period_end date NOT NULL,
 currency text NOT NULL CHECK(currency ~ '^[A-Z]{3}$'),
 revenue_minor bigint NOT NULL, direct_cost_minor bigint NOT NULL, allocated_cost_minor bigint NOT NULL, margin_minor bigint NOT NULL,
 input_trace jsonb NOT NULL,
 model_version text NOT NULL,
 generated_by text NOT NULL, generated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(period_end >= period_start),
 CHECK(margin_minor = revenue_minor - direct_cost_minor - allocated_cost_minor),
 UNIQUE(snapshot_set_id,dimension_id,currency)
);
CREATE INDEX idx_fleet_profitability_snapshots ON fleet_partner_profitability_snapshots(organization_id,dimension_type,period_end DESC);
CREATE TRIGGER trg_fleet_profitability_snapshots_append_only BEFORE UPDATE OR DELETE ON fleet_partner_profitability_snapshots FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('profitability.read','section22','Read profitability analysis and snapshots',true),
('profitability.snapshot','section22','Generate profitability snapshots',true),
('cost_allocations.read','section22','Read cost allocations',true),
('cost_allocations.manage','section22','Allocate approved expenses to cost targets',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code IN('profitability.read','profitability.snapshot','cost_allocations.read','cost_allocations.manage')
WHERE r.id IN('rol_system_owner','rol_system_finance') ON CONFLICT DO NOTHING;
