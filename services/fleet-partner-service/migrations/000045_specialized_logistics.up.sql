-- Section 20: organization capabilities and isolated specialized extension data.
-- Reuse the organization capability authority introduced in 000004. Section
-- 20 extends it with a stable row identifier and typed configuration; it does
-- not create a competing capability table.
ALTER TABLE fleet_partner_organization_capabilities
 DROP CONSTRAINT fleet_partner_organization_capabilities_status_check;
ALTER TABLE fleet_partner_organization_capabilities
 ADD CONSTRAINT fleet_partner_organization_capabilities_status_check
 CHECK(status IN('REQUESTED','APPROVED','REJECTED','WITHDRAWN','DISABLED','ENABLED'));
ALTER TABLE fleet_partner_organization_capabilities
 ADD COLUMN id text DEFAULT ('cap_'||replace(gen_random_uuid()::text,'-','')),
 ADD COLUMN configuration jsonb NOT NULL DEFAULT '{}'::jsonb,
 ADD COLUMN enabled_at timestamptz,
 ADD COLUMN disabled_at timestamptz;
UPDATE fleet_partner_organization_capabilities SET id=('cap_'||replace(gen_random_uuid()::text,'-','')) WHERE id IS NULL;
ALTER TABLE fleet_partner_organization_capabilities ALTER COLUMN id SET NOT NULL;
ALTER TABLE fleet_partner_organization_capabilities ADD CONSTRAINT uq_fleet_partner_organization_capability_id UNIQUE(id);
CREATE TABLE fleet_partner_capability_history (
 id text PRIMARY KEY, organization_id text NOT NULL, capability_id text NOT NULL REFERENCES fleet_partner_organization_capabilities(id) ON DELETE RESTRICT,
 previous_status text NOT NULL, new_status text NOT NULL, actor_reference text NOT NULL, reason text NOT NULL,
 version bigint NOT NULL, snapshot jsonb NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_capability_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_capability_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

-- One typed extension store avoids duplicating orders, shipments, assets, trips,
-- wallets and customers. Payload shape is validated server-side per module and
-- record_type; the common envelope remains tenant/version/idempotency safe.
CREATE TABLE fleet_partner_specialized_records (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 branch_id text REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 capability_code text NOT NULL CHECK(capability_code IN('COURIER_LAST_MILE','TAXI_PASSENGER','TRUCKING_HAULAGE','HEAVY_SPECIALIZED','FREIGHT_FORWARDING','COLD_CHAIN','MEDICAL_SENSITIVE','AGRICULTURAL_FOOD','MOVING_RELOCATION','TOWING_RECOVERY')),
 record_type text NOT NULL CHECK(record_type IN(
  'SHIPPING_LABEL','DELIVERY_WAVE','DELIVERY_ATTEMPT','REDELIVERY_REQUEST','RETURN_TO_SENDER',
  'PASSENGER_MANIFEST','NO_SHOW','WAITING_EVENT','SHUTTLE_DEFINITION','PASSENGER_SAFETY_REQUIREMENT',
  'HAULAGE_PROFILE','LOADING_APPOINTMENT','DETENTION','BACKHAUL',
  'HEAVY_PROFILE','ROUTE_SURVEY','PERMIT','ESCORT_REQUIREMENT','LIFTING_REQUIREMENT',
  'COUNTRY_LEG','BROKER_REFERENCE','CUSTOMS_DOCUMENT','BORDER_EVENT','MULTIMODAL_HANDOVER',
  'TEMPERATURE_REQUIREMENT','SENSOR_REFERENCE','EXCURSION','QUARANTINE','AUTHORIZED_RELEASE',
  'MEDICAL_HANDLING','EMERGENCY_EXCEPTION','ENHANCED_CUSTODY',
  'AGRICULTURAL_PROFILE','SPOILAGE_EVENT','DISPOSITION',
  'MOVING_INVENTORY','SITE_PLAN','CREW_ASSIGNMENT','CONDITION','DAMAGE_RECORD',
  'TOWING_RECOVERY','ROADSIDE_MILESTONE')),
 subject_type text NOT NULL, subject_reference text NOT NULL,
 status text NOT NULL DEFAULT 'DRAFT', restricted_visibility boolean NOT NULL DEFAULT false,
 payload jsonb NOT NULL DEFAULT '{}'::jsonb, external_financial_reference text NOT NULL DEFAULT '',
 source_authority text NOT NULL DEFAULT '', source_reference text NOT NULL DEFAULT '',
 idempotency_key text NOT NULL, version bigint NOT NULL DEFAULT 1,
 created_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz,
 UNIQUE(organization_id,capability_code,idempotency_key)
);
CREATE INDEX idx_fleet_specialized_records_scope ON fleet_partner_specialized_records(organization_id,capability_code,record_type,subject_reference,status);

CREATE TABLE fleet_partner_specialized_milestones (
 id text PRIMARY KEY, organization_id text NOT NULL, record_id text NOT NULL REFERENCES fleet_partner_specialized_records(id) ON DELETE RESTRICT,
 milestone_type text NOT NULL, status text NOT NULL, safe_metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
 source_time timestamptz NOT NULL, received_at timestamptz NOT NULL DEFAULT now(), actor_reference text NOT NULL,
 correlation_id text NOT NULL, UNIQUE(organization_id,record_id,correlation_id)
);
CREATE TRIGGER trg_fleet_specialized_milestones_append_only BEFORE UPDATE OR DELETE ON fleet_partner_specialized_milestones FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_temperature_events (
 id text PRIMARY KEY, organization_id text NOT NULL, record_id text NOT NULL REFERENCES fleet_partner_specialized_records(id) ON DELETE RESTRICT,
 sensor_reference text NOT NULL, temperature numeric(10,3) NOT NULL, unit text NOT NULL CHECK(unit IN('C','F')),
 source_version bigint NOT NULL CHECK(source_version>=0), source_time timestamptz NOT NULL, received_at timestamptz NOT NULL DEFAULT now(),
 excursion boolean NOT NULL, safe_metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
 UNIQUE(organization_id,sensor_reference,source_version)
);
CREATE TRIGGER trg_fleet_temperature_events_append_only BEFORE UPDATE OR DELETE ON fleet_partner_temperature_events FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_sensitive_access_audit (
 id text PRIMARY KEY, organization_id text NOT NULL, specialized_record_id text NOT NULL REFERENCES fleet_partner_specialized_records(id) ON DELETE RESTRICT,
 actor_reference text NOT NULL, permission_code text NOT NULL, purpose text NOT NULL, correlation_id text NOT NULL,
 accessed_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_sensitive_access_append_only BEFORE UPDATE OR DELETE ON fleet_partner_sensitive_access_audit FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('specialized.read','specialized','Read enabled specialized logistics records',true),
('specialized.manage','specialized','Manage enabled specialized logistics records',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r CROSS JOIN fleet_partner_permissions p WHERE r.id IN('rol_system_owner','rol_system_operations') AND p.category='specialized' ON CONFLICT DO NOTHING;
