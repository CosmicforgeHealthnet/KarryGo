-- Section 19.1: deterministic organization compliance and durable restrictions.
CREATE TABLE fleet_partner_compliance_requirements (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 code text NOT NULL,
 name text NOT NULL,
 subject_type text NOT NULL CHECK(subject_type IN('ORGANIZATION','PROVIDER','ASSET','ORDER','SHIPMENT','CARGO','WAREHOUSE','LOCATION')),
 service_type text NOT NULL DEFAULT '', asset_type text NOT NULL DEFAULT '',
 jurisdiction_reference text NOT NULL DEFAULT '', location_reference text NOT NULL DEFAULT '',
 requirement_type text NOT NULL,
 required_document_type text NOT NULL DEFAULT '',
 required_qualification_id text REFERENCES fleet_partner_qualifications(id) ON DELETE RESTRICT,
 required_training_course_id text REFERENCES fleet_partner_training_courses(id) ON DELETE RESTRICT,
 validity_rules jsonb NOT NULL DEFAULT '{}'::jsonb,
 severity text NOT NULL CHECK(severity IN('INFO','WARNING','HIGH','CRITICAL')),
 blocks_operation boolean NOT NULL DEFAULT false,
 effective_from timestamptz NOT NULL,
 effective_until timestamptz,
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','ACTIVE','INACTIVE','ARCHIVED')),
 version bigint NOT NULL DEFAULT 1 CHECK(version > 0),
 created_by text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,code), CHECK(effective_until IS NULL OR effective_until > effective_from),
 CHECK(required_document_type<>'' OR required_qualification_id IS NOT NULL OR required_training_course_id IS NOT NULL OR requirement_type<>'EVIDENCE')
);
CREATE INDEX idx_fleet_compliance_requirements_scope ON fleet_partner_compliance_requirements(organization_id,subject_type,status,effective_from,effective_until);

CREATE TABLE fleet_partner_compliance_statuses (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 subject_type text NOT NULL CHECK(subject_type IN('ORGANIZATION','PROVIDER','ASSET','ORDER','SHIPMENT','CARGO','WAREHOUSE','LOCATION')),
 subject_reference text NOT NULL,
 requirement_id text NOT NULL REFERENCES fleet_partner_compliance_requirements(id) ON DELETE RESTRICT,
 state text NOT NULL CHECK(state IN('UNKNOWN','PENDING','COMPLIANT','DUE_SOON','EXPIRED','NON_COMPLIANT','RESTRICTED','NOT_APPLICABLE')),
 due_at timestamptz, expires_at timestamptz,
 source_type text NOT NULL, source_reference text NOT NULL DEFAULT '', source_version bigint NOT NULL CHECK(source_version >= 0),
 evaluated_at timestamptz NOT NULL, reason_codes jsonb NOT NULL DEFAULT '[]'::jsonb,
 restriction_reference text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1 CHECK(version > 0),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,subject_type,subject_reference,requirement_id),
 CHECK(expires_at IS NULL OR due_at IS NULL OR expires_at >= due_at)
);
CREATE INDEX idx_fleet_compliance_status_subject ON fleet_partner_compliance_statuses(organization_id,subject_type,subject_reference,state);
CREATE INDEX idx_fleet_compliance_status_expiry ON fleet_partner_compliance_statuses(organization_id,state,expires_at);

CREATE TABLE fleet_partner_compliance_status_history (
 id text PRIMARY KEY, organization_id text NOT NULL, compliance_status_id text NOT NULL REFERENCES fleet_partner_compliance_statuses(id) ON DELETE RESTRICT,
 previous_state text NOT NULL, new_state text NOT NULL, source_type text NOT NULL, source_reference text NOT NULL DEFAULT '', source_version bigint NOT NULL,
 reason_codes jsonb NOT NULL DEFAULT '[]'::jsonb, restriction_reference text NOT NULL DEFAULT '', snapshot jsonb NOT NULL,
 actor_reference text NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_compliance_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_compliance_status_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_restrictions (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 subject_type text NOT NULL CHECK(subject_type IN('ORGANIZATION','PROVIDER','ASSET','CAPABILITY','SHIPMENT','CARGO')),
 subject_reference text NOT NULL,
 reason_code text NOT NULL, source_type text NOT NULL, source_reference text NOT NULL,
 severity text NOT NULL CHECK(severity IN('HIGH','CRITICAL')),
 state text NOT NULL DEFAULT 'APPLIED' CHECK(state IN('APPLIED','CLEAR_PENDING','CLEARED')),
 required_authorities jsonb NOT NULL DEFAULT '[]'::jsonb,
 effective_at timestamptz NOT NULL DEFAULT now(), cleared_at timestamptz,
 version bigint NOT NULL DEFAULT 1 CHECK(version > 0),
 created_by text NOT NULL, cleared_by text NOT NULL DEFAULT '', clearance_reason text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,subject_type,subject_reference,source_type,source_reference)
);
CREATE INDEX idx_fleet_restrictions_active ON fleet_partner_restrictions(organization_id,subject_type,subject_reference,state) WHERE state<>'CLEARED';

CREATE TABLE fleet_partner_restriction_history (
 id text PRIMARY KEY, organization_id text NOT NULL, restriction_id text NOT NULL REFERENCES fleet_partner_restrictions(id) ON DELETE RESTRICT,
 previous_state text NOT NULL, new_state text NOT NULL, reason_code text NOT NULL, actor_reference text NOT NULL,
 version bigint NOT NULL, snapshot jsonb NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_restriction_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_restriction_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_restriction_operations (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 restriction_id text NOT NULL REFERENCES fleet_partner_restrictions(id) ON DELETE RESTRICT,
 authority_service text NOT NULL CHECK(authority_service IN('DISPATCH','HAULING')),
 action text NOT NULL CHECK(action IN('APPLY','CLEAR','READ_APPLY','READ_CLEAR')),
 idempotency_key text NOT NULL, expected_version bigint NOT NULL,
 status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','RUNNING','OUTCOME_UNKNOWN','CONFIRMED','RETRY_SCHEDULED','DEAD_LETTERED')),
 attempts integer NOT NULL DEFAULT 0, next_attempt_at timestamptz NOT NULL DEFAULT now(),
 last_error_code text NOT NULL DEFAULT '', authority_version bigint NOT NULL DEFAULT 0,
 confirmed_at timestamptz, version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,authority_service,idempotency_key)
);
CREATE INDEX idx_fleet_restriction_operations_due ON fleet_partner_restriction_operations(status,next_attempt_at) WHERE status IN('PENDING','OUTCOME_UNKNOWN','RETRY_SCHEDULED');

CREATE TABLE fleet_partner_restriction_attempts (
 id text PRIMARY KEY, organization_id text NOT NULL, operation_id text NOT NULL REFERENCES fleet_partner_restriction_operations(id) ON DELETE RESTRICT,
 attempt_number integer NOT NULL CHECK(attempt_number>0), request_action text NOT NULL, outcome text NOT NULL,
 response_code integer, error_code text NOT NULL DEFAULT '', authority_version bigint NOT NULL DEFAULT 0,
 started_at timestamptz NOT NULL, completed_at timestamptz NOT NULL, UNIQUE(operation_id,attempt_number)
);
CREATE TRIGGER trg_fleet_restriction_attempts_append_only BEFORE UPDATE OR DELETE ON fleet_partner_restriction_attempts FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('compliance.read','compliance','Read compliance requirements and evaluations',true),
('compliance.manage','compliance','Manage compliance requirements',true),
('compliance.evaluate','compliance','Evaluate compliance evidence',true),
('compliance.restrict','compliance','Apply operational restrictions',true),
('compliance.clear_restriction','compliance','Clear confirmed operational restrictions',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r CROSS JOIN fleet_partner_permissions p
WHERE r.id IN('rol_system_owner','rol_system_operations','rol_system_compliance') AND p.category='compliance' ON CONFLICT DO NOTHING;
