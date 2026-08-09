-- Section 15 A8: recurring-order templates and durable occurrence generation.
CREATE TABLE IF NOT EXISTS fleet_partner_recurring_order_templates(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 branch_id text NOT NULL REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,customer_id text NOT NULL,
 service_catalog_item_id text NOT NULL,contract_id text NOT NULL DEFAULT '',template_data jsonb NOT NULL,
 recurrence_type text NOT NULL CHECK(recurrence_type IN('DAILY','WEEKLY','MONTHLY','CUSTOM_RRULE')),
 normalized_rrule text NOT NULL DEFAULT '',timezone text NOT NULL,start_date date NOT NULL,end_date date,
 next_run_at timestamptz NOT NULL,missed_run_policy text NOT NULL DEFAULT 'GENERATE_NEXT' CHECK(missed_run_policy IN('GENERATE_NEXT','SKIP')),
 generated_order_initial_state text NOT NULL DEFAULT 'DRAFT' CHECK(generated_order_initial_state IN('DRAFT','AWAITING_APPROVAL')),
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','ACTIVE','INACTIVE','COMPLETED','CANCELLED')),
 version bigint NOT NULL DEFAULT 1,created_by text NOT NULL,updated_by text NOT NULL DEFAULT '',created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),activated_at timestamptz,deactivated_at timestamptz,
 lease_owner text,lease_expires_at timestamptz,last_error_code text NOT NULL DEFAULT '',attempts integer NOT NULL DEFAULT 0,
 UNIQUE(organization_id,id)
);
CREATE INDEX IF NOT EXISTS idx_fleet_recurring_due ON fleet_partner_recurring_order_templates(next_run_at) WHERE status='ACTIVE';
CREATE TABLE IF NOT EXISTS fleet_partner_recurring_occurrences(
 id text PRIMARY KEY,organization_id text NOT NULL,template_id text NOT NULL REFERENCES fleet_partner_recurring_order_templates(id) ON DELETE RESTRICT,
 occurrence_key text NOT NULL,scheduled_for timestamptz NOT NULL,status text NOT NULL CHECK(status IN('CLAIMED','GENERATED','FAILED','DEAD_LETTER')),
 generated_order_id text,attempts integer NOT NULL DEFAULT 0,last_error_code text NOT NULL DEFAULT '',lease_owner text,lease_expires_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(template_id,occurrence_key)
);
CREATE INDEX IF NOT EXISTS idx_fleet_recurring_occurrences ON fleet_partner_recurring_occurrences(organization_id,template_id,scheduled_for DESC);
CREATE TABLE IF NOT EXISTS fleet_partner_recurring_attempts(id text PRIMARY KEY,organization_id text NOT NULL,occurrence_id text NOT NULL REFERENCES fleet_partner_recurring_occurrences(id) ON DELETE RESTRICT,attempt integer NOT NULL,outcome text NOT NULL,safe_error_code text NOT NULL DEFAULT '',occurred_at timestamptz NOT NULL DEFAULT now());
CREATE TRIGGER trg_fleet_recurring_attempts_append_only BEFORE UPDATE OR DELETE ON fleet_partner_recurring_attempts FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
INSERT INTO fleet_partner_permissions(code,category,description,is_active)VALUES('recurring_orders.read','recurring_orders','Read recurring order templates',true),('recurring_orders.manage','recurring_orders','Manage recurring order generation',true)ON CONFLICT(code)DO UPDATE SET category=EXCLUDED.category,description=EXCLUDED.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)SELECT 'rol_system_owner',code FROM fleet_partner_permissions WHERE code LIKE 'recurring_orders.%' ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)SELECT 'rol_system_operations',code FROM fleet_partner_permissions WHERE code LIKE 'recurring_orders.%' ON CONFLICT DO NOTHING;
