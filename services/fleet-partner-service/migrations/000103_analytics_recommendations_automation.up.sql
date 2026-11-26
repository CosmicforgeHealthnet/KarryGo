-- Sections 24 and 54: tenant-scoped report definitions and runs, explainable
-- recommendations with their model registry, and human-governed automation
-- rules with versions and durable executions.

CREATE TABLE fleet_partner_report_definitions (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 200),
 report_type text NOT NULL CHECK(report_type IN('EXECUTIVE','OPERATIONS','FLEET','WAREHOUSE','WORKFORCE','FINANCE','SLA','CUSTOMER')),
 filters jsonb NOT NULL DEFAULT '{}'::jsonb,
 schedule_cron text NOT NULL DEFAULT '', schedule_timezone text NOT NULL DEFAULT 'UTC', next_run_at timestamptz,
 status text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN('ACTIVE','ARCHIVED')),
 created_by text NOT NULL, version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,name)
);
CREATE INDEX idx_fleet_report_definitions_due ON fleet_partner_report_definitions(status,next_run_at) WHERE schedule_cron<>'';

CREATE TABLE fleet_partner_report_runs (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 definition_id text REFERENCES fleet_partner_report_definitions(id) ON DELETE RESTRICT,
 report_type text NOT NULL,
 filter_snapshot jsonb NOT NULL, permission_snapshot jsonb NOT NULL,
 status text NOT NULL DEFAULT 'QUEUED' CHECK(status IN('QUEUED','RUNNING','COMPLETED','FAILED','EXPIRED')),
 result jsonb, row_count integer NOT NULL DEFAULT 0, error_code text NOT NULL DEFAULT '',
 trigger_reference text NOT NULL,
 requested_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 started_at timestamptz, completed_at timestamptz, expires_at timestamptz NOT NULL,
 UNIQUE(organization_id,trigger_reference)
);
CREATE INDEX idx_fleet_report_runs_scope ON fleet_partner_report_runs(organization_id,status,created_at DESC);

-- Platform-wide registry of the models behind recommendations. Every
-- recommendation names its model and version so it can be explained.
CREATE TABLE fleet_partner_model_versions (
 code text NOT NULL, version text NOT NULL,
 kind text NOT NULL CHECK(kind IN('RULES','ML')),
 recommendation_type text NOT NULL,
 description text NOT NULL,
 status text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN('ACTIVE','RETIRED')),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(code,version)
);
INSERT INTO fleet_partner_model_versions(code,version,kind,recommendation_type,description) VALUES
('maintenance-defect-rules','1','RULES','PREVENTIVE_MAINTENANCE','Open HIGH/CRITICAL asset defects without a work order recommend a work-order draft.'),
('fuel-odometer-anomaly-rules','1','RULES','ANOMALY_REVIEW','A fuel record whose odometer is below the previous record for the same asset is flagged for human review.'),
('trip-freshness-delay-rules','1','RULES','DELAY_RISK','A live trip whose projection is stale is flagged as a delay risk for dispatcher review.')
ON CONFLICT DO NOTHING;

CREATE TABLE fleet_partner_recommendations (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 recommendation_type text NOT NULL CHECK(recommendation_type IN('PREVENTIVE_MAINTENANCE','ANOMALY_REVIEW','DELAY_RISK','DISPATCH_CANDIDATE','DOCUMENT_REVIEW')),
 subject_type text NOT NULL, subject_id text NOT NULL,
 model_code text NOT NULL, model_version text NOT NULL,
 confidence numeric(5,4) NOT NULL CHECK(confidence >= 0 AND confidence <= 1),
 explanation jsonb NOT NULL, input_reference jsonb NOT NULL, inputs_hash text NOT NULL,
 status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','ACCEPTED','DISMISSED','EXPIRED')),
 decided_by text NOT NULL DEFAULT '', decided_at timestamptz, decision_reason text NOT NULL DEFAULT '',
 action_draft jsonb,
 expires_at timestamptz NOT NULL, version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(model_code,model_version) REFERENCES fleet_partner_model_versions(code,version) ON DELETE RESTRICT,
 CHECK(status = 'OPEN' OR decided_at IS NOT NULL OR status = 'EXPIRED')
);
-- One live recommendation per subject and model version; regeneration is idempotent.
CREATE UNIQUE INDEX uq_fleet_recommendation_live ON fleet_partner_recommendations(organization_id,recommendation_type,subject_type,subject_id,model_code,model_version) WHERE status='OPEN';
CREATE INDEX idx_fleet_recommendations_scope ON fleet_partner_recommendations(organization_id,status,recommendation_type,created_at DESC);

CREATE TABLE fleet_partner_automation_rules (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 200),
 trigger_type text NOT NULL CHECK(trigger_type IN('EVENT')),
 trigger_config jsonb NOT NULL, condition_tree jsonb NOT NULL, action_plan jsonb NOT NULL,
 approval_mode text NOT NULL DEFAULT 'REQUIRED' CHECK(approval_mode IN('REQUIRED')),
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','PENDING_APPROVAL','ACTIVE','DISABLED')),
 created_by text NOT NULL, submitted_by text NOT NULL DEFAULT '', approved_by text NOT NULL DEFAULT '', approved_at timestamptz,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,name),
 CHECK(status <> 'ACTIVE' OR (approved_by <> '' AND approved_by <> submitted_by))
);
CREATE INDEX idx_fleet_automation_rules_active ON fleet_partner_automation_rules(organization_id,status,trigger_type);

CREATE TABLE fleet_partner_automation_rule_versions (
 rule_id text NOT NULL REFERENCES fleet_partner_automation_rules(id) ON DELETE RESTRICT,
 organization_id text NOT NULL, version bigint NOT NULL,
 snapshot jsonb NOT NULL, status text NOT NULL, actor_reference text NOT NULL,
 occurred_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(rule_id,version)
);
CREATE TRIGGER trg_fleet_automation_rule_versions_append_only BEFORE UPDATE OR DELETE ON fleet_partner_automation_rule_versions FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_automation_executions (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 rule_id text NOT NULL REFERENCES fleet_partner_automation_rules(id) ON DELETE RESTRICT,
 rule_version bigint NOT NULL,
 trigger_reference text NOT NULL,
 status text NOT NULL DEFAULT 'STARTED' CHECK(status IN('STARTED','COMPLETED','FAILED','SKIPPED')),
 evaluation_trace jsonb NOT NULL, action_results jsonb NOT NULL DEFAULT '[]'::jsonb,
 error_code text NOT NULL DEFAULT '',
 started_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz,
 UNIQUE(rule_id,trigger_reference)
);
CREATE INDEX idx_fleet_automation_executions_scope ON fleet_partner_automation_executions(organization_id,status,started_at DESC);

-- The evaluator's position in each organization's outbox.
CREATE TABLE fleet_partner_automation_cursors (
 organization_id text PRIMARY KEY REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 last_created_at timestamptz NOT NULL, last_event_id text NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('reports.read','section24','Read reports, report runs and dashboards',true),
('reports.create','section24','Define and run reports',true),
('analytics.read','section24','Read dashboards',true),
('recommendations.read','section24','Read explainable recommendations',true),
('recommendations.decide','section24','Accept or dismiss recommendations',true),
('recommendations.generate','section24','Run the recommendation models',true),
('automation.read','section24','Read automation rules and executions',true),
('automation.manage','section24','Draft and submit automation rules',true),
('automation.approve','section24','Approve, activate and disable automation rules',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.category='section24'
 AND p.code IN('reports.read','reports.create','analytics.read','recommendations.read','recommendations.decide','recommendations.generate','automation.read','automation.manage','automation.approve')
WHERE r.id IN('rol_system_owner','rol_system_admin') ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code IN('reports.read','reports.create','analytics.read','recommendations.read','recommendations.decide','automation.read')
WHERE r.id IN('rol_system_operations','rol_system_analyst') ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code IN('reports.read','analytics.read')
WHERE r.id IN('rol_system_finance','rol_system_view_only') ON CONFLICT DO NOTHING;
