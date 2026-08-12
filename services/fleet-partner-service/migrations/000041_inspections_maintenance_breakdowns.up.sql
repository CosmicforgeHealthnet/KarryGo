-- Section 18.2: inspections, immutable safety decisions, maintenance and breakdowns.
CREATE TABLE fleet_partner_inspection_templates(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,name text NOT NULL,
 asset_types jsonb NOT NULL DEFAULT '[]',service_types jsonb NOT NULL DEFAULT '[]',checklist_items jsonb NOT NULL,criticality_rules jsonb NOT NULL,
 status text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN('ACTIVE','INACTIVE','ARCHIVED')),version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE fleet_partner_inspections(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,asset_id text NOT NULL REFERENCES fleet_partner_assets(id) ON DELETE RESTRICT,
 template_id text NOT NULL REFERENCES fleet_partner_inspection_templates(id) ON DELETE RESTRICT,inspector_membership_id text NOT NULL,assignment_id text NOT NULL DEFAULT '',trip_reference text NOT NULL DEFAULT '',
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','IN_PROGRESS','COMPLETED','CANCELLED')),overall_result text CHECK(overall_result IN('PASS','PASS_WITH_ADVISORY','FAIL','CRITICAL_FAIL')),
 defect_summary jsonb NOT NULL DEFAULT '[]',evidence_media_ids jsonb NOT NULL DEFAULT '[]',started_at timestamptz,completed_at timestamptz,version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE fleet_partner_inspection_results(
 id text PRIMARY KEY,organization_id text NOT NULL,inspection_id text NOT NULL REFERENCES fleet_partner_inspections(id) ON DELETE RESTRICT,item_code text NOT NULL,
 answer jsonb NOT NULL,evidence_media_ids jsonb NOT NULL DEFAULT '[]',result text NOT NULL,critical boolean NOT NULL DEFAULT false,created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(inspection_id,item_code)
);
CREATE TRIGGER trg_fleet_inspection_results_append_only BEFORE UPDATE OR DELETE ON fleet_partner_inspection_results FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
CREATE TABLE fleet_partner_asset_safety_blocks(
 id text PRIMARY KEY,organization_id text NOT NULL,asset_id text NOT NULL REFERENCES fleet_partner_assets(id) ON DELETE RESTRICT,source_type text NOT NULL,source_reference text NOT NULL,
 status text NOT NULL CHECK(status IN('ACTIVE','CLEARED')),reason text NOT NULL,created_by text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),cleared_by text NOT NULL DEFAULT '',cleared_at timestamptz,clear_reason text NOT NULL DEFAULT '',UNIQUE(organization_id,source_type,source_reference)
);
CREATE TRIGGER trg_fleet_asset_safety_blocks_append_only BEFORE UPDATE OR DELETE ON fleet_partner_asset_safety_blocks FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
CREATE TABLE fleet_partner_maintenance_schedules(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,asset_id text NOT NULL REFERENCES fleet_partner_assets(id) ON DELETE RESTRICT,
 trigger_type text NOT NULL CHECK(trigger_type IN('DATE','MILEAGE','OPERATING_HOURS','COMBINED')),interval_value numeric(18,3),next_due_date date,next_due_odometer numeric(18,3),next_due_hours numeric(18,3),
 critical boolean NOT NULL DEFAULT false,status text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN('ACTIVE','PAUSED','COMPLETED','CANCELLED')),version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE fleet_partner_work_orders(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,asset_id text NOT NULL REFERENCES fleet_partner_assets(id) ON DELETE RESTRICT,
 source_type text NOT NULL,source_reference text NOT NULL DEFAULT '',priority text NOT NULL CHECK(priority IN('LOW','MEDIUM','HIGH','CRITICAL')),diagnosis text NOT NULL DEFAULT '',required_work text NOT NULL,
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','PENDING_APPROVAL','APPROVED','IN_PROGRESS','COMPLETED','CANCELLED')),approved_by text NOT NULL DEFAULT '',approved_at timestamptz,started_at timestamptz,completed_at timestamptz,
 downtime_start timestamptz,downtime_end timestamptz,evidence_media_ids jsonb NOT NULL DEFAULT '[]',return_to_service_status text NOT NULL DEFAULT 'NOT_REQUESTED' CHECK(return_to_service_status IN('NOT_REQUESTED','PENDING','APPROVED','REJECTED')),
 version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,source_type,source_reference)
);
CREATE TABLE fleet_partner_work_order_history(
 id text PRIMARY KEY,organization_id text NOT NULL,work_order_id text NOT NULL REFERENCES fleet_partner_work_orders(id) ON DELETE RESTRICT,previous_status text NOT NULL,new_status text NOT NULL,
 actor_id text NOT NULL,reason text NOT NULL DEFAULT '',version bigint NOT NULL,occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_work_order_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_work_order_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
CREATE TABLE fleet_partner_return_to_service_decisions(
 id text PRIMARY KEY,organization_id text NOT NULL,work_order_id text NOT NULL REFERENCES fleet_partner_work_orders(id) ON DELETE RESTRICT,asset_id text NOT NULL REFERENCES fleet_partner_assets(id) ON DELETE RESTRICT,
 decision text NOT NULL CHECK(decision IN('APPROVED','REJECTED')),reason text NOT NULL,evidence_media_ids jsonb NOT NULL,decided_by text NOT NULL,decided_at timestamptz NOT NULL DEFAULT now(),UNIQUE(work_order_id)
);
CREATE TRIGGER trg_fleet_return_to_service_append_only BEFORE UPDATE OR DELETE ON fleet_partner_return_to_service_decisions FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
CREATE TABLE fleet_partner_breakdowns(
 id text PRIMARY KEY,organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,asset_id text NOT NULL REFERENCES fleet_partner_assets(id) ON DELETE RESTRICT,
 provider_membership_id text NOT NULL,trip_reference text NOT NULL DEFAULT '',location_reference text NOT NULL,occurred_at timestamptz NOT NULL,severity text NOT NULL CHECK(severity IN('LOW','MEDIUM','HIGH','CRITICAL')),
 trip_impact text NOT NULL,rescue_status text NOT NULL DEFAULT 'REQUESTED',towing_reference text NOT NULL DEFAULT '',replacement_asset_id text REFERENCES fleet_partner_assets(id) ON DELETE RESTRICT,
 status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','IN_PROGRESS','RESOLVED','CANCELLED')),version bigint NOT NULL DEFAULT 1,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE fleet_partner_safety_decline_inbox(
 event_id text PRIMARY KEY,organization_id text NOT NULL,assignment_id text NOT NULL,asset_reference text NOT NULL,reason_code text NOT NULL,payload_hash text NOT NULL,processed_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,assignment_id,reason_code)
);
