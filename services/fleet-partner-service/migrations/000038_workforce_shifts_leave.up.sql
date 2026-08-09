-- Sections 17.1: organization workforce, shifts, attendance and leave.
CREATE TABLE fleet_partner_workforce_profiles(
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 branch_id text NOT NULL REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT, membership_id text NOT NULL,
 employee_number text NOT NULL, relationship_type text NOT NULL CHECK(relationship_type IN('EMPLOYEE','CONTRACTOR','MANAGED_PROVIDER','TEMPORARY','OTHER')),
 role text NOT NULL, supervisor_membership_id text NOT NULL DEFAULT '', start_date date NOT NULL, end_date date,
 status text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN('ACTIVE','INACTIVE','ENDED','ARCHIVED')),
 version bigint NOT NULL DEFAULT 1 CHECK(version>0), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(organization_id,employee_number), UNIQUE(organization_id,membership_id), CHECK(end_date IS NULL OR end_date>=start_date)
);
CREATE INDEX idx_fleet_workforce_scope ON fleet_partner_workforce_profiles(organization_id,branch_id,status,role);
CREATE TABLE fleet_partner_shifts(
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 branch_id text NOT NULL REFERENCES fleet_partner_branches(id) ON DELETE RESTRICT, name text NOT NULL, role text NOT NULL,
 start_at timestamptz NOT NULL, end_at timestamptz NOT NULL, timezone text NOT NULL, required_capacity integer NOT NULL CHECK(required_capacity>0),
 status text NOT NULL DEFAULT 'DRAFT' CHECK(status IN('DRAFT','PUBLISHED','IN_PROGRESS','COMPLETED','CANCELLED')),
 planner_user_id text NOT NULL, version bigint NOT NULL DEFAULT 1 CHECK(version>0), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), CHECK(end_at>start_at)
);
CREATE INDEX idx_fleet_shifts_scope ON fleet_partner_shifts(organization_id,branch_id,start_at,status);
CREATE TABLE fleet_partner_shift_assignments(
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 shift_id text NOT NULL REFERENCES fleet_partner_shifts(id) ON DELETE RESTRICT, membership_id text NOT NULL,
 assignment_type text NOT NULL DEFAULT 'PRIMARY', status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','CONFIRMED','DECLINED','CHECKED_IN','CHECKED_OUT','NO_SHOW','CANCELLED')),
 expected_start timestamptz NOT NULL, expected_end timestamptz NOT NULL, checked_in_at timestamptz, checked_out_at timestamptz,
 check_in_source text NOT NULL DEFAULT '', check_out_source text NOT NULL DEFAULT '', version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(shift_id,membership_id), CHECK(expected_end>expected_start), CHECK(checked_out_at IS NULL OR checked_in_at IS NOT NULL AND checked_out_at>=checked_in_at)
);
CREATE INDEX idx_fleet_shift_assignments_member ON fleet_partner_shift_assignments(organization_id,membership_id,expected_start,expected_end);
CREATE TABLE fleet_partner_leave_records(
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES fleet_partner_organizations(id) ON DELETE RESTRICT,
 membership_id text NOT NULL, leave_type text NOT NULL, start_at timestamptz NOT NULL, end_at timestamptz NOT NULL, reason text NOT NULL,
 status text NOT NULL DEFAULT 'REQUESTED' CHECK(status IN('REQUESTED','APPROVED','REJECTED','CANCELLED','COMPLETED')),
 approved_by text NOT NULL DEFAULT '', approved_at timestamptz, version bigint NOT NULL DEFAULT 1 CHECK(version>0), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), CHECK(end_at>start_at)
);
CREATE INDEX idx_fleet_leave_member ON fleet_partner_leave_records(organization_id,membership_id,start_at,end_at,status);
CREATE TABLE fleet_partner_workforce_history(
 id text PRIMARY KEY,organization_id text NOT NULL,aggregate_type text NOT NULL,aggregate_id text NOT NULL,previous_status text NOT NULL DEFAULT '',new_status text NOT NULL,
 actor_id text NOT NULL,reason text NOT NULL DEFAULT '',version bigint NOT NULL,snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_fleet_workforce_history_append_only BEFORE UPDATE OR DELETE ON fleet_partner_workforce_history FOR EACH ROW EXECUTE FUNCTION fleet_partner_forbid_mutation();
