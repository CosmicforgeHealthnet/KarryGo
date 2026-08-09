-- Section 23.2: authoritative support projections, internal quality, SLA snapshots and escalations.
CREATE TABLE fleet_partner_support_case_projections (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 support_service_case_id text NOT NULL, case_type text NOT NULL, status text NOT NULL, priority text NOT NULL,
 customer_reference text NOT NULL DEFAULT '', provider_reference text NOT NULL DEFAULT '', trip_reference text NOT NULL DEFAULT '',
 order_reference text NOT NULL DEFAULT '', assignment_reference text NOT NULL DEFAULT '', owner_reference text NOT NULL DEFAULT '',
 source_version bigint NOT NULL CHECK(source_version >= 0), last_synced_at timestamptz NOT NULL,
 gap_status text NOT NULL DEFAULT 'CURRENT' CHECK(gap_status IN('CURRENT','GAP_DETECTED','RECONCILING','CONFLICT','UNKNOWN')),
 create_idempotency_key text NOT NULL DEFAULT '', version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,support_service_case_id)
);
CREATE INDEX idx_fleet_support_projections_scope ON fleet_partner_support_case_projections(organization_id,status,priority,gap_status);
CREATE UNIQUE INDEX uq_fleet_support_projection_create_key ON fleet_partner_support_case_projections(organization_id,create_idempotency_key) WHERE create_idempotency_key<>'';

CREATE TABLE fleet_partner_support_case_events_inbox (
 id text PRIMARY KEY, organization_id text NOT NULL, event_id text NOT NULL, support_service_case_id text NOT NULL,
 source_version bigint NOT NULL, event_type text NOT NULL, safe_payload jsonb NOT NULL,
 status text NOT NULL CHECK(status IN('APPLIED','DUPLICATE','GAP_DETECTED','CONFLICT','REJECTED')),
 received_at timestamptz NOT NULL DEFAULT now(), UNIQUE(event_id)
);
CREATE TRIGGER trg_fleet_support_events_append_only BEFORE UPDATE OR DELETE ON fleet_partner_support_case_events_inbox FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_quality_cases (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 branch_id text REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 category text NOT NULL, severity text NOT NULL CHECK(severity IN('LOW','MEDIUM','HIGH','CRITICAL')),
 source_type text NOT NULL, source_reference text NOT NULL, customer_reference text NOT NULL DEFAULT '', provider_reference text NOT NULL DEFAULT '',
 owner_user_id text NOT NULL DEFAULT '', status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','UNDER_REVIEW','INVESTIGATING','ACTION_REQUIRED','RESOLVED','CLOSED','CANCELLED')),
 root_cause text NOT NULL DEFAULT '', corrective_action_summary text NOT NULL DEFAULT '', resolved_by text NOT NULL DEFAULT '', resolved_at timestamptz,
 version bigint NOT NULL DEFAULT 1, created_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,source_type,source_reference,category)
);
CREATE INDEX idx_fleet_quality_cases_scope ON fleet_partner_quality_cases(organization_id,branch_id,status,severity);

CREATE TABLE fleet_partner_quality_case_history (
 id text PRIMARY KEY, organization_id text NOT NULL, quality_case_id text NOT NULL REFERENCES fleet_partner_quality_cases(id) ON DELETE RESTRICT,
 previous_status text NOT NULL, new_status text NOT NULL, root_cause text NOT NULL DEFAULT '', corrective_action_summary text NOT NULL DEFAULT '',
 actor_reference text NOT NULL, version bigint NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_quality_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_quality_case_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_sla_measurements (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 agreement_reference text NOT NULL, contract_id text REFERENCES fleet_partner_contracts(id) ON DELETE RESTRICT,
 customer_id text REFERENCES fleet_partner_company_customers(id) ON DELETE RESTRICT,
 metric_code text NOT NULL, formula_version bigint NOT NULL, period_start timestamptz NOT NULL, period_end timestamptz NOT NULL,
 target_value numeric(20,6) NOT NULL, measured_value numeric(20,6), unit text NOT NULL,
 breach_status text NOT NULL CHECK(breach_status IN('MET','AT_RISK','BREACHED','UNKNOWN')),
 source_inputs jsonb NOT NULL, source_versions jsonb NOT NULL, generated_at timestamptz NOT NULL,
 version bigint NOT NULL DEFAULT 1, UNIQUE(organization_id,agreement_reference,metric_code,period_start,period_end,formula_version),
 CHECK(period_end > period_start)
);
CREATE INDEX idx_fleet_sla_measurements_scope ON fleet_partner_sla_measurements(organization_id,breach_status,period_end);
CREATE TRIGGER trg_fleet_sla_measurements_append_only BEFORE UPDATE OR DELETE ON fleet_partner_sla_measurements FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_escalations (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 source_type text NOT NULL, source_reference text NOT NULL, escalation_level integer NOT NULL CHECK(escalation_level > 0),
 recipient_type text NOT NULL, recipient_reference text NOT NULL, due_at timestamptz NOT NULL,
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','NOTIFIED','ACKNOWLEDGED','RESOLVED','EXPIRED','CANCELLED')),
 acknowledged_at timestamptz, resolved_at timestamptz, notification_operation_reference text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,source_type,source_reference,escalation_level)
);
CREATE INDEX idx_fleet_escalations_due ON fleet_partner_escalations(organization_id,status,due_at);

CREATE TABLE fleet_partner_escalation_history (
 id text PRIMARY KEY, organization_id text NOT NULL, escalation_id text NOT NULL REFERENCES fleet_partner_escalations(id) ON DELETE RESTRICT,
 previous_status text NOT NULL, new_status text NOT NULL, actor_reference text NOT NULL, version bigint NOT NULL,
 notification_operation_reference text NOT NULL DEFAULT '', occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_escalation_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_escalation_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('support.read','section23','Read organization support projections',true),('support.create','section23','Create canonical platform support cases',true),
('quality.read','section23','Read internal quality cases',true),('quality.manage','section23','Manage internal quality cases',true),
('quality.resolve','section23','Resolve internal quality cases',true),('sla.read','section23','Read SLA measurements',true),
('escalations.read','section23','Read escalations',true),('escalations.acknowledge','section23','Acknowledge escalations',true),
('escalations.resolve','section23','Resolve escalations',true),('escalations.replay','section23','Replay escalation delivery',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code IN('support.read','support.create','quality.read','quality.manage','quality.resolve','sla.read','escalations.read','escalations.acknowledge','escalations.resolve','escalations.replay')
WHERE r.id IN('rol_system_owner','rol_system_operations','rol_system_compliance') ON CONFLICT DO NOTHING;
