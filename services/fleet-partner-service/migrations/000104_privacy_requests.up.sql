-- Sections 28.5 and 56: traceable privacy requests. A request is reviewed,
-- an erasure is refused while a legal hold covers the subject, and
-- completion records what was done; nothing here deletes financial, safety,
-- trip or audit evidence.
CREATE TABLE fleet_partner_privacy_requests (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 request_type text NOT NULL CHECK(request_type IN('EXPORT','ERASURE','RECTIFICATION','RESTRICTION')),
 subject_type text NOT NULL CHECK(subject_type IN('PROVIDER','CUSTOMER','ORGANIZATION_USER')),
 subject_reference text NOT NULL,
 requested_by text NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 2000),
 status text NOT NULL DEFAULT 'SUBMITTED' CHECK(status IN('SUBMITTED','APPROVED','REJECTED','BLOCKED_BY_LEGAL_HOLD','COMPLETED')),
 decided_by text NOT NULL DEFAULT '', decided_at timestamptz, decision_reason text NOT NULL DEFAULT '',
 blocking_hold_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
 completion_report jsonb,
 completed_by text NOT NULL DEFAULT '', completed_at timestamptz,
 due_at timestamptz NOT NULL,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(status <> 'COMPLETED' OR completion_report IS NOT NULL),
 CHECK(decided_by = '' OR decided_by <> requested_by)
);
CREATE UNIQUE INDEX uq_fleet_privacy_request_open ON fleet_partner_privacy_requests(organization_id,request_type,subject_type,subject_reference) WHERE status IN('SUBMITTED','APPROVED');
CREATE INDEX idx_fleet_privacy_requests_scope ON fleet_partner_privacy_requests(organization_id,status,due_at);

CREATE TABLE fleet_partner_privacy_request_history (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 privacy_request_id text NOT NULL REFERENCES fleet_partner_privacy_requests(id) ON DELETE RESTRICT,
 previous_status text NOT NULL, new_status text NOT NULL, actor_reference text NOT NULL, note text NOT NULL DEFAULT '',
 occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_privacy_request_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_privacy_request_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('privacy.manage','section56','Submit and complete privacy requests',true),
('privacy.decide','section56','Approve or reject privacy requests',true),
('security_events.read','section56','Read organization security events',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code IN('privacy.manage','privacy.decide','security_events.read')
WHERE r.id IN('rol_system_owner','rol_system_admin','rol_system_compliance') ON CONFLICT DO NOTHING;
