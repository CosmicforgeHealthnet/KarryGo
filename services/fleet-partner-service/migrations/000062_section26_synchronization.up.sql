CREATE TABLE fleet_partner_synchronization_projections (
 id text PRIMARY KEY, organization_id text NOT NULL, projection_type text NOT NULL, source_service text NOT NULL,
 subject_reference text NOT NULL, source_version bigint NOT NULL, payload_hash text NOT NULL, safe_state jsonb NOT NULL DEFAULT '{}',
 gap_conflict_state text NOT NULL DEFAULT 'CURRENT' CHECK(gap_conflict_state IN('CURRENT','GAP','CONFLICT')),
 last_event_at timestamptz NOT NULL, last_reconciled_at timestamptz, next_reconciliation_at timestamptz,
 reconciliation_status text NOT NULL DEFAULT 'IDLE' CHECK(reconciliation_status IN('IDLE','PENDING','LEASED','DEAD_LETTER')),
 last_safe_error_code text NOT NULL DEFAULT '', attempts int NOT NULL DEFAULT 0, lease_owner text, lease_expires_at timestamptz,
 heartbeat_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,projection_type,source_service,subject_reference)
);
CREATE INDEX fleet_sync_projection_claim ON fleet_partner_synchronization_projections(reconciliation_status,next_reconciliation_at) WHERE reconciliation_status IN('PENDING','LEASED');
CREATE TABLE fleet_partner_synchronization_cases (
 id text PRIMARY KEY, organization_id text NOT NULL, projection_id text NOT NULL REFERENCES fleet_partner_synchronization_projections(id),
 reason text NOT NULL, status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','LEASED','RESOLVED','DEAD_LETTER')),
 attempts int NOT NULL DEFAULT 0, next_attempt_at timestamptz NOT NULL DEFAULT now(), lease_owner text, lease_expires_at timestamptz,
 heartbeat_at timestamptz, last_safe_error_code text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), resolved_at timestamptz
);
CREATE INDEX fleet_sync_case_claim ON fleet_partner_synchronization_cases(status,next_attempt_at) WHERE status IN('OPEN','LEASED');
CREATE TABLE fleet_partner_synchronization_attempts (
 id text PRIMARY KEY, organization_id text NOT NULL, projection_id text NOT NULL, case_id text,
 attempt int NOT NULL, outcome text NOT NULL, safe_error_code text NOT NULL DEFAULT '', occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER fleet_synchronization_attempts_append_only BEFORE UPDATE OR DELETE ON fleet_partner_synchronization_attempts FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
CREATE TABLE fleet_partner_synchronization_dead_letters (
 id text PRIMARY KEY, organization_id text NOT NULL, source_service text NOT NULL, event_id text NOT NULL,
 event_type text NOT NULL, subject_reference text NOT NULL, source_version bigint NOT NULL, payload_hash text NOT NULL,
 status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','REPLAYED')), attempts int NOT NULL DEFAULT 0,
 last_safe_error_code text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), replayed_at timestamptz,
 UNIQUE(source_service,event_id)
);
INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
 ('synchronization.read','synchronization','Read safe synchronization state',true),
 ('synchronization.reconcile','synchronization','Request authoritative reconciliation',true),
 ('synchronization.replay','synchronization','Replay synchronization dead letters',true)
ON CONFLICT(code) DO UPDATE SET category=EXCLUDED.category,description=EXCLUDED.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code)
SELECT r.id,p.code FROM fleet_partner_roles r CROSS JOIN fleet_partner_permissions p
WHERE r.id IN('rol_system_owner','rol_system_admin','rol_system_operations') AND p.code LIKE 'synchronization.%' ON CONFLICT DO NOTHING;
