-- Sections 23 and 53: operational exceptions with owner, severity, SLA,
-- evidence, root cause and resolution; and per-user / organization-default
-- notification preferences that narrow delivery channels.

CREATE TABLE fleet_partner_operational_exceptions (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 branch_id text REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 exception_type text NOT NULL CHECK(exception_type IN('DELAY','DAMAGE','LOSS','CUSTODY','TEMPERATURE','PROVIDER_UNAVAILABLE','VEHICLE','CUSTOMER','DOCUMENT','SYSTEM','OTHER')),
 severity text NOT NULL CHECK(severity IN('LOW','MEDIUM','HIGH','CRITICAL')),
 status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','ACKNOWLEDGED','ESCALATED','RESOLVED','CLOSED')),
 source_type text NOT NULL, source_reference text NOT NULL,
 summary text NOT NULL CHECK(length(summary) BETWEEN 1 AND 2000),
 owner_user_id text NOT NULL DEFAULT '',
 evidence_file_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
 sla_due_at timestamptz NOT NULL,
 escalation_level integer NOT NULL DEFAULT 0 CHECK(escalation_level >= 0),
 root_cause text NOT NULL DEFAULT '', resolution text NOT NULL DEFAULT '',
 acknowledged_at timestamptz, resolved_at timestamptz,
 created_by text NOT NULL, version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(status NOT IN('RESOLVED','CLOSED') OR (root_cause <> '' AND resolution <> '' AND resolved_at IS NOT NULL))
);
-- At most one live exception per source and type: re-raising returns it.
CREATE UNIQUE INDEX uq_fleet_operational_exception_live ON fleet_partner_operational_exceptions(organization_id,source_type,source_reference,exception_type) WHERE status IN('OPEN','ACKNOWLEDGED','ESCALATED');
CREATE INDEX idx_fleet_operational_exceptions_scope ON fleet_partner_operational_exceptions(organization_id,branch_id,status,severity,sla_due_at);

CREATE TABLE fleet_partner_operational_exception_history (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 exception_id text NOT NULL REFERENCES fleet_partner_operational_exceptions(id) ON DELETE RESTRICT,
 previous_status text NOT NULL, new_status text NOT NULL, actor_reference text NOT NULL,
 note text NOT NULL DEFAULT '', version bigint NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fleet_operational_exception_history ON fleet_partner_operational_exception_history(organization_id,exception_id,occurred_at);
CREATE TRIGGER trg_fleet_operational_exception_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_operational_exception_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_notification_preferences (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 subject_type text NOT NULL CHECK(subject_type IN('ORGANIZATION','USER')),
 subject_id text NOT NULL,
 event_type text NOT NULL,
 channels jsonb NOT NULL DEFAULT '[]'::jsonb,
 quiet_hours jsonb NOT NULL DEFAULT '{}'::jsonb,
 enabled boolean NOT NULL DEFAULT true,
 version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,subject_type,subject_id,event_type)
);

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('exceptions.read','section23','Read operational exceptions',true),
('exceptions.manage','section23','Raise, acknowledge and escalate operational exceptions',true),
('exceptions.resolve','section23','Resolve operational exceptions with root cause',true),
('notification_preferences.manage','section23','Manage own notification preferences',true),
('notification_preferences.manage_organization','section23','Manage organization default notification preferences',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code IN('exceptions.read','exceptions.manage','exceptions.resolve','notification_preferences.manage_organization')
WHERE r.id IN('rol_system_owner','rol_system_admin','rol_system_operations') ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r JOIN fleet_partner_permissions p ON p.code IN('exceptions.read','exceptions.manage')
WHERE r.id IN('rol_system_dispatcher','rol_system_support','rol_system_warehouse','rol_system_fleet') ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,'notification_preferences.manage' FROM fleet_partner_roles r WHERE r.id LIKE 'rol_system_%' AND EXISTS(SELECT 1 FROM fleet_partner_permissions WHERE code='notification_preferences.manage') ON CONFLICT DO NOTHING;
