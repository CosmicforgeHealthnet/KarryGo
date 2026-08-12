-- Section 15 (WP14), slice A7: manifests and immutable closure snapshots.
CREATE TABLE IF NOT EXISTS fleet_partner_manifests (
 id text PRIMARY KEY,
 organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 branch_id text NOT NULL REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 manifest_reference text NOT NULL,
 route_reference text NOT NULL DEFAULT '',
 future_route_plan_id text,
 vehicle_reference text NOT NULL DEFAULT '',
 provider_membership_id text NOT NULL DEFAULT '',
 seal_number text NOT NULL DEFAULT '',
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN ('DRAFT','OPEN','CLOSED','DISPATCHED','COMPLETED','CANCELLED')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>=1),
 created_by text NOT NULL,
 updated_by text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 closed_at timestamptz, dispatched_at timestamptz, completed_at timestamptz, cancelled_at timestamptz,
 UNIQUE(organization_id,manifest_reference), UNIQUE(organization_id,id)
);
CREATE INDEX IF NOT EXISTS idx_fleet_manifests_cursor ON fleet_partner_manifests(organization_id,created_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS fleet_partner_manifest_items (
 id text PRIMARY KEY,
 organization_id text NOT NULL,
 manifest_id text NOT NULL REFERENCES fleet_partner_manifests(id) ON DELETE RESTRICT,
 item_type text NOT NULL CHECK(item_type IN ('SHIPMENT','PACKAGE','ORDER_STOP')),
 item_id text NOT NULL,
 shipment_id text,
 added_by text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(manifest_id,item_type,item_id)
);
CREATE INDEX IF NOT EXISTS idx_fleet_manifest_items_resource ON fleet_partner_manifest_items(organization_id,item_type,item_id);
CREATE TABLE IF NOT EXISTS fleet_partner_manifest_history (
 id text PRIMARY KEY,organization_id text NOT NULL,manifest_id text NOT NULL REFERENCES fleet_partner_manifests(id) ON DELETE RESTRICT,
 previous_status text NOT NULL,new_status text NOT NULL,reason_code text NOT NULL DEFAULT '',actor_type text NOT NULL,actor_reference text NOT NULL,
 correlation_id text NOT NULL DEFAULT '',occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_manifest_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_manifest_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
CREATE TABLE IF NOT EXISTS fleet_partner_manifest_snapshots (
 id text PRIMARY KEY,organization_id text NOT NULL,manifest_id text NOT NULL REFERENCES fleet_partner_manifests(id) ON DELETE RESTRICT,
 manifest_version bigint NOT NULL,snapshot jsonb NOT NULL,request_fingerprint text NOT NULL,created_by text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(manifest_id,manifest_version)
);
CREATE TRIGGER trg_fleet_manifest_snapshots_append_only BEFORE UPDATE OR DELETE ON fleet_partner_manifest_snapshots FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
INSERT INTO fleet_partner_permissions(code,category,description,is_active) VALUES
 ('manifests.read','manifests','Read manifests and snapshots',true),('manifests.manage','manifests','Create, edit and close manifests',true)
ON CONFLICT(code) DO UPDATE SET category=EXCLUDED.category,description=EXCLUDED.description,is_active=true;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code) SELECT 'rol_system_owner',code FROM fleet_partner_permissions WHERE code LIKE 'manifests.%' ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code) SELECT 'rol_system_operations',code FROM fleet_partner_permissions WHERE code LIKE 'manifests.%' ON CONFLICT DO NOTHING;
INSERT INTO fleet_partner_role_permissions(role_id,permission_code) SELECT 'rol_system_analyst',code FROM fleet_partner_permissions WHERE code='manifests.read' ON CONFLICT DO NOTHING;
