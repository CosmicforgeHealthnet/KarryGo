package workforce

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
)

type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}
type RestrictionGate interface {
	Restricted(context.Context, string, string, string) (bool, error)
}
type Options struct {
	Pool         *pgxpool.Pool
	Events       EventPublisher
	Audit        *audit.Recorder
	Media        platformclients.MediaContract
	Restrictions RestrictionGate
	Queue        *jobs.Queue
	Flags        Flags
	Now          func() time.Time
}
type Service struct{ o Options }

func NewService(o Options) *Service {
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	if o.Queue == nil {
		o.Queue = jobs.NewQueue(o.Pool)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Service{o: o}
}
func (s *Service) gate(on, write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet operations are disabled.", nil)
	}
	if !on {
		if write {
			return apperrors.Forbidden("Capability is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}
func (s *Service) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.o.Pool, fn)
}
func limit(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}
func (s *Service) memberExists(ctx context.Context, q database.Querier, org, id string) bool {
	var ok bool
	_ = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1 AND id=$2 AND status='ACTIVE') OR EXISTS(SELECT 1 FROM fleet_partner_organization_users WHERE organization_id=$1 AND id=$2 AND status='ACTIVE')`, org, id).Scan(&ok)
	return ok
}
func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, version int64, payload any) error {
	if e := s.o.Audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)); e != nil {
		return e
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, version, t.OrganizationID, t.CorrelationID, payload)
	}
	return nil
}
func history(ctx context.Context, tx pgx.Tx, t tenant.Context, kind, id, from, to, reason string, version int64, payload any) error {
	raw, _ := json.Marshal(payload)
	_, e := tx.Exec(ctx, `INSERT INTO fleet_partner_workforce_history(id,organization_id,aggregate_type,aggregate_id,previous_status,new_status,actor_id,reason,version,snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New("wfh"), t.OrganizationID, kind, id, from, to, t.ActorID, reason, version, raw)
	return e
}

type CreateProfileInput struct {
	BranchID, MembershipID, EmployeeNumber, RelationshipType, Role, SupervisorMembershipID string
	StartDate                                                                              time.Time
}

func (s *Service) CreateProfile(ctx context.Context, t tenant.Context, in CreateProfileInput) (Profile, error) {
	if e := s.gate(s.o.Flags.Workforce, true); e != nil {
		return Profile{}, e
	}
	if !t.BranchPermitted(in.BranchID) || !s.memberExists(ctx, s.o.Pool, t.OrganizationID, in.MembershipID) {
		return Profile{}, apperrors.NotFound("Resource not found.", nil)
	}
	if in.SupervisorMembershipID != "" && !s.memberExists(ctx, s.o.Pool, t.OrganizationID, in.SupervisorMembershipID) {
		return Profile{}, apperrors.BadRequest("Supervisor must belong to the organization.", nil)
	}
	var p Profile
	e := s.tx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `INSERT INTO fleet_partner_workforce_profiles(id,organization_id,branch_id,membership_id,employee_number,relationship_type,role,supervisor_membership_id,start_date) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,organization_id,branch_id,membership_id,employee_number,relationship_type,role,supervisor_membership_id,status,start_date,end_date,version,created_at,updated_at`, ids.New("wfp"), t.OrganizationID, in.BranchID, in.MembershipID, in.EmployeeNumber, strings.ToUpper(in.RelationshipType), in.Role, in.SupervisorMembershipID, in.StartDate).Scan(&p.ID, &p.OrganizationID, &p.BranchID, &p.MembershipID, &p.EmployeeNumber, &p.RelationshipType, &p.Role, &p.SupervisorMembershipID, &p.Status, &p.StartDate, &p.EndDate, &p.Version, &p.CreatedAt, &p.UpdatedAt)
		if e != nil {
			return e
		}
		if e = history(ctx, tx, t, "WORKFORCE_PROFILE", p.ID, "", p.Status, "", p.Version, p); e != nil {
			return e
		}
		return s.record(ctx, tx, t, "workforce_profile.created.v1", "workforce_profile", p.ID, p.Version, p)
	})
	return p, e
}

const profileCols = `id,organization_id,branch_id,membership_id,employee_number,relationship_type,role,supervisor_membership_id,status,start_date,end_date,version,created_at,updated_at`

func scanProfile(r pgx.Row) (Profile, error) {
	var p Profile
	e := r.Scan(&p.ID, &p.OrganizationID, &p.BranchID, &p.MembershipID, &p.EmployeeNumber, &p.RelationshipType, &p.Role, &p.SupervisorMembershipID, &p.Status, &p.StartDate, &p.EndDate, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	return p, e
}
func (s *Service) GetProfile(ctx context.Context, t tenant.Context, id string) (Profile, error) {
	if e := s.gate(s.o.Flags.Workforce, false); e != nil {
		return Profile{}, e
	}
	p, e := scanProfile(s.o.Pool.QueryRow(ctx, `SELECT `+profileCols+` FROM fleet_partner_workforce_profiles WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil || !t.BranchPermitted(p.BranchID) {
		return Profile{}, apperrors.NotFound("Resource not found.", e)
	}
	return p, nil
}
func (s *Service) ListProfiles(ctx context.Context, t tenant.Context, f Page) ([]Profile, error) {
	if e := s.gate(s.o.Flags.Workforce, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+profileCols+` FROM fleet_partner_workforce_profiles WHERE organization_id=$1 AND($2='' OR branch_id=$2)AND($3='' OR status=$3) ORDER BY created_at DESC LIMIT $4 OFFSET $5`, t.OrganizationID, f.BranchID, f.Status, limit(f.Limit), f.Offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Profile{}
	for rows.Next() {
		p, e := scanProfile(rows)
		if e != nil {
			return nil, e
		}
		if t.BranchPermitted(p.BranchID) {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}
func (s *Service) UpdateProfile(ctx context.Context, t tenant.Context, id string, expected int64, role, supervisor, status string) (Profile, error) {
	if e := s.gate(s.o.Flags.Workforce, true); e != nil {
		return Profile{}, e
	}
	old, e := s.GetProfile(ctx, t, id)
	if e != nil {
		return Profile{}, e
	}
	if expected <= 0 {
		return Profile{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	if status == "" {
		status = old.Status
	}
	if !ValidProfileTransition(old.Status, status) {
		return Profile{}, apperrors.Conflict("Invalid workforce profile transition.", nil)
	}
	if supervisor != "" && !s.memberExists(ctx, s.o.Pool, t.OrganizationID, supervisor) {
		return Profile{}, apperrors.BadRequest("Supervisor must belong to the organization.", nil)
	}
	var p Profile
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		p, x = scanProfile(tx.QueryRow(ctx, `UPDATE fleet_partner_workforce_profiles SET role=COALESCE(NULLIF($4,''),role),supervisor_membership_id=CASE WHEN $5<>'' THEN $5 ELSE supervisor_membership_id END,status=$6,end_date=CASE WHEN $6='ENDED' THEN CURRENT_DATE ELSE end_date END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+profileCols, t.OrganizationID, id, expected, role, supervisor, status))
		if x != nil {
			return apperrors.Conflict("Version conflict.", x)
		}
		if x = history(ctx, tx, t, "WORKFORCE_PROFILE", id, old.Status, p.Status, "", p.Version, p); x != nil {
			return x
		}
		ev := "workforce_profile.updated.v1"
		if p.Status == "ENDED" || p.Status == "ARCHIVED" {
			ev = "workforce_profile.ended.v1"
		}
		return s.record(ctx, tx, t, ev, "workforce_profile", id, p.Version, p)
	})
	return p, e
}

type CreateShiftInput struct {
	BranchID, Name, Role, Timezone string
	StartAt, EndAt                 time.Time
	RequiredCapacity               int
}

const shiftCols = `id,organization_id,branch_id,name,role,start_at,end_at,timezone,required_capacity,status,planner_user_id,version,created_at,updated_at`

func scanShift(r pgx.Row) (Shift, error) {
	var v Shift
	e := r.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.Name, &v.Role, &v.StartAt, &v.EndAt, &v.Timezone, &v.RequiredCapacity, &v.Status, &v.PlannerUserID, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func (s *Service) CreateShift(ctx context.Context, t tenant.Context, in CreateShiftInput) (Shift, error) {
	if e := s.gate(s.o.Flags.Shifts, true); e != nil {
		return Shift{}, e
	}
	if !t.BranchPermitted(in.BranchID) || !in.EndAt.After(in.StartAt) || in.RequiredCapacity <= 0 {
		return Shift{}, apperrors.BadRequest("Valid branch, time window and capacity are required.", nil)
	}
	var v Shift
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanShift(tx.QueryRow(ctx, `INSERT INTO fleet_partner_shifts(id,organization_id,branch_id,name,role,start_at,end_at,timezone,required_capacity,planner_user_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)RETURNING `+shiftCols, ids.New("sft"), t.OrganizationID, in.BranchID, in.Name, in.Role, in.StartAt, in.EndAt, in.Timezone, in.RequiredCapacity, t.OrganizationUserID))
		if x != nil {
			return x
		}
		_ = history(ctx, tx, t, "SHIFT", v.ID, "", v.Status, "", v.Version, v)
		return s.record(ctx, tx, t, "shift.created.v1", "shift", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetShift(ctx context.Context, t tenant.Context, id string) (Shift, error) {
	if e := s.gate(s.o.Flags.Shifts, false); e != nil {
		return Shift{}, e
	}
	v, e := scanShift(s.o.Pool.QueryRow(ctx, `SELECT `+shiftCols+` FROM fleet_partner_shifts WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil || !t.BranchPermitted(v.BranchID) {
		return Shift{}, apperrors.NotFound("Resource not found.", e)
	}
	return v, nil
}
func (s *Service) ListShifts(ctx context.Context, t tenant.Context, f Page) ([]Shift, error) {
	if e := s.gate(s.o.Flags.Shifts, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+shiftCols+` FROM fleet_partner_shifts WHERE organization_id=$1 AND($2=''OR branch_id=$2)AND($3=''OR status=$3)ORDER BY start_at DESC LIMIT $4 OFFSET $5`, t.OrganizationID, f.BranchID, f.Status, limit(f.Limit), f.Offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Shift{}
	for rows.Next() {
		v, e := scanShift(rows)
		if e != nil {
			return nil, e
		}
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, rows.Err()
}
func (s *Service) UpdateShift(ctx context.Context, t tenant.Context, id string, expected int64, name string, start, end *time.Time, to string) (Shift, error) {
	if e := s.gate(s.o.Flags.Shifts, true); e != nil {
		return Shift{}, e
	}
	old, e := s.GetShift(ctx, t, id)
	if e != nil {
		return Shift{}, e
	}
	if expected <= 0 {
		return Shift{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	if old.Status != "DRAFT" && to == "" {
		return Shift{}, apperrors.Conflict("Only draft shifts are editable.", nil)
	}
	if to == "" {
		to = old.Status
	}
	if !ValidShiftTransition(old.Status, to) {
		return Shift{}, apperrors.Conflict("Invalid shift transition.", nil)
	}
	var v Shift
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanShift(tx.QueryRow(ctx, `UPDATE fleet_partner_shifts SET name=COALESCE(NULLIF($4,''),name),start_at=COALESCE($5,start_at),end_at=COALESCE($6,end_at),status=$7,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND COALESCE($6,end_at)>COALESCE($5,start_at) RETURNING `+shiftCols, t.OrganizationID, id, expected, name, start, end, to))
		if x != nil {
			return apperrors.Conflict("Version conflict.", x)
		}
		_ = history(ctx, tx, t, "SHIFT", id, old.Status, v.Status, "", v.Version, v)
		ev := "shift.updated.v1"
		if to == "PUBLISHED" {
			ev = "shift.published.v1"
		}
		return s.record(ctx, tx, t, ev, "shift", id, v.Version, v)
	})
	return v, e
}

type AssignShiftInput struct {
	MembershipID, AssignmentType string
	ExpectedStart, ExpectedEnd   time.Time
}

const assignmentCols = `id,organization_id,shift_id,membership_id,assignment_type,status,expected_start,expected_end,checked_in_at,checked_out_at,check_in_source,check_out_source,version,created_at,updated_at`

func scanAssignment(r pgx.Row) (ShiftAssignment, error) {
	var v ShiftAssignment
	e := r.Scan(&v.ID, &v.OrganizationID, &v.ShiftID, &v.MembershipID, &v.AssignmentType, &v.Status, &v.ExpectedStart, &v.ExpectedEnd, &v.CheckedInAt, &v.CheckedOutAt, &v.CheckInSource, &v.CheckOutSource, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func (s *Service) AssignShift(ctx context.Context, t tenant.Context, shiftID string, in AssignShiftInput) (ShiftAssignment, error) {
	if e := s.gate(s.o.Flags.Shifts, true); e != nil {
		return ShiftAssignment{}, e
	}
	sh, e := s.GetShift(ctx, t, shiftID)
	if e != nil {
		return ShiftAssignment{}, e
	}
	if !s.memberExists(ctx, s.o.Pool, t.OrganizationID, in.MembershipID) {
		return ShiftAssignment{}, apperrors.NotFound("Membership not found.", nil)
	}
	if in.ExpectedStart.IsZero() {
		in.ExpectedStart = sh.StartAt
	}
	if in.ExpectedEnd.IsZero() {
		in.ExpectedEnd = sh.EndAt
	}
	var v ShiftAssignment
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if _, x := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, t.OrganizationID+":"+in.MembershipID); x != nil {
			return x
		}
		if _, x := tx.Exec(ctx, `SELECT id FROM fleet_partner_shifts WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, shiftID); x != nil {
			return x
		}
		var conflict bool
		if x := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_shift_assignments WHERE organization_id=$1 AND membership_id=$2 AND status IN('PENDING','CONFIRMED','CHECKED_IN') AND tstzrange(expected_start,expected_end,'[)')&&tstzrange($3,$4,'[)')) OR EXISTS(SELECT 1 FROM fleet_partner_leave_records WHERE organization_id=$1 AND membership_id=$2 AND status='APPROVED' AND tstzrange(start_at,end_at,'[)')&&tstzrange($3,$4,'[)'))`, t.OrganizationID, in.MembershipID, in.ExpectedStart, in.ExpectedEnd).Scan(&conflict); x != nil {
			return x
		}
		if conflict {
			return apperrors.Conflict("Shift overlaps existing work or approved leave.", nil)
		}
		var count int
		_ = tx.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_shift_assignments WHERE shift_id=$1 AND status NOT IN('DECLINED','CANCELLED')`, shiftID).Scan(&count)
		if count >= sh.RequiredCapacity {
			return apperrors.Conflict("Shift capacity is full.", nil)
		}
		var x error
		v, x = scanAssignment(tx.QueryRow(ctx, `INSERT INTO fleet_partner_shift_assignments(id,organization_id,shift_id,membership_id,assignment_type,status,expected_start,expected_end)VALUES($1,$2,$3,$4,$5,'CONFIRMED',$6,$7)RETURNING `+assignmentCols, ids.New("sfa"), t.OrganizationID, shiftID, in.MembershipID, in.AssignmentType, in.ExpectedStart, in.ExpectedEnd))
		if x != nil {
			return x
		}
		_ = history(ctx, tx, t, "SHIFT_ASSIGNMENT", v.ID, "", v.Status, "", v.Version, v)
		return s.record(ctx, tx, t, "shift_assignment.created.v1", "shift_assignment", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetAssignment(ctx context.Context, t tenant.Context, id string) (ShiftAssignment, error) {
	if e := s.gate(s.o.Flags.Shifts, false); e != nil {
		return ShiftAssignment{}, e
	}
	v, e := scanAssignment(s.o.Pool.QueryRow(ctx, `SELECT `+assignmentCols+` FROM fleet_partner_shift_assignments WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	sh, e := s.GetShift(ctx, t, v.ShiftID)
	if e != nil || !t.BranchPermitted(sh.BranchID) {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	return v, nil
}
func (s *Service) AssignmentAction(ctx context.Context, t tenant.Context, id, to, source string, expected int64) (ShiftAssignment, error) {
	old, e := s.GetAssignment(ctx, t, id)
	if e != nil {
		return ShiftAssignment{}, e
	}
	if expected <= 0 || !ValidAssignmentTransition(old.Status, to) {
		return ShiftAssignment{}, apperrors.Conflict("Invalid assignment transition or version.", nil)
	}
	if (to == "CHECKED_IN" || to == "CHECKED_OUT") && t.OrganizationUserID != old.MembershipID && t.ActorType != tenant.ActorTypeSystem {
		var supervisor bool
		_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_workforce_profiles WHERE organization_id=$1 AND membership_id=$2 AND supervisor_membership_id IN($3,$4))`, t.OrganizationID, old.MembershipID, t.OrganizationUserID, t.ActorID).Scan(&supervisor)
		if !supervisor {
			return ShiftAssignment{}, apperrors.Forbidden("Only the assigned person or an authorized supervisor may record attendance.", nil)
		}
	}
	var v ShiftAssignment
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanAssignment(tx.QueryRow(ctx, `UPDATE fleet_partner_shift_assignments SET status=$4,checked_in_at=CASE WHEN $4='CHECKED_IN' THEN now() ELSE checked_in_at END,checked_out_at=CASE WHEN $4='CHECKED_OUT' THEN now() ELSE checked_out_at END,check_in_source=CASE WHEN $4='CHECKED_IN' THEN $5 ELSE check_in_source END,check_out_source=CASE WHEN $4='CHECKED_OUT' THEN $5 ELSE check_out_source END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+assignmentCols, t.OrganizationID, id, expected, to, source))
		if x != nil {
			return apperrors.Conflict("Version conflict.", x)
		}
		_ = history(ctx, tx, t, "SHIFT_ASSIGNMENT", id, old.Status, to, "", v.Version, v)
		return s.record(ctx, tx, t, "shift_assignment."+strings.ToLower(to)+".v1", "shift_assignment", id, v.Version, v)
	})
	return v, e
}

type CreateLeaveInput struct {
	MembershipID, LeaveType, Reason string
	StartAt, EndAt                  time.Time
}

const leaveCols = `id,organization_id,membership_id,leave_type,start_at,end_at,reason,status,approved_by,approved_at,version,created_at,updated_at`

func scanLeave(r pgx.Row) (Leave, error) {
	var v Leave
	e := r.Scan(&v.ID, &v.OrganizationID, &v.MembershipID, &v.LeaveType, &v.StartAt, &v.EndAt, &v.Reason, &v.Status, &v.ApprovedBy, &v.ApprovedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func (s *Service) CreateLeave(ctx context.Context, t tenant.Context, in CreateLeaveInput) (Leave, error) {
	if e := s.gate(s.o.Flags.Leave, true); e != nil {
		return Leave{}, e
	}
	if !s.memberExists(ctx, s.o.Pool, t.OrganizationID, in.MembershipID) || !in.EndAt.After(in.StartAt) {
		return Leave{}, apperrors.BadRequest("Valid membership and leave period are required.", nil)
	}
	var v Leave
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanLeave(tx.QueryRow(ctx, `INSERT INTO fleet_partner_leave_records(id,organization_id,membership_id,leave_type,start_at,end_at,reason)VALUES($1,$2,$3,$4,$5,$6,$7)RETURNING `+leaveCols, ids.New("lve"), t.OrganizationID, in.MembershipID, in.LeaveType, in.StartAt, in.EndAt, in.Reason))
		if x != nil {
			return x
		}
		_ = history(ctx, tx, t, "LEAVE", v.ID, "", v.Status, "", v.Version, v)
		return s.record(ctx, tx, t, "leave.created.v1", "leave", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetLeave(ctx context.Context, t tenant.Context, id string) (Leave, error) {
	if e := s.gate(s.o.Flags.Leave, false); e != nil {
		return Leave{}, e
	}
	v, e := scanLeave(s.o.Pool.QueryRow(ctx, `SELECT `+leaveCols+` FROM fleet_partner_leave_records WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	return v, nil
}
func (s *Service) ListLeave(ctx context.Context, t tenant.Context, f Page) ([]Leave, error) {
	if e := s.gate(s.o.Flags.Leave, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+leaveCols+` FROM fleet_partner_leave_records WHERE organization_id=$1 AND($2=''OR status=$2)ORDER BY start_at DESC LIMIT $3 OFFSET $4`, t.OrganizationID, f.Status, limit(f.Limit), f.Offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Leave{}
	for rows.Next() {
		v, e := scanLeave(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) LeaveAction(ctx context.Context, t tenant.Context, id, to string, expected int64) (Leave, error) {
	old, e := s.GetLeave(ctx, t, id)
	if e != nil {
		return Leave{}, e
	}
	if expected <= 0 || !ValidLeaveTransition(old.Status, to) {
		return Leave{}, apperrors.Conflict("Invalid leave transition or version.", nil)
	}
	var v Leave
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if to == "APPROVED" {
			if _, x := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, t.OrganizationID+":"+old.MembershipID); x != nil {
				return x
			}
			var conflict bool
			if x := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_shift_assignments WHERE organization_id=$1 AND membership_id=$2 AND status IN('PENDING','CONFIRMED','CHECKED_IN') AND tstzrange(expected_start,expected_end,'[)')&&tstzrange($3,$4,'[)'))`, t.OrganizationID, old.MembershipID, old.StartAt, old.EndAt).Scan(&conflict); x != nil {
				return x
			}
			if conflict {
				return apperrors.Conflict("Approved leave conflicts with a shift assignment.", nil)
			}
		}
		var x error
		v, x = scanLeave(tx.QueryRow(ctx, `UPDATE fleet_partner_leave_records SET status=$4,approved_by=CASE WHEN $4='APPROVED' THEN $5 ELSE approved_by END,approved_at=CASE WHEN $4='APPROVED' THEN now() ELSE approved_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+leaveCols, t.OrganizationID, id, expected, to, t.OrganizationUserID))
		if x != nil {
			return apperrors.Conflict("Version conflict.", x)
		}
		_ = history(ctx, tx, t, "LEAVE", id, old.Status, to, "", v.Version, v)
		return s.record(ctx, tx, t, "leave."+strings.ToLower(to)+".v1", "leave", id, v.Version, v)
	})
	return v, e
}

// Qualification/training APIs use JSON rule arrays and server-owned verification transitions.
func (s *Service) CreateQualification(ctx context.Context, t tenant.Context, v Qualification) (Qualification, error) {
	if e := s.gate(s.o.Flags.Qualifications, true); e != nil {
		return v, e
	}
	v.ID = ids.New("qlf")
	v.OrganizationID = t.OrganizationID
	v.Status = "ACTIVE"
	v.Version = 1
	roles, _ := json.Marshal(v.ApplicableRoles)
	services, _ := json.Marshal(v.ApplicableServices)
	assets, _ := json.Marshal(v.ApplicableAssetTypes)
	e := s.tx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `INSERT INTO fleet_partner_qualifications(id,organization_id,code,name,applicable_roles,applicable_services,applicable_asset_types,validity_period_seconds,evidence_required)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)RETURNING created_at,updated_at`, v.ID, v.OrganizationID, v.Code, v.Name, roles, services, assets, v.ValidityPeriodSeconds, v.EvidenceRequired).Scan(&v.CreatedAt, &v.UpdatedAt)
		if e != nil {
			return e
		}
		return s.record(ctx, tx, t, "qualification.created.v1", "qualification", v.ID, 1, v)
	})
	return v, e
}
func (s *Service) ListQualifications(ctx context.Context, t tenant.Context) ([]Qualification, error) {
	if e := s.gate(s.o.Flags.Qualifications, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id,organization_id,code,name,applicable_roles,applicable_services,applicable_asset_types,validity_period_seconds,evidence_required,status,version,created_at,updated_at FROM fleet_partner_qualifications WHERE organization_id=$1 ORDER BY code`, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Qualification{}
	for rows.Next() {
		var v Qualification
		var a, b, c []byte
		if e = rows.Scan(&v.ID, &v.OrganizationID, &v.Code, &v.Name, &a, &b, &c, &v.ValidityPeriodSeconds, &v.EvidenceRequired, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt); e != nil {
			return nil, e
		}
		_ = json.Unmarshal(a, &v.ApplicableRoles)
		_ = json.Unmarshal(b, &v.ApplicableServices)
		_ = json.Unmarshal(c, &v.ApplicableAssetTypes)
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) GetQualification(ctx context.Context, t tenant.Context, id string) (Qualification, error) {
	all, e := s.ListQualifications(ctx, t)
	if e != nil {
		return Qualification{}, e
	}
	for _, v := range all {
		if v.ID == id {
			return v, nil
		}
	}
	return Qualification{}, apperrors.NotFound("Resource not found.", nil)
}
func (s *Service) UpdateQualification(ctx context.Context, t tenant.Context, id string, expected int64, v Qualification) (Qualification, error) {
	old, e := s.GetQualification(ctx, t, id)
	if e != nil {
		return v, e
	}
	if expected <= 0 {
		return v, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	roles, _ := json.Marshal(v.ApplicableRoles)
	services, _ := json.Marshal(v.ApplicableServices)
	assets, _ := json.Marshal(v.ApplicableAssetTypes)
	e = s.tx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `UPDATE fleet_partner_qualifications SET name=COALESCE(NULLIF($4,''),name),applicable_roles=CASE WHEN $5::jsonb<>'[]' THEN $5 ELSE applicable_roles END,applicable_services=CASE WHEN $6::jsonb<>'[]' THEN $6 ELSE applicable_services END,applicable_asset_types=CASE WHEN $7::jsonb<>'[]' THEN $7 ELSE applicable_asset_types END,evidence_required=$8,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING version,updated_at`, t.OrganizationID, id, expected, v.Name, roles, services, assets, v.EvidenceRequired).Scan(&v.Version, &v.UpdatedAt)
		if e != nil {
			return apperrors.Conflict("Version conflict.", e)
		}
		v.ID = id
		v.OrganizationID = t.OrganizationID
		v.Code = old.Code
		v.Status = old.Status
		v.CreatedAt = old.CreatedAt
		return s.record(ctx, tx, t, "qualification.updated.v1", "qualification", id, v.Version, v)
	})
	return v, e
}
func (s *Service) AddProviderQualification(ctx context.Context, t tenant.Context, membership string, v ProviderQualification) (ProviderQualification, error) {
	if e := s.gate(s.o.Flags.Qualifications, true); e != nil {
		return v, e
	}
	if !s.memberExists(ctx, s.o.Pool, t.OrganizationID, membership) {
		return v, apperrors.NotFound("Membership not found.", nil)
	}
	q, e := s.GetQualification(ctx, t, v.QualificationID)
	if e != nil {
		return v, e
	}
	if q.EvidenceRequired {
		if e = s.validateMedia(ctx, t, v.EvidenceMediaID, membership); e != nil {
			return v, e
		}
	}
	v.ID = ids.New("pvq")
	v.OrganizationID = t.OrganizationID
	v.MembershipID = membership
	v.Status = "PENDING_VERIFICATION"
	e = s.tx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `INSERT INTO fleet_partner_provider_qualifications(id,organization_id,membership_id,qualification_id,evidence_media_id,issued_at,expires_at)VALUES($1,$2,$3,$4,$5,$6,$7)RETURNING version,created_at,updated_at`, v.ID, v.OrganizationID, membership, v.QualificationID, v.EvidenceMediaID, v.IssuedAt, v.ExpiresAt).Scan(&v.Version, &v.CreatedAt, &v.UpdatedAt)
		if e != nil {
			return e
		}
		return s.record(ctx, tx, t, "provider_qualification.created.v1", "provider_qualification", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) ListProviderQualifications(ctx context.Context, t tenant.Context, membership string) ([]ProviderQualification, error) {
	if e := s.gate(s.o.Flags.Qualifications, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id,organization_id,membership_id,qualification_id,evidence_media_id,verification_reference,issued_at,expires_at,status,verified_by,verified_at,version,created_at,updated_at FROM fleet_partner_provider_qualifications WHERE organization_id=$1 AND membership_id=$2`, t.OrganizationID, membership)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ProviderQualification{}
	for rows.Next() {
		var v ProviderQualification
		if e = rows.Scan(&v.ID, &v.OrganizationID, &v.MembershipID, &v.QualificationID, &v.EvidenceMediaID, &v.VerificationReference, &v.IssuedAt, &v.ExpiresAt, &v.Status, &v.VerifiedBy, &v.VerifiedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) QualificationAction(ctx context.Context, t tenant.Context, id, to, reference string, expected int64) (ProviderQualification, error) {
	if expected <= 0 {
		return ProviderQualification{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	if to != "VERIFIED" && to != "REJECTED" && to != "REVOKED" {
		return ProviderQualification{}, apperrors.BadRequest("Invalid verification action.", nil)
	}
	var v ProviderQualification
	e := s.tx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `UPDATE fleet_partner_provider_qualifications SET status=$4,verification_reference=CASE WHEN $4='VERIFIED' THEN $5 ELSE verification_reference END,verified_by=$6,verified_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND status IN('PENDING_VERIFICATION','VERIFIED') RETURNING id,organization_id,membership_id,qualification_id,evidence_media_id,verification_reference,issued_at,expires_at,status,verified_by,verified_at,version,created_at,updated_at`, t.OrganizationID, id, expected, to, reference, t.OrganizationUserID).Scan(&v.ID, &v.OrganizationID, &v.MembershipID, &v.QualificationID, &v.EvidenceMediaID, &v.VerificationReference, &v.IssuedAt, &v.ExpiresAt, &v.Status, &v.VerifiedBy, &v.VerifiedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
		if e != nil {
			return apperrors.Conflict("Verification race or invalid state.", e)
		}
		return s.record(ctx, tx, t, "provider_qualification."+strings.ToLower(to)+".v1", "provider_qualification", id, v.Version, v)
	})
	return v, e
}

func (s *Service) CreateCourse(ctx context.Context, t tenant.Context, v Course) (Course, error) {
	if e := s.gate(s.o.Flags.Training, true); e != nil {
		return v, e
	}
	v.ID = ids.New("crs")
	v.OrganizationID = t.OrganizationID
	v.Status = "ACTIVE"
	roles, _ := json.Marshal(v.RequiredRoles)
	services, _ := json.Marshal(v.RequiredServices)
	e := s.tx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `INSERT INTO fleet_partner_training_courses(id,organization_id,code,title,provider_reference,required_roles,required_services,validity_period_seconds,passing_score)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)RETURNING version,created_at,updated_at`, v.ID, v.OrganizationID, v.Code, v.Title, v.ProviderReference, roles, services, v.ValidityPeriodSeconds, v.PassingScore).Scan(&v.Version, &v.CreatedAt, &v.UpdatedAt)
		if e != nil {
			return e
		}
		return s.record(ctx, tx, t, "training.course_created.v1", "training_course", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) ListCourses(ctx context.Context, t tenant.Context) ([]Course, error) {
	if e := s.gate(s.o.Flags.Training, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id,organization_id,code,title,provider_reference,required_roles,required_services,validity_period_seconds,passing_score,status,version,created_at,updated_at FROM fleet_partner_training_courses WHERE organization_id=$1 ORDER BY code`, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Course{}
	for rows.Next() {
		var v Course
		var a, b []byte
		if e = rows.Scan(&v.ID, &v.OrganizationID, &v.Code, &v.Title, &v.ProviderReference, &a, &b, &v.ValidityPeriodSeconds, &v.PassingScore, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt); e != nil {
			return nil, e
		}
		_ = json.Unmarshal(a, &v.RequiredRoles)
		_ = json.Unmarshal(b, &v.RequiredServices)
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Service) GetCourse(ctx context.Context, t tenant.Context, id string) (Course, error) {
	all, e := s.ListCourses(ctx, t)
	if e != nil {
		return Course{}, e
	}
	for _, v := range all {
		if v.ID == id {
			return v, nil
		}
	}
	return Course{}, apperrors.NotFound("Resource not found.", nil)
}
func (s *Service) UpdateCourse(ctx context.Context, t tenant.Context, id string, expected int64, v Course) (Course, error) {
	old, e := s.GetCourse(ctx, t, id)
	if e != nil {
		return v, e
	}
	if expected <= 0 {
		return v, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `UPDATE fleet_partner_training_courses SET title=COALESCE(NULLIF($4,''),title),passing_score=CASE WHEN $5>=0 THEN $5 ELSE passing_score END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING version,updated_at`, t.OrganizationID, id, expected, v.Title, v.PassingScore).Scan(&v.Version, &v.UpdatedAt)
		if e != nil {
			return apperrors.Conflict("Version conflict.", e)
		}
		v.ID = id
		v.OrganizationID = t.OrganizationID
		v.Code = old.Code
		v.Status = old.Status
		v.CreatedAt = old.CreatedAt
		return s.record(ctx, tx, t, "training.course_updated.v1", "training_course", id, v.Version, v)
	})
	return v, e
}
func (s *Service) CompleteCourse(ctx context.Context, t tenant.Context, courseID string, v Completion) (Completion, error) {
	c, e := s.GetCourse(ctx, t, courseID)
	if e != nil {
		return v, e
	}
	if !s.memberExists(ctx, s.o.Pool, t.OrganizationID, v.MembershipID) {
		return v, apperrors.NotFound("Membership not found.", nil)
	}
	if v.CertificateMediaID != "" {
		if e = s.validateMedia(ctx, t, v.CertificateMediaID, v.MembershipID); e != nil {
			return v, e
		}
	}
	v.ID = ids.New("trc")
	v.CourseID = courseID
	v.OrganizationID = t.OrganizationID
	v.Status = "FAILED"
	if v.Score >= c.PassingScore {
		v.Status = "PASSED"
	}
	if v.CompletedAt.IsZero() {
		v.CompletedAt = s.o.Now()
	}
	if c.ValidityPeriodSeconds != nil {
		x := v.CompletedAt.Add(time.Duration(*c.ValidityPeriodSeconds) * time.Second)
		v.ExpiresAt = &x
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		e := tx.QueryRow(ctx, `INSERT INTO fleet_partner_training_completions(id,organization_id,course_id,membership_id,score,status,certificate_media_id,completed_at,expires_at,verified_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)RETURNING version,created_at,updated_at`, v.ID, v.OrganizationID, courseID, v.MembershipID, v.Score, v.Status, v.CertificateMediaID, v.CompletedAt, v.ExpiresAt, t.OrganizationUserID).Scan(&v.Version, &v.CreatedAt, &v.UpdatedAt)
		if e != nil {
			return e
		}
		return s.record(ctx, tx, t, "training.completed.v1", "training_completion", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) validateMedia(ctx context.Context, t tenant.Context, id, owner string) error {
	if id == "" {
		return apperrors.BadRequest("Evidence Media ID is required.", nil)
	}
	if s.o.Media == nil || !s.o.Media.Configured() {
		return apperrors.Unavailable("Media authority is unavailable.", nil)
	}
	m, e := s.o.Media.GetFile(ctx, platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID}, id)
	if e != nil {
		return e
	}
	if !m.Ready() || m.OwnerID != owner && m.OwnerID != t.OrganizationID {
		return apperrors.Forbidden("Media evidence ownership or safety validation failed.", nil)
	}
	return nil
}

func (s *Service) Evaluate(ctx context.Context, org, membership, service, assetType string, at time.Time) (Eligibility, error) {
	if s.o.Restrictions != nil {
		for _, subject := range [][2]string{{"ORGANIZATION", org}, {"PROVIDER", membership}} {
			restricted, err := s.o.Restrictions.Restricted(ctx, org, subject[0], subject[1])
			if err != nil {
				return Eligibility{Eligible: false, HardBlocks: []string{"RESTRICTION_AUTHORITY_UNAVAILABLE"}}, err
			}
			if restricted {
				return Eligibility{Eligible: false, HardBlocks: []string{"COMPLIANCE_RESTRICTION_ACTIVE"}}, nil
			}
		}
	}
	if !s.o.Flags.Workforce && !s.o.Flags.Qualifications && !s.o.Flags.Training {
		return Eligibility{Eligible: true, Reasons: []string{"workforce_policy_disabled"}}, nil
	}
	res := Eligibility{Eligible: true, Reasons: []string{}}
	var role, status string
	e := s.o.Pool.QueryRow(ctx, `SELECT role,status FROM fleet_partner_workforce_profiles WHERE organization_id=$1 AND membership_id=$2`, org, membership).Scan(&role, &status)
	if e != nil || status != "ACTIVE" {
		res.HardBlocks = append(res.HardBlocks, "WORKFORCE_RELATIONSHIP_INACTIVE")
	}
	var leave bool
	_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_leave_records WHERE organization_id=$1 AND membership_id=$2 AND status='APPROVED' AND $3>=start_at AND $3<end_at)`, org, membership, at).Scan(&leave)
	if leave {
		res.HardBlocks = append(res.HardBlocks, "APPROVED_LEAVE")
	}
	var shiftPolicy, shiftOK bool
	_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_shifts WHERE organization_id=$1 AND status IN('PUBLISHED','IN_PROGRESS') AND $3>=start_at AND $3<end_at),EXISTS(SELECT 1 FROM fleet_partner_shift_assignments a JOIN fleet_partner_shifts s ON s.id=a.shift_id WHERE a.organization_id=$1 AND a.membership_id=$2 AND a.status IN('CONFIRMED','CHECKED_IN') AND $3>=a.expected_start AND $3<a.expected_end)`, org, membership, at).Scan(&shiftPolicy, &shiftOK)
	if shiftPolicy && !shiftOK {
		res.HardBlocks = append(res.HardBlocks, "SHIFT_NOT_ASSIGNED")
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id,evidence_required FROM fleet_partner_qualifications WHERE organization_id=$1 AND status='ACTIVE' AND(($2<>'' AND applicable_services ? $2)OR($3<>'' AND applicable_asset_types ? $3)OR($4<>'' AND applicable_roles ? $4))`, org, service, assetType, role)
	if e != nil {
		return res, e
	}
	for rows.Next() {
		var id string
		var evidence bool
		_ = rows.Scan(&id, &evidence)
		var ok bool
		_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_provider_qualifications WHERE organization_id=$1 AND membership_id=$2 AND qualification_id=$3 AND status='VERIFIED' AND(expires_at IS NULL OR expires_at>$4))`, org, membership, id, at).Scan(&ok)
		if !ok {
			res.HardBlocks = append(res.HardBlocks, "REQUIRED_QUALIFICATION_MISSING_OR_EXPIRED")
		}
	}
	rows.Close()
	rows, e = s.o.Pool.Query(ctx, `SELECT id FROM fleet_partner_training_courses WHERE organization_id=$1 AND status='ACTIVE' AND(($2<>'' AND required_services ? $2)OR($3<>'' AND required_roles ? $3))`, org, service, role)
	if e != nil {
		return res, e
	}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		var ok bool
		_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_training_completions WHERE organization_id=$1 AND membership_id=$2 AND course_id=$3 AND status='PASSED' AND(expires_at IS NULL OR expires_at>$4))`, org, membership, id, at).Scan(&ok)
		if !ok {
			res.HardBlocks = append(res.HardBlocks, "REQUIRED_TRAINING_MISSING_OR_EXPIRED")
		}
	}
	rows.Close()
	res.Eligible = len(res.HardBlocks) == 0
	if res.Eligible {
		res.Reasons = append(res.Reasons, "WORKFORCE_REQUIREMENTS_SATISFIED")
	}
	return res, nil
}

func (s *Service) GeneratePerformance(ctx context.Context, t tenant.Context, membership string, start, end time.Time) (PerformanceSnapshot, error) {
	if e := s.gate(s.o.Flags.Performance, true); e != nil {
		return PerformanceSnapshot{}, e
	}
	if !end.After(start) {
		return PerformanceSnapshot{}, apperrors.BadRequest("Valid performance period required.", nil)
	}
	metrics := map[string]int{"jobsAssigned": 0, "jobsAccepted": 0, "jobsCompleted": 0, "jobsFailed": 0, "declinedSafety": 0, "declinedOperational": 0, "declinedAvailability": 0, "declinedPersonal": 0, "declinedTechnical": 0, "declinedOther": 0}
	rows, e := s.o.Pool.Query(ctx, `SELECT status,decline_reason,version FROM fleet_partner_private_assignments WHERE organization_id=$1 AND provider_membership_id=$2 AND created_at>=$3 AND created_at<$4 ORDER BY id`, t.OrganizationID, membership, start, end)
	if e != nil {
		return PerformanceSnapshot{}, e
	}
	var maxAssignmentVersion int64
	for rows.Next() {
		var status, reason string
		var sourceVersion int64
		if e = rows.Scan(&status, &reason, &sourceVersion); e != nil {
			rows.Close()
			return PerformanceSnapshot{}, e
		}
		if sourceVersion > maxAssignmentVersion {
			maxAssignmentVersion = sourceVersion
		}
		metrics["jobsAssigned"]++
		switch status {
		case "ACCEPTED", "READY_FOR_EXECUTION", "IN_PROGRESS":
			metrics["jobsAccepted"]++
		case "DECLINED":
			cat := declineCategory(reason)
			metrics["declined"+cat]++
		case "COMPLETED":
			metrics["jobsAccepted"]++
			metrics["jobsCompleted"]++
		case "CANCELLED":
			metrics["jobsFailed"]++
		}
	}
	rows.Close()
	inputs, _ := json.Marshal(map[string]any{"privateAssignments": map[string]any{"periodStart": start, "periodEnd": end, "membershipId": membership}, "declineCategories": []string{"SAFETY", "OPERATIONAL", "AVAILABILITY", "PERSONAL", "TECHNICAL", "OTHER"}, "automaticPunishment": false})
	calc, _ := json.Marshal(metrics)
	explain, _ := json.Marshal(map[string]any{"formula": "counts grouped from verified assignment projections", "ranking": false, "disciplinaryAction": false, "safetyDeclinesAreMisconduct": false})
	versions, _ := json.Marshal(map[string]any{"formula": 1, "privateAssignmentMaxVersion": maxAssignmentVersion, "privateAssignmentCount": metrics["jobsAssigned"]})
	v := PerformanceSnapshot{ID: ids.New("pfs"), OrganizationID: t.OrganizationID, MembershipID: membership, PeriodStart: start, PeriodEnd: end, MetricInputs: inputs, CalculatedMetrics: calc, Explanation: explain, SourceVersions: versions, GenerationStatus: "COMPLETE", FormulaVersion: "v1", Version: 1}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var inserted bool
		e := tx.QueryRow(ctx, `INSERT INTO fleet_partner_performance_snapshots(id,organization_id,membership_id,period_start,period_end,metric_inputs,calculated_metrics,explanation,source_versions,generation_status,formula_version)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)ON CONFLICT(organization_id,membership_id,period_start,period_end,formula_version)DO NOTHING RETURNING id,generated_at,true`, v.ID, v.OrganizationID, membership, start, end, inputs, calc, explain, versions, v.GenerationStatus, v.FormulaVersion).Scan(&v.ID, &v.GeneratedAt, &inserted)
		if errors.Is(e, pgx.ErrNoRows) {
			return tx.QueryRow(ctx, `SELECT id,generated_at FROM fleet_partner_performance_snapshots WHERE organization_id=$1 AND membership_id=$2 AND period_start=$3 AND period_end=$4 AND formula_version=$5`, t.OrganizationID, membership, start, end, v.FormulaVersion).Scan(&v.ID, &v.GeneratedAt)
		}
		if e != nil {
			return e
		}
		if inserted {
			return s.record(ctx, tx, t, "performance_snapshot.generated.v1", "performance_snapshot", v.ID, 1, v)
		}
		return nil
	})
	return v, e
}
func declineCategory(reason string) string {
	r := strings.ToUpper(reason)
	switch {
	case strings.Contains(r, "UNSAFE") || strings.Contains(r, "SAFETY") || strings.Contains(r, "DEFECT"):
		return "Safety"
	case strings.Contains(r, "AVAILABLE") || strings.Contains(r, "BUSY"):
		return "Availability"
	case strings.Contains(r, "TECH"):
		return "Technical"
	case strings.Contains(r, "PERSONAL"):
		return "Personal"
	case strings.Contains(r, "OPERATION"):
		return "Operational"
	default:
		return "Other"
	}
}
func (s *Service) ListPerformance(ctx context.Context, t tenant.Context, membership string) ([]PerformanceSnapshot, error) {
	if e := s.gate(s.o.Flags.Performance, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id,organization_id,membership_id,period_start,period_end,metric_inputs,calculated_metrics,explanation,source_versions,generation_status,formula_version,generated_at,version FROM fleet_partner_performance_snapshots WHERE organization_id=$1 AND membership_id=$2 ORDER BY period_end DESC`, t.OrganizationID, membership)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []PerformanceSnapshot{}
	for rows.Next() {
		var v PerformanceSnapshot
		if e = rows.Scan(&v.ID, &v.OrganizationID, &v.MembershipID, &v.PeriodStart, &v.PeriodEnd, &v.MetricInputs, &v.CalculatedMetrics, &v.Explanation, &v.SourceVersions, &v.GenerationStatus, &v.FormulaVersion, &v.GeneratedAt, &v.Version); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
