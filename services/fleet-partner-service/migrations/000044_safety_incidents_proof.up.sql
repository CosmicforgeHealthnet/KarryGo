-- Section 19.2: organization safety, incident management and immutable proof.
CREATE TABLE fleet_partner_safety_reports (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 branch_id text REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 reporter_type text NOT NULL, reporter_reference_protected text NOT NULL DEFAULT '', anonymous boolean NOT NULL DEFAULT false,
 report_type text NOT NULL, severity text NOT NULL CHECK(severity IN('LOW','MEDIUM','HIGH','CRITICAL')),
 location_reference text NOT NULL DEFAULT '', route_id text NOT NULL DEFAULT '', assignment_id text NOT NULL DEFAULT '', trip_reference text NOT NULL DEFAULT '',
 provider_membership_id text NOT NULL DEFAULT '', asset_id text REFERENCES fleet_partner_assets(id) ON DELETE RESTRICT,
 description text NOT NULL, immediate_action text NOT NULL DEFAULT '',
 status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','UNDER_REVIEW','ACTION_REQUIRED','RESOLVED','DISMISSED')),
 owner_user_id text NOT NULL DEFAULT '', resolution_action text NOT NULL DEFAULT '', resolution_reason text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), closed_at timestamptz
);
CREATE INDEX idx_fleet_safety_reports_scope ON fleet_partner_safety_reports(organization_id,branch_id,status,severity,created_at);

CREATE TABLE fleet_partner_incidents (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 branch_id text REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 incident_type text NOT NULL CHECK(incident_type IN('ACCIDENT','SECURITY','THEFT','CARGO_DAMAGE','PERSONAL_INJURY','ASSET_DAMAGE','ENVIRONMENTAL','OPERATIONAL','DATA_OR_DOCUMENT','OTHER')),
 severity text NOT NULL CHECK(severity IN('LOW','MEDIUM','HIGH','CRITICAL')),
 status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','INVESTIGATING','CONTAINED','CORRECTIVE_ACTION_REQUIRED','RESOLVED','CLOSED')),
 occurred_at timestamptz NOT NULL, location_reference text NOT NULL DEFAULT '', route_id text NOT NULL DEFAULT '', assignment_id text NOT NULL DEFAULT '', trip_reference text NOT NULL DEFAULT '',
 people_references jsonb NOT NULL DEFAULT '[]', asset_references jsonb NOT NULL DEFAULT '[]', shipment_references jsonb NOT NULL DEFAULT '[]',
 external_reference text NOT NULL DEFAULT '', description text NOT NULL, reported_by text NOT NULL, owner_user_id text NOT NULL DEFAULT '',
 resolution_action text NOT NULL DEFAULT '', resolution_reason text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), closed_at timestamptz
);
CREATE INDEX idx_fleet_incidents_scope ON fleet_partner_incidents(organization_id,branch_id,status,severity,occurred_at);

CREATE TABLE fleet_partner_incident_history (
 id text PRIMARY KEY, organization_id text NOT NULL, incident_id text NOT NULL REFERENCES fleet_partner_incidents(id) ON DELETE RESTRICT,
 previous_status text NOT NULL, new_status text NOT NULL, action text NOT NULL, actor_reference text NOT NULL,
 reason text NOT NULL DEFAULT '', version bigint NOT NULL, snapshot jsonb NOT NULL, occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_incident_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_incident_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_incident_evidence (
 id text PRIMARY KEY, organization_id text NOT NULL, incident_id text NOT NULL REFERENCES fleet_partner_incidents(id) ON DELETE RESTRICT,
 media_file_id text NOT NULL, hash_metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
 captured_by text NOT NULL, captured_at timestamptz NOT NULL, source_time timestamptz,
 safe_metadata jsonb NOT NULL DEFAULT '{}'::jsonb, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,incident_id,media_file_id)
);
CREATE TRIGGER trg_fleet_incident_evidence_append_only BEFORE UPDATE OR DELETE ON fleet_partner_incident_evidence FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_corrective_actions (
 id text PRIMARY KEY, organization_id text NOT NULL, incident_id text NOT NULL REFERENCES fleet_partner_incidents(id) ON DELETE RESTRICT,
 action_type text NOT NULL, owner_user_id text NOT NULL, due_at timestamptz NOT NULL,
 status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','IN_PROGRESS','COMPLETED','VERIFIED','OVERDUE','CANCELLED')),
 verification_requirements jsonb NOT NULL DEFAULT '{}'::jsonb,
 completion_notes text NOT NULL DEFAULT '', completed_at timestamptz, verified_by text NOT NULL DEFAULT '', verified_at timestamptz,
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(verified_at IS NULL OR completed_at IS NOT NULL)
);
CREATE INDEX idx_fleet_corrective_actions_due ON fleet_partner_corrective_actions(organization_id,status,due_at);

CREATE TABLE fleet_partner_proof_policies (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 policy_scope text NOT NULL CHECK(policy_scope IN('ORGANIZATION','CUSTOMER','CONTRACT','SERVICE','ORDER','SHIPMENT','STOP','CARGO')),
 scope_reference text NOT NULL DEFAULT '', service_type text NOT NULL DEFAULT '', stop_type text NOT NULL DEFAULT '',
 required_proof_types jsonb NOT NULL, minimum_count integer NOT NULL DEFAULT 1 CHECK(minimum_count>0),
 location_required boolean NOT NULL DEFAULT false, time_window_rules jsonb NOT NULL DEFAULT '{}'::jsonb,
 failure_policy text NOT NULL CHECK(failure_policy IN('BLOCK','WARN')),
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','PUBLISHED','INACTIVE','ARCHIVED')),
 version bigint NOT NULL DEFAULT 1, created_by text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,policy_scope,scope_reference,service_type,stop_type)
);

CREATE TABLE fleet_partner_proof_policy_versions (
 id text PRIMARY KEY, organization_id text NOT NULL, policy_id text NOT NULL REFERENCES fleet_partner_proof_policies(id) ON DELETE RESTRICT,
 policy_version bigint NOT NULL, snapshot jsonb NOT NULL, published_by text NOT NULL, published_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(policy_id,policy_version)
);
CREATE TRIGGER trg_fleet_proof_policy_versions_append_only BEFORE UPDATE OR DELETE ON fleet_partner_proof_policy_versions FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_proof_records (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 policy_snapshot_reference text NOT NULL REFERENCES fleet_partner_proof_policy_versions(id) ON DELETE RESTRICT,
 proof_type text NOT NULL CHECK(proof_type IN('OTP','SIGNATURE','PHOTO','PACKAGE_SCAN','DOCUMENT_SCAN','LOCATION','TIMESTAMP','RECIPIENT_NAME','IDENTITY_REFERENCE','SEAL','TEMPERATURE_READING','CUSTOM_FIELD')),
 order_id text NOT NULL DEFAULT '', shipment_id text NOT NULL DEFAULT '', package_id text REFERENCES fleet_partner_packages(id) ON DELETE RESTRICT,
 route_stop_id text NOT NULL DEFAULT '', assignment_id text NOT NULL DEFAULT '', canonical_trip_reference text NOT NULL DEFAULT '',
 actor_type text NOT NULL, actor_reference text NOT NULL, media_file_id text NOT NULL DEFAULT '',
 value_hash text NOT NULL DEFAULT '', safe_value_metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
 latitude numeric(9,6), longitude numeric(9,6), accuracy numeric(10,3),
 source_timestamp timestamptz NOT NULL, received_at timestamptz NOT NULL DEFAULT now(), source text NOT NULL,
 verification_status text NOT NULL DEFAULT 'PENDING' CHECK(verification_status IN('PENDING','VERIFIED','REJECTED','UNKNOWN')),
 source_version bigint NOT NULL DEFAULT 0, idempotency_key text NOT NULL,
 version bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,idempotency_key),
 CHECK(latitude IS NULL OR latitude BETWEEN -90 AND 90), CHECK(longitude IS NULL OR longitude BETWEEN -180 AND 180),
 CHECK(proof_type<>'OTP' OR value_hash<>''), CHECK(proof_type NOT IN('PHOTO','SIGNATURE','DOCUMENT_SCAN') OR media_file_id<>'')
);
CREATE INDEX idx_fleet_proof_records_subject ON fleet_partner_proof_records(organization_id,order_id,shipment_id,package_id,route_stop_id,proof_type);
CREATE TRIGGER trg_fleet_proof_records_append_only BEFORE UPDATE OR DELETE ON fleet_partner_proof_records FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

CREATE TABLE fleet_partner_proof_verifications (
 id text PRIMARY KEY, organization_id text NOT NULL, proof_record_id text NOT NULL REFERENCES fleet_partner_proof_records(id) ON DELETE RESTRICT,
 status text NOT NULL CHECK(status IN('VERIFIED','REJECTED','UNKNOWN')), authority_type text NOT NULL,
 authority_reference text NOT NULL DEFAULT '', source_version bigint NOT NULL DEFAULT 0,
 reason_codes jsonb NOT NULL DEFAULT '[]'::jsonb, verified_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(proof_record_id,authority_type,source_version)
);
CREATE TRIGGER trg_fleet_proof_verifications_append_only BEFORE UPDATE OR DELETE ON fleet_partner_proof_verifications FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();

INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
('safety.read','safety','Read safety reports',true),('safety.report','safety','Create safety reports',true),('safety.manage','safety','Manage safety reports',true),
('incidents.read','incidents','Read incidents',true),('incidents.manage','incidents','Manage incidents',true),('incidents.attach_evidence','incidents','Attach incident evidence',true),('incidents.verify_actions','incidents','Verify corrective actions',true),
('proof.read','proof','Read proof policies and records',true),('proof.manage_policies','proof','Manage proof policies',true),('proof.record','proof','Record proof',true),('proof.verify','proof','Verify proof',true)
ON CONFLICT(code) DO UPDATE SET category=excluded.category,description=excluded.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r CROSS JOIN fleet_partner_permissions p WHERE r.id IN('rol_system_owner','rol_system_operations') AND p.category IN('safety','incidents','proof') ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT 'rol_system_compliance',code FROM fleet_partner_permissions WHERE code IN(
 'safety.read','safety.report','safety.manage',
 'incidents.read','incidents.manage','incidents.attach_evidence','incidents.verify_actions',
 'proof.read','proof.manage_policies','proof.record','proof.verify'
) ON CONFLICT DO NOTHING;
