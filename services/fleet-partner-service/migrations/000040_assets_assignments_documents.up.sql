-- Section 18.1: organization asset master, allocation and Media references.
CREATE TABLE fleet_partner_assets(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,branch_id text NOT NULL REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT,
 asset_code text NOT NULL,asset_type text NOT NULL,platform_vehicle_id text NOT NULL DEFAULT '',ownership_type text NOT NULL,registration_reference text NOT NULL DEFAULT '',manufacturer text NOT NULL DEFAULT '',model text NOT NULL DEFAULT '',year integer,
 capacity numeric(18,3) NOT NULL DEFAULT 0,capacity_unit text NOT NULL DEFAULT '',odometer numeric(18,3) NOT NULL DEFAULT 0,operating_hours numeric(18,3) NOT NULL DEFAULT 0,
 operational_status text NOT NULL DEFAULT 'AVAILABLE' CHECK(operational_status IN('AVAILABLE','ASSIGNED','IN_USE','BLOCKED','UNDER_MAINTENANCE','OUT_OF_SERVICE')),
 lifecycle_status text NOT NULL DEFAULT 'ACTIVE' CHECK(lifecycle_status IN('ACTIVE','RETIRED','ARCHIVED')),version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),archived_at timestamptz,retired_at timestamptz,
 UNIQUE(organization_id,asset_code),CHECK(year IS NULL OR year BETWEEN 1900 AND 2200),CHECK(capacity>=0 AND odometer>=0 AND operating_hours>=0)
);
CREATE INDEX idx_fleet_assets_scope ON fleet_partner_assets(organization_id,branch_id,operational_status,lifecycle_status,asset_type);
CREATE UNIQUE INDEX uq_fleet_assets_platform_vehicle ON fleet_partner_assets(organization_id,platform_vehicle_id) WHERE platform_vehicle_id<>'';
CREATE TABLE fleet_partner_asset_assignments(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,asset_id text NOT NULL REFERENCES fleet_partner_assets(id) ON DELETE RESTRICT,
 provider_membership_id text NOT NULL,shift_id text REFERENCES fleet_partner_shifts(id) ON DELETE RESTRICT,assignment_type text NOT NULL,start_at timestamptz NOT NULL,end_at timestamptz,
 status text NOT NULL DEFAULT 'PLANNED' CHECK(status IN('PLANNED','ACTIVE','COMPLETED','CANCELLED')),version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),CHECK(end_at IS NULL OR end_at>start_at)
);
CREATE UNIQUE INDEX uq_fleet_asset_active_assignment ON fleet_partner_asset_assignments(asset_id) WHERE status='ACTIVE';
CREATE INDEX idx_fleet_asset_assignment_member ON fleet_partner_asset_assignments(organization_id,provider_membership_id,status,start_at,end_at);
CREATE TABLE fleet_partner_asset_documents(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,asset_id text NOT NULL REFERENCES fleet_partner_assets(id) ON DELETE RESTRICT,
 media_file_id text NOT NULL,document_type text NOT NULL,issued_at timestamptz,expires_at timestamptz,verification_reference text NOT NULL DEFAULT '',
 verification_status text NOT NULL DEFAULT 'PENDING' CHECK(verification_status IN('PENDING','VERIFIED','REJECTED','EXPIRED','REVOKED')),
 version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,asset_id,document_type,media_file_id),CHECK(expires_at IS NULL OR issued_at IS NULL OR expires_at>issued_at)
);
CREATE TABLE fleet_partner_asset_history(
 id text PRIMARY KEY,organization_id text NOT NULL,asset_id text NOT NULL,previous_operational_status text NOT NULL,new_operational_status text NOT NULL,
 previous_lifecycle_status text NOT NULL,new_lifecycle_status text NOT NULL,reason text NOT NULL,actor_id text NOT NULL,version bigint NOT NULL,snapshot jsonb NOT NULL DEFAULT '{}',occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_asset_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_asset_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
