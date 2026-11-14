// Package repositories owns the workforce feature's SQL. Every statement is
// organization-scoped. Each method takes the querier it runs on, so a use case
// decides whether a statement runs inside its transaction or on the pool.
package repositories

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
)

// Repository is the Postgres implementation of the workforce persistence.
type Repository struct{}

// NewRepository builds the repository.
func NewRepository() *Repository { return &Repository{} }

// limit clamps a page size to the workforce default (50) and maximum (200).
func limit(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}

// keysetLimit asks for one row more than the page so the caller can tell
// whether a further page exists.
func keysetLimit(n int) int { return limit(n) + 1 }

// branchScope is the SQL array of permitted branches; empty means all.
func branchScope(q models.ListQuery) []string {
	if q.BranchScope == nil {
		return []string{}
	}
	return q.BranchScope
}

// ---- Shared checks ----

// MemberActive reports whether id is an active provider membership or
// organization user of the organization. A lookup failure reads as false.
func (r *Repository) MemberActive(ctx context.Context, q database.Querier, org, id string) bool {
	var ok bool
	_ = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1 AND id=$2 AND status='ACTIVE') OR EXISTS(SELECT 1 FROM fleet_partner_organization_users WHERE organization_id=$1 AND id=$2 AND status='ACTIVE')`, org, id).Scan(&ok)
	return ok
}

// BranchActive reports whether the branch is active. A lookup failure reads
// as false.
func (r *Repository) BranchActive(ctx context.Context, q database.Querier, org, branch string) bool {
	var ok bool
	_ = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_branches WHERE organization_id=$1 AND id=$2 AND status='ACTIVE')`, org, branch).Scan(&ok)
	return ok
}

// InsertHistory appends one workforce status-history row.
func (r *Repository) InsertHistory(ctx context.Context, q database.Querier, org, kind, id, from, to, actorID, reason string, version int64, snapshot []byte) error {
	_, e := q.Exec(ctx, `INSERT INTO fleet_partner_workforce_history(id,organization_id,aggregate_type,aggregate_id,previous_status,new_status,actor_id,reason,version,snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New("wfh"), org, kind, id, from, to, actorID, reason, version, snapshot)
	return e
}

// LockMemberSchedule serializes schedule changes for one member until the
// transaction ends.
func (r *Repository) LockMemberSchedule(ctx context.Context, q database.Querier, org, membership string) error {
	_, e := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, org+":"+membership)
	return e
}

// ---- Profiles ----

const profileCols = `id,organization_id,branch_id,membership_id,employee_number,relationship_type,role,supervisor_membership_id,status,start_date,end_date,version,created_at,updated_at`

func scanProfile(r pgx.Row) (models.Profile, error) {
	var p models.Profile
	e := r.Scan(&p.ID, &p.OrganizationID, &p.BranchID, &p.MembershipID, &p.EmployeeNumber, &p.RelationshipType, &p.Role, &p.SupervisorMembershipID, &p.Status, &p.StartDate, &p.EndDate, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	return p, e
}

// InsertProfile stores a new workforce profile.
func (r *Repository) InsertProfile(ctx context.Context, q database.Querier, p models.Profile) (models.Profile, error) {
	return scanProfile(q.QueryRow(ctx, `INSERT INTO fleet_partner_workforce_profiles(id,organization_id,branch_id,membership_id,employee_number,relationship_type,role,supervisor_membership_id,start_date) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+profileCols, ids.New("wfp"), p.OrganizationID, p.BranchID, p.MembershipID, p.EmployeeNumber, p.RelationshipType, p.Role, p.SupervisorMembershipID, p.StartDate))
}

// GetProfile loads one profile.
func (r *Repository) GetProfile(ctx context.Context, q database.Querier, org, id string) (models.Profile, error) {
	return scanProfile(q.QueryRow(ctx, `SELECT `+profileCols+` FROM fleet_partner_workforce_profiles WHERE organization_id=$1 AND id=$2`, org, id))
}

// ListProfiles returns one page of profiles, newest first.
func (r *Repository) ListProfiles(ctx context.Context, q database.Querier, org string, f models.ListQuery) ([]models.Profile, error) {
	var rows pgx.Rows
	var e error
	if f.Keyset {
		rows, e = q.Query(ctx, `SELECT `+profileCols+` FROM fleet_partner_workforce_profiles WHERE organization_id=$1 AND($2='' OR branch_id=$2)AND($3='' OR status=$3)
			AND (COALESCE(cardinality($5::text[]),0)=0 OR branch_id=ANY($5::text[]))
			AND ($6::timestamptz IS NULL OR (created_at,id) < ($6::timestamptz,$7))
			ORDER BY created_at DESC, id DESC LIMIT $4`, org, f.BranchID, f.Status, keysetLimit(f.Limit), branchScope(f), f.After, f.AfterID)
	} else {
		rows, e = q.Query(ctx, `SELECT `+profileCols+` FROM fleet_partner_workforce_profiles WHERE organization_id=$1 AND($2='' OR branch_id=$2)AND($3='' OR status=$3) ORDER BY created_at DESC LIMIT $4 OFFSET $5`, org, f.BranchID, f.Status, limit(f.Limit), pagination.ClampOffset(f.Offset))
	}
	if e != nil {
		return nil, e
	}
	return collect(rows, scanProfile)
}

// UpdateProfile applies a guarded profile change; pgx.ErrNoRows means the
// version did not match.
func (r *Repository) UpdateProfile(ctx context.Context, q database.Querier, org, id string, expected int64, role, supervisor, status string) (models.Profile, error) {
	return scanProfile(q.QueryRow(ctx, `UPDATE fleet_partner_workforce_profiles SET role=COALESCE(NULLIF($4,''),role),supervisor_membership_id=CASE WHEN $5<>'' THEN $5 ELSE supervisor_membership_id END,status=$6,end_date=CASE WHEN $6='ENDED' THEN CURRENT_DATE ELSE end_date END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+profileCols, org, id, expected, role, supervisor, status))
}

// ActiveProfileMemberships lists the memberships with an active profile.
func (r *Repository) ActiveProfileMemberships(ctx context.Context, q database.Querier, org string) ([]string, error) {
	rows, e := q.Query(ctx, `SELECT membership_id FROM fleet_partner_workforce_profiles WHERE organization_id=$1 AND status='ACTIVE'`, org)
	if e != nil {
		return nil, e
	}
	members := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		members = append(members, id)
	}
	rows.Close()
	return members, nil
}

// ---- Shifts ----

const shiftCols = `id,organization_id,branch_id,name,role,start_at,end_at,timezone,required_capacity,status,planner_user_id,version,created_at,updated_at`

func scanShift(r pgx.Row) (models.Shift, error) {
	var v models.Shift
	e := r.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.Name, &v.Role, &v.StartAt, &v.EndAt, &v.Timezone, &v.RequiredCapacity, &v.Status, &v.PlannerUserID, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// InsertShift stores a new DRAFT shift.
func (r *Repository) InsertShift(ctx context.Context, q database.Querier, v models.Shift) (models.Shift, error) {
	return scanShift(q.QueryRow(ctx, `INSERT INTO fleet_partner_shifts(id,organization_id,branch_id,name,role,start_at,end_at,timezone,required_capacity,planner_user_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)RETURNING `+shiftCols, ids.New("sft"), v.OrganizationID, v.BranchID, v.Name, v.Role, v.StartAt, v.EndAt, v.Timezone, v.RequiredCapacity, v.PlannerUserID))
}

// InsertTemplateShift stores one shift generated from a template for a local
// date. pgx.ErrNoRows means that day was already generated.
func (r *Repository) InsertTemplateShift(ctx context.Context, q database.Querier, tpl models.ShiftTemplate, start, end time.Time, plannerUserID, localDate string) (models.Shift, error) {
	return scanShift(q.QueryRow(ctx, `INSERT INTO fleet_partner_shifts(id,organization_id,branch_id,name,role,start_at,end_at,timezone,required_capacity,planner_user_id,template_id,template_local_date)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
				ON CONFLICT (template_id, template_local_date) WHERE template_id IS NOT NULL DO NOTHING
				RETURNING `+shiftCols,
		ids.New("sft"), tpl.OrganizationID, tpl.BranchID, tpl.Name, tpl.Role, start, end, tpl.Timezone, tpl.RequiredCapacity, plannerUserID, tpl.ID, localDate))
}

// GetShift loads one shift.
func (r *Repository) GetShift(ctx context.Context, q database.Querier, org, id string) (models.Shift, error) {
	return scanShift(q.QueryRow(ctx, `SELECT `+shiftCols+` FROM fleet_partner_shifts WHERE organization_id=$1 AND id=$2`, org, id))
}

// ListShifts returns one page of shifts, latest start first.
func (r *Repository) ListShifts(ctx context.Context, q database.Querier, org string, f models.ListQuery) ([]models.Shift, error) {
	var rows pgx.Rows
	var e error
	if f.Keyset {
		rows, e = q.Query(ctx, `SELECT `+shiftCols+` FROM fleet_partner_shifts WHERE organization_id=$1 AND($2=''OR branch_id=$2)AND($3=''OR status=$3)
			AND (COALESCE(cardinality($5::text[]),0)=0 OR branch_id=ANY($5::text[]))
			AND ($6::timestamptz IS NULL OR (start_at,id) < ($6::timestamptz,$7))
			ORDER BY start_at DESC, id DESC LIMIT $4`, org, f.BranchID, f.Status, keysetLimit(f.Limit), branchScope(f), f.After, f.AfterID)
	} else {
		rows, e = q.Query(ctx, `SELECT `+shiftCols+` FROM fleet_partner_shifts WHERE organization_id=$1 AND($2=''OR branch_id=$2)AND($3=''OR status=$3)ORDER BY start_at DESC LIMIT $4 OFFSET $5`, org, f.BranchID, f.Status, limit(f.Limit), pagination.ClampOffset(f.Offset))
	}
	if e != nil {
		return nil, e
	}
	return collect(rows, scanShift)
}

// UpdateShift applies a guarded shift change; pgx.ErrNoRows means a version
// conflict or an empty time window.
func (r *Repository) UpdateShift(ctx context.Context, q database.Querier, org, id string, expected int64, name string, start, end *time.Time, to string) (models.Shift, error) {
	return scanShift(q.QueryRow(ctx, `UPDATE fleet_partner_shifts SET name=COALESCE(NULLIF($4,''),name),start_at=COALESCE($5,start_at),end_at=COALESCE($6,end_at),status=$7,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND COALESCE($6,end_at)>COALESCE($5,start_at) RETURNING `+shiftCols, org, id, expected, name, start, end, to))
}

// LockShift locks a shift row until the transaction ends.
func (r *Repository) LockShift(ctx context.Context, q database.Querier, org, shiftID string) error {
	_, e := q.Exec(ctx, `SELECT id FROM fleet_partner_shifts WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, shiftID)
	return e
}

// ---- Shift assignments ----

const assignmentCols = `id,organization_id,shift_id,membership_id,assignment_type,status,expected_start,expected_end,checked_in_at,checked_out_at,check_in_source,check_out_source,version,created_at,updated_at`

func scanAssignment(r pgx.Row) (models.ShiftAssignment, error) {
	var v models.ShiftAssignment
	e := r.Scan(&v.ID, &v.OrganizationID, &v.ShiftID, &v.MembershipID, &v.AssignmentType, &v.Status, &v.ExpectedStart, &v.ExpectedEnd, &v.CheckedInAt, &v.CheckedOutAt, &v.CheckInSource, &v.CheckOutSource, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// ScheduleConflict reports whether the member already has live work or
// approved leave overlapping [start, end).
func (r *Repository) ScheduleConflict(ctx context.Context, q database.Querier, org, membership string, start, end time.Time) (bool, error) {
	var conflict bool
	e := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_shift_assignments WHERE organization_id=$1 AND membership_id=$2 AND status IN('PENDING','CONFIRMED','CHECKED_IN') AND tstzrange(expected_start,expected_end,'[)')&&tstzrange($3,$4,'[)')) OR EXISTS(SELECT 1 FROM fleet_partner_leave_records WHERE organization_id=$1 AND membership_id=$2 AND status='APPROVED' AND tstzrange(start_at,end_at,'[)')&&tstzrange($3,$4,'[)'))`, org, membership, start, end).Scan(&conflict)
	return conflict, e
}

// AssignmentConflict reports whether the member has live shift work
// overlapping [start, end).
func (r *Repository) AssignmentConflict(ctx context.Context, q database.Querier, org, membership string, start, end time.Time) (bool, error) {
	var conflict bool
	e := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_shift_assignments WHERE organization_id=$1 AND membership_id=$2 AND status IN('PENDING','CONFIRMED','CHECKED_IN') AND tstzrange(expected_start,expected_end,'[)')&&tstzrange($3,$4,'[)'))`, org, membership, start, end).Scan(&conflict)
	return conflict, e
}

// ActiveAssignmentCount counts the shift's assignments that still hold a slot.
func (r *Repository) ActiveAssignmentCount(ctx context.Context, q database.Querier, shiftID string) (int, error) {
	var count int
	e := q.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_shift_assignments WHERE shift_id=$1 AND status NOT IN('DECLINED','CANCELLED')`, shiftID).Scan(&count)
	return count, e
}

// InsertAssignment stores a CONFIRMED shift assignment.
func (r *Repository) InsertAssignment(ctx context.Context, q database.Querier, v models.ShiftAssignment) (models.ShiftAssignment, error) {
	return scanAssignment(q.QueryRow(ctx, `INSERT INTO fleet_partner_shift_assignments(id,organization_id,shift_id,membership_id,assignment_type,status,expected_start,expected_end)VALUES($1,$2,$3,$4,$5,'CONFIRMED',$6,$7)RETURNING `+assignmentCols, ids.New("sfa"), v.OrganizationID, v.ShiftID, v.MembershipID, v.AssignmentType, v.ExpectedStart, v.ExpectedEnd))
}

// GetAssignment loads one shift assignment.
func (r *Repository) GetAssignment(ctx context.Context, q database.Querier, org, id string) (models.ShiftAssignment, error) {
	return scanAssignment(q.QueryRow(ctx, `SELECT `+assignmentCols+` FROM fleet_partner_shift_assignments WHERE organization_id=$1 AND id=$2`, org, id))
}

// IsSupervisor reports whether either actor identity supervises the member.
func (r *Repository) IsSupervisor(ctx context.Context, q database.Querier, org, membership, organizationUserID, actorID string) (bool, error) {
	var supervisor bool
	e := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_workforce_profiles WHERE organization_id=$1 AND membership_id=$2 AND supervisor_membership_id IN($3,$4))`, org, membership, organizationUserID, actorID).Scan(&supervisor)
	return supervisor, e
}

// UpdateAssignmentStatus applies a guarded attendance transition.
func (r *Repository) UpdateAssignmentStatus(ctx context.Context, q database.Querier, org, id string, expected int64, to, source string) (models.ShiftAssignment, error) {
	return scanAssignment(q.QueryRow(ctx, `UPDATE fleet_partner_shift_assignments SET status=$4,checked_in_at=CASE WHEN $4='CHECKED_IN' THEN now() ELSE checked_in_at END,checked_out_at=CASE WHEN $4='CHECKED_OUT' THEN now() ELSE checked_out_at END,check_in_source=CASE WHEN $4='CHECKED_IN' THEN $5 ELSE check_in_source END,check_out_source=CASE WHEN $4='CHECKED_OUT' THEN $5 ELSE check_out_source END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+assignmentCols, org, id, expected, to, source))
}

// ---- Leave ----

const leaveCols = `id,organization_id,membership_id,leave_type,start_at,end_at,reason,status,approved_by,approved_at,version,created_at,updated_at`

func scanLeave(r pgx.Row) (models.Leave, error) {
	var v models.Leave
	e := r.Scan(&v.ID, &v.OrganizationID, &v.MembershipID, &v.LeaveType, &v.StartAt, &v.EndAt, &v.Reason, &v.Status, &v.ApprovedBy, &v.ApprovedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// InsertLeave stores a REQUESTED leave record.
func (r *Repository) InsertLeave(ctx context.Context, q database.Querier, v models.Leave) (models.Leave, error) {
	return scanLeave(q.QueryRow(ctx, `INSERT INTO fleet_partner_leave_records(id,organization_id,membership_id,leave_type,start_at,end_at,reason)VALUES($1,$2,$3,$4,$5,$6,$7)RETURNING `+leaveCols, ids.New("lve"), v.OrganizationID, v.MembershipID, v.LeaveType, v.StartAt, v.EndAt, v.Reason))
}

// GetLeave loads one leave record.
func (r *Repository) GetLeave(ctx context.Context, q database.Querier, org, id string) (models.Leave, error) {
	return scanLeave(q.QueryRow(ctx, `SELECT `+leaveCols+` FROM fleet_partner_leave_records WHERE organization_id=$1 AND id=$2`, org, id))
}

// ListLeave returns one page of leave, latest start first.
func (r *Repository) ListLeave(ctx context.Context, q database.Querier, org string, f models.ListQuery) ([]models.Leave, error) {
	var rows pgx.Rows
	var e error
	if f.Keyset {
		rows, e = q.Query(ctx, `SELECT `+leaveCols+` FROM fleet_partner_leave_records WHERE organization_id=$1 AND($2=''OR status=$2)
			AND ($4::timestamptz IS NULL OR (start_at,id) < ($4::timestamptz,$5))
			ORDER BY start_at DESC, id DESC LIMIT $3`, org, f.Status, keysetLimit(f.Limit), f.After, f.AfterID)
	} else {
		rows, e = q.Query(ctx, `SELECT `+leaveCols+` FROM fleet_partner_leave_records WHERE organization_id=$1 AND($2=''OR status=$2)ORDER BY start_at DESC LIMIT $3 OFFSET $4`, org, f.Status, limit(f.Limit), pagination.ClampOffset(f.Offset))
	}
	if e != nil {
		return nil, e
	}
	return collect(rows, scanLeave)
}

// UpdateLeaveStatus applies a guarded leave transition.
func (r *Repository) UpdateLeaveStatus(ctx context.Context, q database.Querier, org, id string, expected int64, to, approver string) (models.Leave, error) {
	return scanLeave(q.QueryRow(ctx, `UPDATE fleet_partner_leave_records SET status=$4,approved_by=CASE WHEN $4='APPROVED' THEN $5 ELSE approved_by END,approved_at=CASE WHEN $4='APPROVED' THEN now() ELSE approved_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+leaveCols, org, id, expected, to, approver))
}

// ---- Qualifications ----

const qualificationCols = `id,organization_id,code,name,applicable_roles,applicable_services,applicable_asset_types,validity_period_seconds,evidence_required,status,version,created_at,updated_at`

func scanQualification(r pgx.Row) (models.Qualification, error) {
	var v models.Qualification
	var a, b, c []byte
	if e := r.Scan(&v.ID, &v.OrganizationID, &v.Code, &v.Name, &a, &b, &c, &v.ValidityPeriodSeconds, &v.EvidenceRequired, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt); e != nil {
		return v, e
	}
	_ = json.Unmarshal(a, &v.ApplicableRoles)
	_ = json.Unmarshal(b, &v.ApplicableServices)
	_ = json.Unmarshal(c, &v.ApplicableAssetTypes)
	return v, nil
}

// InsertQualification stores a qualification definition and returns it with
// its timestamps.
func (r *Repository) InsertQualification(ctx context.Context, q database.Querier, v models.Qualification) (models.Qualification, error) {
	roles, _ := json.Marshal(v.ApplicableRoles)
	services, _ := json.Marshal(v.ApplicableServices)
	assets, _ := json.Marshal(v.ApplicableAssetTypes)
	e := q.QueryRow(ctx, `INSERT INTO fleet_partner_qualifications(id,organization_id,code,name,applicable_roles,applicable_services,applicable_asset_types,validity_period_seconds,evidence_required)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)RETURNING created_at,updated_at`, v.ID, v.OrganizationID, v.Code, v.Name, roles, services, assets, v.ValidityPeriodSeconds, v.EvidenceRequired).Scan(&v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// ListQualifications returns every qualification definition, by code.
func (r *Repository) ListQualifications(ctx context.Context, q database.Querier, org string) ([]models.Qualification, error) {
	rows, e := q.Query(ctx, `SELECT `+qualificationCols+` FROM fleet_partner_qualifications WHERE organization_id=$1 ORDER BY code`, org)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanQualification)
}

// ListQualificationsPage returns one keyset page by (code, id); AfterID names
// the last row of the previous page.
func (r *Repository) ListQualificationsPage(ctx context.Context, q database.Querier, org string, f models.ListQuery) ([]models.Qualification, error) {
	rows, e := q.Query(ctx, `SELECT `+qualificationCols+` FROM fleet_partner_qualifications WHERE organization_id=$1
		AND ($3='' OR (code,id) > ((SELECT code FROM fleet_partner_qualifications WHERE organization_id=$1 AND id=$3),$3))
		ORDER BY code, id LIMIT $2`, org, keysetLimit(f.Limit), f.AfterID)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanQualification)
}

// UpdateQualification stores a guarded qualification change and returns the
// new version and update time.
func (r *Repository) UpdateQualification(ctx context.Context, q database.Querier, org, id string, expected int64, v models.Qualification) (int64, time.Time, error) {
	roles, _ := json.Marshal(nonNilStrings(v.ApplicableRoles))
	services, _ := json.Marshal(nonNilStrings(v.ApplicableServices))
	assets, _ := json.Marshal(nonNilStrings(v.ApplicableAssetTypes))
	var version int64
	var updated time.Time
	e := q.QueryRow(ctx, `UPDATE fleet_partner_qualifications SET name=$4,applicable_roles=$5,applicable_services=$6,applicable_asset_types=$7,evidence_required=$8,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING version,updated_at`, org, id, expected, v.Name, roles, services, assets, v.EvidenceRequired).Scan(&version, &updated)
	return version, updated, e
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

const providerQualificationCols = `id,organization_id,membership_id,qualification_id,evidence_media_id,verification_reference,issued_at,expires_at,status,verified_by,verified_at,version,created_at,updated_at`

func scanProviderQualification(r pgx.Row) (models.ProviderQualification, error) {
	var v models.ProviderQualification
	e := r.Scan(&v.ID, &v.OrganizationID, &v.MembershipID, &v.QualificationID, &v.EvidenceMediaID, &v.VerificationReference, &v.IssuedAt, &v.ExpiresAt, &v.Status, &v.VerifiedBy, &v.VerifiedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// InsertProviderQualification stores a provider's qualification claim.
func (r *Repository) InsertProviderQualification(ctx context.Context, q database.Querier, v models.ProviderQualification) (models.ProviderQualification, error) {
	e := q.QueryRow(ctx, `INSERT INTO fleet_partner_provider_qualifications(id,organization_id,membership_id,qualification_id,evidence_media_id,issued_at,expires_at)VALUES($1,$2,$3,$4,$5,$6,$7)RETURNING version,created_at,updated_at`, v.ID, v.OrganizationID, v.MembershipID, v.QualificationID, v.EvidenceMediaID, v.IssuedAt, v.ExpiresAt).Scan(&v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// ListProviderQualifications returns a member's qualification records.
func (r *Repository) ListProviderQualifications(ctx context.Context, q database.Querier, org, membership string) ([]models.ProviderQualification, error) {
	rows, e := q.Query(ctx, `SELECT `+providerQualificationCols+` FROM fleet_partner_provider_qualifications WHERE organization_id=$1 AND membership_id=$2`, org, membership)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanProviderQualification)
}

// UpdateProviderQualificationStatus applies a guarded verification decision.
func (r *Repository) UpdateProviderQualificationStatus(ctx context.Context, q database.Querier, org, id string, expected int64, to, reference, verifier string) (models.ProviderQualification, error) {
	return scanProviderQualification(q.QueryRow(ctx, `UPDATE fleet_partner_provider_qualifications SET status=$4,verification_reference=CASE WHEN $4='VERIFIED' THEN $5 ELSE verification_reference END,verified_by=$6,verified_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND status IN('PENDING_VERIFICATION','VERIFIED') RETURNING `+providerQualificationCols, org, id, expected, to, reference, verifier))
}

// ---- Training courses and completions ----

const courseCols = `id,organization_id,code,title,provider_reference,required_roles,required_services,validity_period_seconds,passing_score,status,version,created_at,updated_at`

func scanCourse(r pgx.Row) (models.Course, error) {
	var v models.Course
	var a, b []byte
	if e := r.Scan(&v.ID, &v.OrganizationID, &v.Code, &v.Title, &v.ProviderReference, &a, &b, &v.ValidityPeriodSeconds, &v.PassingScore, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt); e != nil {
		return v, e
	}
	_ = json.Unmarshal(a, &v.RequiredRoles)
	_ = json.Unmarshal(b, &v.RequiredServices)
	return v, nil
}

// InsertCourse stores a training course.
func (r *Repository) InsertCourse(ctx context.Context, q database.Querier, v models.Course) (models.Course, error) {
	roles, _ := json.Marshal(v.RequiredRoles)
	services, _ := json.Marshal(v.RequiredServices)
	e := q.QueryRow(ctx, `INSERT INTO fleet_partner_training_courses(id,organization_id,code,title,provider_reference,required_roles,required_services,validity_period_seconds,passing_score)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)RETURNING version,created_at,updated_at`, v.ID, v.OrganizationID, v.Code, v.Title, v.ProviderReference, roles, services, v.ValidityPeriodSeconds, v.PassingScore).Scan(&v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// ListCourses returns every training course, by code.
func (r *Repository) ListCourses(ctx context.Context, q database.Querier, org string) ([]models.Course, error) {
	rows, e := q.Query(ctx, `SELECT `+courseCols+` FROM fleet_partner_training_courses WHERE organization_id=$1 ORDER BY code`, org)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanCourse)
}

// ListCoursesPage returns one keyset page by (code, id); AfterID names the
// last row of the previous page.
func (r *Repository) ListCoursesPage(ctx context.Context, q database.Querier, org string, f models.ListQuery) ([]models.Course, error) {
	rows, e := q.Query(ctx, `SELECT `+courseCols+` FROM fleet_partner_training_courses WHERE organization_id=$1
		AND ($3='' OR (code,id) > ((SELECT code FROM fleet_partner_training_courses WHERE organization_id=$1 AND id=$3),$3))
		ORDER BY code, id LIMIT $2`, org, keysetLimit(f.Limit), f.AfterID)
	if e != nil {
		return nil, e
	}
	return collect(rows, scanCourse)
}

// UpdateCourse stores a guarded course change and returns the new version and
// update time.
func (r *Repository) UpdateCourse(ctx context.Context, q database.Querier, org, id string, expected int64, title string, passingScore float64) (int64, time.Time, error) {
	var version int64
	var updated time.Time
	e := q.QueryRow(ctx, `UPDATE fleet_partner_training_courses SET title=$4,passing_score=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING version,updated_at`, org, id, expected, title, passingScore).Scan(&version, &updated)
	return version, updated, e
}

// InsertCompletion stores one course result.
func (r *Repository) InsertCompletion(ctx context.Context, q database.Querier, v models.Completion, verifier string) (models.Completion, error) {
	e := q.QueryRow(ctx, `INSERT INTO fleet_partner_training_completions(id,organization_id,course_id,membership_id,score,status,certificate_media_id,completed_at,expires_at,verified_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)RETURNING version,created_at,updated_at`, v.ID, v.OrganizationID, v.CourseID, v.MembershipID, v.Score, v.Status, v.CertificateMediaID, v.CompletedAt, v.ExpiresAt, verifier).Scan(&v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// ---- Eligibility ----

// ProfileRoleStatus returns the member's workforce role and status.
func (r *Repository) ProfileRoleStatus(ctx context.Context, q database.Querier, org, membership string) (string, string, error) {
	var role, status string
	e := q.QueryRow(ctx, `SELECT role,status FROM fleet_partner_workforce_profiles WHERE organization_id=$1 AND membership_id=$2`, org, membership).Scan(&role, &status)
	return role, status, e
}

// OnApprovedLeave reports whether the member is on approved leave at at.
func (r *Repository) OnApprovedLeave(ctx context.Context, q database.Querier, org, membership string, at time.Time) (bool, error) {
	var leave bool
	e := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_leave_records WHERE organization_id=$1 AND membership_id=$2 AND status='APPROVED' AND $3>=start_at AND $3<end_at)`, org, membership, at).Scan(&leave)
	return leave, e
}

// ShiftCoverage reports whether any shift is running at at (so shift policy
// applies) and whether the member holds a confirmed assignment then.
func (r *Repository) ShiftCoverage(ctx context.Context, q database.Querier, org, membership string, at time.Time) (bool, bool, error) {
	var shiftPolicy, shiftOK bool
	e := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_shifts WHERE organization_id=$1 AND status IN('PUBLISHED','IN_PROGRESS') AND $3>=start_at AND $3<end_at),EXISTS(SELECT 1 FROM fleet_partner_shift_assignments a JOIN fleet_partner_shifts s ON s.id=a.shift_id WHERE a.organization_id=$1 AND a.membership_id=$2 AND a.status IN('CONFIRMED','CHECKED_IN') AND $3>=a.expected_start AND $3<a.expected_end)`, org, membership, at).Scan(&shiftPolicy, &shiftOK)
	return shiftPolicy, shiftOK, e
}

// ApplicableQualificationIDs lists the active qualifications required for
// the service, asset type or role. Row scan failures are skipped, as the
// eligibility check always has.
func (r *Repository) ApplicableQualificationIDs(ctx context.Context, q database.Querier, org, service, assetType, role string) ([]string, error) {
	rows, e := q.Query(ctx, `SELECT id,evidence_required FROM fleet_partner_qualifications WHERE organization_id=$1 AND status='ACTIVE' AND(($2<>'' AND applicable_services ? $2)OR($3<>'' AND applicable_asset_types ? $3)OR($4<>'' AND applicable_roles ? $4))`, org, service, assetType, role)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		var evidence bool
		_ = rows.Scan(&id, &evidence)
		out = append(out, id)
	}
	return out, nil
}

// HasVerifiedQualification reports whether the member holds a verified,
// unexpired record of the qualification at at.
func (r *Repository) HasVerifiedQualification(ctx context.Context, q database.Querier, org, membership, qualificationID string, at time.Time) (bool, error) {
	var ok bool
	e := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_provider_qualifications WHERE organization_id=$1 AND membership_id=$2 AND qualification_id=$3 AND status='VERIFIED' AND(expires_at IS NULL OR expires_at>$4))`, org, membership, qualificationID, at).Scan(&ok)
	return ok, e
}

// ApplicableCourseIDs lists the active courses required for the service or
// role. Row scan failures are skipped, as the eligibility check always has.
func (r *Repository) ApplicableCourseIDs(ctx context.Context, q database.Querier, org, service, role string) ([]string, error) {
	rows, e := q.Query(ctx, `SELECT id FROM fleet_partner_training_courses WHERE organization_id=$1 AND status='ACTIVE' AND(($2<>'' AND required_services ? $2)OR($3<>'' AND required_roles ? $3))`, org, service, role)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		out = append(out, id)
	}
	return out, nil
}

// HasPassedCourse reports whether the member holds a passed, unexpired
// completion of the course at at.
func (r *Repository) HasPassedCourse(ctx context.Context, q database.Querier, org, membership, courseID string, at time.Time) (bool, error) {
	var ok bool
	e := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_training_completions WHERE organization_id=$1 AND membership_id=$2 AND course_id=$3 AND status='PASSED' AND(expires_at IS NULL OR expires_at>$4))`, org, membership, courseID, at).Scan(&ok)
	return ok, e
}

// ---- Performance ----

// AssignmentRecords returns every marketplace and private company assignment
// created for the member in [start, end).
func (r *Repository) AssignmentRecords(ctx context.Context, q database.Querier, org, membership string, start, end time.Time) ([]models.AssignmentRecord, error) {
	rows, e := q.Query(ctx, `
		SELECT 'MARKETPLACE', a.id, a.status, a.accepted_at IS NOT NULL, a.declined_at IS NOT NULL OR r.id IS NOT NULL,
		       a.expired_at IS NOT NULL, COALESCE(r.reason_code, ''), COALESCE(r.safety_related, false), a.version
		FROM fleet_partner_provider_assignments a
		LEFT JOIN fleet_partner_assignment_responses r ON r.assignment_id = a.id AND r.response = 'DECLINED'
		WHERE a.organization_id = $1 AND a.membership_id = $2 AND a.created_at >= $3 AND a.created_at < $4
		UNION ALL
		SELECT 'PRIVATE', p.id, p.status, p.accepted_at IS NOT NULL, p.declined_at IS NOT NULL,
		       p.expired_at IS NOT NULL, p.decline_reason, COALESCE(d.safety_protected, false), p.version
		FROM fleet_partner_private_assignments p
		LEFT JOIN fleet_partner_private_assignment_decisions d ON d.assignment_id = p.id AND d.decision = 'DECLINE'
		WHERE p.organization_id = $1 AND p.provider_membership_id = $2 AND p.created_at >= $3 AND p.created_at < $4
		ORDER BY 1, 2`, org, membership, start, end)
	if e != nil {
		return nil, e
	}
	out := []models.AssignmentRecord{}
	for rows.Next() {
		var a models.AssignmentRecord
		if e = rows.Scan(&a.Origin, &a.AssignmentID, &a.SourceStatus, &a.Accepted, &a.Declined, &a.Expired, &a.Reason, &a.SafetyRelated, &a.Version); e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, a)
	}
	rows.Close()
	return out, rows.Err()
}

const performanceColumns = `id,organization_id,membership_id,period_start,period_end,metric_inputs,calculated_metrics,explanation,source_versions,generation_status,formula_version,generated_at,version`

func performanceScanTargets(v *models.PerformanceSnapshot) []any {
	return []any{&v.ID, &v.OrganizationID, &v.MembershipID, &v.PeriodStart, &v.PeriodEnd, &v.MetricInputs,
		&v.CalculatedMetrics, &v.Explanation, &v.SourceVersions, &v.GenerationStatus, &v.FormulaVersion, &v.GeneratedAt, &v.Version}
}

// InsertPerformanceSnapshot stores a snapshot once per (membership, period,
// formula). pgx.ErrNoRows means one already exists; v then carries the input
// unchanged.
func (r *Repository) InsertPerformanceSnapshot(ctx context.Context, q database.Querier, v models.PerformanceSnapshot) (models.PerformanceSnapshot, error) {
	var inserted bool
	e := q.QueryRow(ctx, `INSERT INTO fleet_partner_performance_snapshots(id,organization_id,membership_id,period_start,period_end,metric_inputs,calculated_metrics,explanation,source_versions,generation_status,formula_version)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)ON CONFLICT(organization_id,membership_id,period_start,period_end,formula_version)DO NOTHING RETURNING id,generated_at,true`, v.ID, v.OrganizationID, v.MembershipID, v.PeriodStart, v.PeriodEnd, v.MetricInputs, v.CalculatedMetrics, v.Explanation, v.SourceVersions, v.GenerationStatus, v.FormulaVersion).Scan(&v.ID, &v.GeneratedAt, &inserted)
	return v, e
}

// GetPerformanceSnapshot loads the stored snapshot of one period and formula.
func (r *Repository) GetPerformanceSnapshot(ctx context.Context, q database.Querier, org, membership string, start, end time.Time, formula string) (models.PerformanceSnapshot, error) {
	var v models.PerformanceSnapshot
	e := q.QueryRow(ctx, `SELECT `+performanceColumns+` FROM fleet_partner_performance_snapshots WHERE organization_id=$1 AND membership_id=$2 AND period_start=$3 AND period_end=$4 AND formula_version=$5`, org, membership, start, end, formula).Scan(performanceScanTargets(&v)...)
	return v, e
}

// ListPerformance returns the member's snapshots, latest period first.
func (r *Repository) ListPerformance(ctx context.Context, q database.Querier, org, membership string) ([]models.PerformanceSnapshot, error) {
	rows, e := q.Query(ctx, `SELECT `+performanceColumns+` FROM fleet_partner_performance_snapshots WHERE organization_id=$1 AND membership_id=$2 ORDER BY period_end DESC`, org, membership)
	if e != nil {
		return nil, e
	}
	return collect(rows, func(row pgx.Row) (models.PerformanceSnapshot, error) {
		var v models.PerformanceSnapshot
		e := row.Scan(performanceScanTargets(&v)...)
		return v, e
	})
}

// ---- Maintenance ----

// ActiveOrganizationIDs lists every active organization.
func (r *Repository) ActiveOrganizationIDs(ctx context.Context, q database.Querier) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT id FROM fleet_partner_organizations WHERE status='ACTIVE'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var org string
		if err = rows.Scan(&org); err != nil {
			return nil, err
		}
		out = append(out, org)
	}
	return out, rows.Err()
}

// AdvanceSchedules moves shifts and leave whose time has come.
func (r *Repository) AdvanceSchedules(ctx context.Context, q database.Querier, org string) error {
	statements := []string{
		`UPDATE fleet_partner_shifts SET status='IN_PROGRESS',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='PUBLISHED' AND start_at<=now() AND end_at>now()`,
		`UPDATE fleet_partner_shifts SET status='COMPLETED',version=version+1,updated_at=now() WHERE organization_id=$1 AND status IN('PUBLISHED','IN_PROGRESS') AND end_at<=now()`,
		`UPDATE fleet_partner_leave_records SET status='COMPLETED',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='APPROVED' AND end_at<=now()`,
	}
	for _, statement := range statements {
		if _, e := q.Exec(ctx, statement, org); e != nil {
			return e
		}
	}
	return nil
}

// MarkNoShows moves confirmed assignments whose window passed to NO_SHOW.
func (r *Repository) MarkNoShows(ctx context.Context, q database.Querier, org string) ([]models.VersionedID, error) {
	return versionedUpdate(ctx, q, `UPDATE fleet_partner_shift_assignments SET status='NO_SHOW',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='CONFIRMED' AND expected_end<=now() RETURNING id,version`, org)
}

// ExpireProviderQualifications expires verified qualifications past expiry.
func (r *Repository) ExpireProviderQualifications(ctx context.Context, q database.Querier, org string) ([]models.VersionedID, error) {
	return versionedUpdate(ctx, q, `UPDATE fleet_partner_provider_qualifications SET status='EXPIRED',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='VERIFIED' AND expires_at<=now() RETURNING id,version`, org)
}

// ExpireTrainingCompletions expires passed completions past expiry.
func (r *Repository) ExpireTrainingCompletions(ctx context.Context, q database.Querier, org string) ([]models.VersionedID, error) {
	return versionedUpdate(ctx, q, `UPDATE fleet_partner_training_completions SET status='EXPIRED',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='PASSED' AND expires_at<=now() RETURNING id,version`, org)
}

func versionedUpdate(ctx context.Context, q database.Querier, sql, org string) ([]models.VersionedID, error) {
	rows, e := q.Query(ctx, sql, org)
	if e != nil {
		return nil, e
	}
	var updated []models.VersionedID
	for rows.Next() {
		var item models.VersionedID
		if e = rows.Scan(&item.ID, &item.Version); e != nil {
			rows.Close()
			return nil, e
		}
		updated = append(updated, item)
	}
	rows.Close()
	return updated, rows.Err()
}

// collect scans every row with scan and closes rows.
func collect[T any](rows pgx.Rows, scan func(pgx.Row) (T, error)) ([]T, error) {
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
