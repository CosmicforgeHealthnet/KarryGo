// Package usecases owns the workforce workflows: profiles, shifts and shift
// templates, attendance, leave, qualifications, training, eligibility and
// explainable performance snapshots.
package usecases

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}
type RestrictionGate interface {
	Restricted(context.Context, string, string, string) (bool, error)
}

// Repository is the persistence the workforce use cases need. Each method
// takes the querier it runs on (the pool, or the use case's transaction). The
// Postgres implementation lives in workforce/repositories and is wired at the
// composition root.
type Repository interface {
	MemberActive(ctx context.Context, q database.Querier, org, id string) bool
	BranchActive(ctx context.Context, q database.Querier, org, branch string) bool
	InsertHistory(ctx context.Context, q database.Querier, org, kind, id, from, to, actorID, reason string, version int64, snapshot []byte) error
	LockMemberSchedule(ctx context.Context, q database.Querier, org, membership string) error

	InsertProfile(ctx context.Context, q database.Querier, p models.Profile) (models.Profile, error)
	GetProfile(ctx context.Context, q database.Querier, org, id string) (models.Profile, error)
	ListProfiles(ctx context.Context, q database.Querier, org string, f models.ListQuery) ([]models.Profile, error)
	UpdateProfile(ctx context.Context, q database.Querier, org, id string, expected int64, role, supervisor, status string) (models.Profile, error)
	ActiveProfileMemberships(ctx context.Context, q database.Querier, org string) ([]string, error)

	InsertShift(ctx context.Context, q database.Querier, v models.Shift) (models.Shift, error)
	InsertTemplateShift(ctx context.Context, q database.Querier, tpl models.ShiftTemplate, start, end time.Time, plannerUserID, localDate string) (models.Shift, error)
	GetShift(ctx context.Context, q database.Querier, org, id string) (models.Shift, error)
	ListShifts(ctx context.Context, q database.Querier, org string, f models.ListQuery) ([]models.Shift, error)
	UpdateShift(ctx context.Context, q database.Querier, org, id string, expected int64, name string, start, end *time.Time, to string) (models.Shift, error)
	LockShift(ctx context.Context, q database.Querier, org, shiftID string) error

	ScheduleConflict(ctx context.Context, q database.Querier, org, membership string, start, end time.Time) (bool, error)
	AssignmentConflict(ctx context.Context, q database.Querier, org, membership string, start, end time.Time) (bool, error)
	ActiveAssignmentCount(ctx context.Context, q database.Querier, shiftID string) (int, error)
	InsertAssignment(ctx context.Context, q database.Querier, v models.ShiftAssignment) (models.ShiftAssignment, error)
	GetAssignment(ctx context.Context, q database.Querier, org, id string) (models.ShiftAssignment, error)
	IsSupervisor(ctx context.Context, q database.Querier, org, membership, organizationUserID, actorID string) (bool, error)
	UpdateAssignmentStatus(ctx context.Context, q database.Querier, org, id string, expected int64, to, source string) (models.ShiftAssignment, error)

	InsertLeave(ctx context.Context, q database.Querier, v models.Leave) (models.Leave, error)
	GetLeave(ctx context.Context, q database.Querier, org, id string) (models.Leave, error)
	ListLeave(ctx context.Context, q database.Querier, org string, f models.ListQuery) ([]models.Leave, error)
	UpdateLeaveStatus(ctx context.Context, q database.Querier, org, id string, expected int64, to, approver string) (models.Leave, error)

	InsertQualification(ctx context.Context, q database.Querier, v models.Qualification) (models.Qualification, error)
	ListQualifications(ctx context.Context, q database.Querier, org string) ([]models.Qualification, error)
	ListQualificationsPage(ctx context.Context, q database.Querier, org string, f models.ListQuery) ([]models.Qualification, error)
	UpdateQualification(ctx context.Context, q database.Querier, org, id string, expected int64, v models.Qualification) (int64, time.Time, error)
	InsertProviderQualification(ctx context.Context, q database.Querier, v models.ProviderQualification) (models.ProviderQualification, error)
	ListProviderQualifications(ctx context.Context, q database.Querier, org, membership string) ([]models.ProviderQualification, error)
	UpdateProviderQualificationStatus(ctx context.Context, q database.Querier, org, id string, expected int64, to, reference, verifier string) (models.ProviderQualification, error)

	InsertCourse(ctx context.Context, q database.Querier, v models.Course) (models.Course, error)
	ListCourses(ctx context.Context, q database.Querier, org string) ([]models.Course, error)
	ListCoursesPage(ctx context.Context, q database.Querier, org string, f models.ListQuery) ([]models.Course, error)
	UpdateCourse(ctx context.Context, q database.Querier, org, id string, expected int64, title string, passingScore float64) (int64, time.Time, error)
	InsertCompletion(ctx context.Context, q database.Querier, v models.Completion, verifier string) (models.Completion, error)

	ProfileRoleStatus(ctx context.Context, q database.Querier, org, membership string) (string, string, error)
	OnApprovedLeave(ctx context.Context, q database.Querier, org, membership string, at time.Time) (bool, error)
	ShiftCoverage(ctx context.Context, q database.Querier, org, membership string, at time.Time) (bool, bool, error)
	ApplicableQualificationIDs(ctx context.Context, q database.Querier, org, service, assetType, role string) ([]string, error)
	HasVerifiedQualification(ctx context.Context, q database.Querier, org, membership, qualificationID string, at time.Time) (bool, error)
	ApplicableCourseIDs(ctx context.Context, q database.Querier, org, service, role string) ([]string, error)
	HasPassedCourse(ctx context.Context, q database.Querier, org, membership, courseID string, at time.Time) (bool, error)

	AssignmentRecords(ctx context.Context, q database.Querier, org, membership string, start, end time.Time) ([]models.AssignmentRecord, error)
	InsertPerformanceSnapshot(ctx context.Context, q database.Querier, v models.PerformanceSnapshot) (models.PerformanceSnapshot, error)
	GetPerformanceSnapshot(ctx context.Context, q database.Querier, org, membership string, start, end time.Time, formula string) (models.PerformanceSnapshot, error)
	ListPerformance(ctx context.Context, q database.Querier, org, membership string) ([]models.PerformanceSnapshot, error)

	InsertTemplate(ctx context.Context, q database.Querier, v models.ShiftTemplate, rules []byte, createdBy string) (models.ShiftTemplate, error)
	ListTemplates(ctx context.Context, q database.Querier, org string, f models.ListQuery) ([]models.ShiftTemplate, error)
	GetTemplate(ctx context.Context, q database.Querier, org, id string) (models.ShiftTemplate, error)
	UpdateTemplate(ctx context.Context, q database.Querier, org, id string, expected int64, next models.ShiftTemplate, rules []byte) (models.ShiftTemplate, error)

	InsertEnrollment(ctx context.Context, q database.Querier, org string, course models.Course, membership, assignedBy string, dueAt *time.Time) (models.Enrollment, error)
	ListEnrollments(ctx context.Context, q database.Querier, org string, f models.ListQuery) ([]models.Enrollment, error)
	GetEnrollment(ctx context.Context, q database.Querier, org, id string) (models.Enrollment, error)
	CompleteEnrollment(ctx context.Context, q database.Querier, org, id, status string, completedAt time.Time, score float64, completionID string) (models.Enrollment, error)
	CancelEnrollment(ctx context.Context, q database.Querier, org, id string) (models.Enrollment, error)

	ProviderOwnShifts(ctx context.Context, q database.Querier, org, membership string, from, to time.Time) ([]models.OwnShift, error)
	ProviderOwnLeave(ctx context.Context, q database.Querier, org, membership string, from, to time.Time) ([]models.OwnLeave, error)
	ProviderOwnQualifications(ctx context.Context, q database.Querier, org, membership string) ([]models.OwnQualification, error)
	ProviderOwnEnrollments(ctx context.Context, q database.Querier, org, membership string) ([]models.OwnTrainingEnrollment, error)
	ProviderOwnCompletions(ctx context.Context, q database.Querier, org, membership string) ([]models.OwnTrainingCompletion, error)

	ActiveOrganizationIDs(ctx context.Context, q database.Querier) ([]string, error)
	AdvanceSchedules(ctx context.Context, q database.Querier, org string) error
	MarkNoShows(ctx context.Context, q database.Querier, org string) ([]models.VersionedID, error)
	ExpireProviderQualifications(ctx context.Context, q database.Querier, org string) ([]models.VersionedID, error)
	ExpireTrainingCompletions(ctx context.Context, q database.Querier, org string) ([]models.VersionedID, error)
}

type Flags struct {
	Workforce, Shifts, Leave, Qualifications, Training, Performance, Workers, KillSwitch bool
}

type Options struct {
	Pool         *pgxpool.Pool
	Repository   Repository
	Events       EventPublisher
	Audit        *audit.Recorder
	Media        platformclients.MediaContract
	Restrictions RestrictionGate
	Queue        *jobs.Queue
	Flags        Flags
	Now          func() time.Time
}
type Service struct {
	o    Options
	repo Repository
}

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
	return &Service{o: o, repo: o.Repository}
}

// Now is the service clock.
func (s *Service) Now() time.Time { return s.o.Now() }

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

// Page is a list request. Offset pages are the backward-compatible form;
// Keyset pages (FLEET-6f) continue from After/AfterID, the sort key and id of
// the last row of the previous page.
type Page struct {
	BranchID, Status string
	Limit, Offset    int
	Keyset           bool
	After            *time.Time
	AfterID          string
}

// validPage refuses a negative page window as a client error instead of
// letting PostgreSQL reject the OFFSET with a server error.
func validPage(f Page) error {
	if f.Limit < 0 || f.Offset < 0 {
		return apperrors.BadRequest("limit and offset must not be negative.", nil)
	}
	return nil
}

// query converts a page to the repository's list query, confining keyset
// pages to the caller's permitted branches in SQL.
func query(t tenant.Context, f Page) models.ListQuery {
	return models.ListQuery{BranchID: f.BranchID, Status: f.Status, Limit: f.Limit, Offset: f.Offset,
		Keyset: f.Keyset, After: f.After, AfterID: f.AfterID, BranchScope: t.PermittedBranchIDs}
}

// pageSize is the number of rows a page serves (default 50, maximum 200).
func pageSize(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}

// trim cuts a keyset page fetched with one extra row and reports whether a
// further page exists.
func trim[T any](rows []T, f Page) ([]T, bool) {
	if !f.Keyset || len(rows) <= pageSize(f.Limit) {
		return rows, false
	}
	return rows[:pageSize(f.Limit)], true
}

func (s *Service) memberExists(ctx context.Context, q database.Querier, org, id string) bool {
	return s.repo.MemberActive(ctx, q, org, id)
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
func (s *Service) history(ctx context.Context, tx pgx.Tx, t tenant.Context, kind, id, from, to, reason string, version int64, payload any) error {
	raw, _ := json.Marshal(payload)
	return s.repo.InsertHistory(ctx, tx, t.OrganizationID, kind, id, from, to, t.ActorID, reason, version, raw)
}

type CreateProfileInput struct {
	BranchID               string    `json:"branchId"`
	MembershipID           string    `json:"membershipId"`
	EmployeeNumber         string    `json:"employeeNumber"`
	RelationshipType       string    `json:"relationshipType"`
	Role                   string    `json:"role"`
	SupervisorMembershipID string    `json:"supervisorMembershipId"`
	StartDate              time.Time `json:"startDate"`
}

func (s *Service) CreateProfile(ctx context.Context, t tenant.Context, in CreateProfileInput) (models.Profile, error) {
	if e := s.gate(s.o.Flags.Workforce, true); e != nil {
		return models.Profile{}, e
	}
	if !t.BranchPermitted(in.BranchID) || !s.memberExists(ctx, s.o.Pool, t.OrganizationID, in.MembershipID) {
		return models.Profile{}, apperrors.NotFound("Resource not found.", nil)
	}
	if in.SupervisorMembershipID != "" && !s.memberExists(ctx, s.o.Pool, t.OrganizationID, in.SupervisorMembershipID) {
		return models.Profile{}, apperrors.BadRequest("Supervisor must belong to the organization.", nil)
	}
	var p models.Profile
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var e error
		p, e = s.repo.InsertProfile(ctx, tx, models.Profile{OrganizationID: t.OrganizationID, BranchID: in.BranchID,
			MembershipID: in.MembershipID, EmployeeNumber: in.EmployeeNumber, RelationshipType: strings.ToUpper(in.RelationshipType),
			Role: in.Role, SupervisorMembershipID: in.SupervisorMembershipID, StartDate: in.StartDate})
		if e != nil {
			return e
		}
		if e = s.history(ctx, tx, t, "WORKFORCE_PROFILE", p.ID, "", p.Status, "", p.Version, p); e != nil {
			return e
		}
		return s.record(ctx, tx, t, "workforce_profile.created.v1", "workforce_profile", p.ID, p.Version, p)
	})
	return p, e
}

func (s *Service) GetProfile(ctx context.Context, t tenant.Context, id string) (models.Profile, error) {
	if e := s.gate(s.o.Flags.Workforce, false); e != nil {
		return models.Profile{}, e
	}
	p, e := s.repo.GetProfile(ctx, s.o.Pool, t.OrganizationID, id)
	if e != nil || !t.BranchPermitted(p.BranchID) {
		return models.Profile{}, apperrors.NotFound("Resource not found.", e)
	}
	return p, nil
}
func (s *Service) ListProfiles(ctx context.Context, t tenant.Context, f Page) ([]models.Profile, error) {
	out, _, e := s.ListProfilesPage(ctx, t, f)
	return out, e
}

// ListProfilesPage returns one page of profiles and, for keyset pages,
// whether a further page exists.
func (s *Service) ListProfilesPage(ctx context.Context, t tenant.Context, f Page) ([]models.Profile, bool, error) {
	if e := s.gate(s.o.Flags.Workforce, false); e != nil {
		return nil, false, e
	}
	rows, e := s.repo.ListProfiles(ctx, s.o.Pool, t.OrganizationID, query(t, f))
	if e != nil {
		return nil, false, e
	}
	rows, more := trim(rows, f)
	out := []models.Profile{}
	for _, p := range rows {
		if t.BranchPermitted(p.BranchID) {
			out = append(out, p)
		}
	}
	return out, more, nil
}
func (s *Service) UpdateProfile(ctx context.Context, t tenant.Context, id string, expected int64, role, supervisor, status string) (models.Profile, error) {
	if e := s.gate(s.o.Flags.Workforce, true); e != nil {
		return models.Profile{}, e
	}
	old, e := s.GetProfile(ctx, t, id)
	if e != nil {
		return models.Profile{}, e
	}
	if expected <= 0 {
		return models.Profile{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	if status == "" {
		status = old.Status
	}
	if !models.ValidProfileTransition(old.Status, status) {
		return models.Profile{}, apperrors.Conflict("Invalid workforce profile transition.", nil)
	}
	if supervisor != "" && !s.memberExists(ctx, s.o.Pool, t.OrganizationID, supervisor) {
		return models.Profile{}, apperrors.BadRequest("Supervisor must belong to the organization.", nil)
	}
	var p models.Profile
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		p, x = s.repo.UpdateProfile(ctx, tx, t.OrganizationID, id, expected, role, supervisor, status)
		if x != nil {
			return apperrors.Conflict("Version conflict.", x)
		}
		if x = s.history(ctx, tx, t, "WORKFORCE_PROFILE", id, old.Status, p.Status, "", p.Version, p); x != nil {
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
	BranchID         string    `json:"branchId"`
	Name             string    `json:"name"`
	Role             string    `json:"role"`
	Timezone         string    `json:"timezone"`
	StartAt          time.Time `json:"startAt"`
	EndAt            time.Time `json:"endAt"`
	RequiredCapacity int       `json:"requiredCapacity"`
}

func (s *Service) CreateShift(ctx context.Context, t tenant.Context, in CreateShiftInput) (models.Shift, error) {
	if e := s.gate(s.o.Flags.Shifts, true); e != nil {
		return models.Shift{}, e
	}
	if !t.BranchPermitted(in.BranchID) || !in.EndAt.After(in.StartAt) || in.RequiredCapacity <= 0 {
		return models.Shift{}, apperrors.BadRequest("Valid branch, time window and capacity are required.", nil)
	}
	var v models.Shift
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.InsertShift(ctx, tx, models.Shift{OrganizationID: t.OrganizationID, BranchID: in.BranchID, Name: in.Name,
			Role: in.Role, StartAt: in.StartAt, EndAt: in.EndAt, Timezone: in.Timezone, RequiredCapacity: in.RequiredCapacity,
			PlannerUserID: t.OrganizationUserID})
		if x != nil {
			return x
		}
		_ = s.history(ctx, tx, t, "SHIFT", v.ID, "", v.Status, "", v.Version, v)
		return s.record(ctx, tx, t, "shift.created.v1", "shift", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetShift(ctx context.Context, t tenant.Context, id string) (models.Shift, error) {
	if e := s.gate(s.o.Flags.Shifts, false); e != nil {
		return models.Shift{}, e
	}
	v, e := s.repo.GetShift(ctx, s.o.Pool, t.OrganizationID, id)
	if e != nil || !t.BranchPermitted(v.BranchID) {
		return models.Shift{}, apperrors.NotFound("Resource not found.", e)
	}
	return v, nil
}
func (s *Service) ListShifts(ctx context.Context, t tenant.Context, f Page) ([]models.Shift, error) {
	out, _, e := s.ListShiftsPage(ctx, t, f)
	return out, e
}

// ListShiftsPage returns one page of shifts and, for keyset pages, whether a
// further page exists.
func (s *Service) ListShiftsPage(ctx context.Context, t tenant.Context, f Page) ([]models.Shift, bool, error) {
	if e := s.gate(s.o.Flags.Shifts, false); e != nil {
		return nil, false, e
	}
	rows, e := s.repo.ListShifts(ctx, s.o.Pool, t.OrganizationID, query(t, f))
	if e != nil {
		return nil, false, e
	}
	rows, more := trim(rows, f)
	out := []models.Shift{}
	for _, v := range rows {
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, more, nil
}
func (s *Service) UpdateShift(ctx context.Context, t tenant.Context, id string, expected int64, name string, start, end *time.Time, to string) (models.Shift, error) {
	if e := s.gate(s.o.Flags.Shifts, true); e != nil {
		return models.Shift{}, e
	}
	old, e := s.GetShift(ctx, t, id)
	if e != nil {
		return models.Shift{}, e
	}
	if expected <= 0 {
		return models.Shift{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	if old.Status != "DRAFT" && to == "" {
		return models.Shift{}, apperrors.Conflict("Only draft shifts are editable.", nil)
	}
	if to == "" {
		to = old.Status
	}
	if !models.ValidShiftTransition(old.Status, to) {
		return models.Shift{}, apperrors.Conflict("Invalid shift transition.", nil)
	}
	var v models.Shift
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.UpdateShift(ctx, tx, t.OrganizationID, id, expected, name, start, end, to)
		if x != nil {
			return apperrors.Conflict("Version conflict.", x)
		}
		_ = s.history(ctx, tx, t, "SHIFT", id, old.Status, v.Status, "", v.Version, v)
		ev := "shift.updated.v1"
		if to == "PUBLISHED" {
			ev = "shift.published.v1"
		}
		return s.record(ctx, tx, t, ev, "shift", id, v.Version, v)
	})
	return v, e
}

type AssignShiftInput struct {
	MembershipID   string    `json:"membershipId"`
	AssignmentType string    `json:"assignmentType"`
	ExpectedStart  time.Time `json:"expectedStart"`
	ExpectedEnd    time.Time `json:"expectedEnd"`
}

func (s *Service) AssignShift(ctx context.Context, t tenant.Context, shiftID string, in AssignShiftInput) (models.ShiftAssignment, error) {
	if e := s.gate(s.o.Flags.Shifts, true); e != nil {
		return models.ShiftAssignment{}, e
	}
	sh, e := s.GetShift(ctx, t, shiftID)
	if e != nil {
		return models.ShiftAssignment{}, e
	}
	if !s.memberExists(ctx, s.o.Pool, t.OrganizationID, in.MembershipID) {
		return models.ShiftAssignment{}, apperrors.NotFound("Membership not found.", nil)
	}
	if in.ExpectedStart.IsZero() {
		in.ExpectedStart = sh.StartAt
	}
	if in.ExpectedEnd.IsZero() {
		in.ExpectedEnd = sh.EndAt
	}
	var v models.ShiftAssignment
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if x := s.repo.LockMemberSchedule(ctx, tx, t.OrganizationID, in.MembershipID); x != nil {
			return x
		}
		if x := s.repo.LockShift(ctx, tx, t.OrganizationID, shiftID); x != nil {
			return x
		}
		conflict, x := s.repo.ScheduleConflict(ctx, tx, t.OrganizationID, in.MembershipID, in.ExpectedStart, in.ExpectedEnd)
		if x != nil {
			return x
		}
		if conflict {
			return apperrors.Conflict("Shift overlaps existing work or approved leave.", nil)
		}
		count, _ := s.repo.ActiveAssignmentCount(ctx, tx, shiftID)
		if count >= sh.RequiredCapacity {
			return apperrors.Conflict("Shift capacity is full.", nil)
		}
		v, x = s.repo.InsertAssignment(ctx, tx, models.ShiftAssignment{OrganizationID: t.OrganizationID, ShiftID: shiftID,
			MembershipID: in.MembershipID, AssignmentType: in.AssignmentType, ExpectedStart: in.ExpectedStart, ExpectedEnd: in.ExpectedEnd})
		if x != nil {
			return x
		}
		_ = s.history(ctx, tx, t, "SHIFT_ASSIGNMENT", v.ID, "", v.Status, "", v.Version, v)
		return s.record(ctx, tx, t, "shift_assignment.created.v1", "shift_assignment", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetAssignment(ctx context.Context, t tenant.Context, id string) (models.ShiftAssignment, error) {
	if e := s.gate(s.o.Flags.Shifts, false); e != nil {
		return models.ShiftAssignment{}, e
	}
	v, e := s.repo.GetAssignment(ctx, s.o.Pool, t.OrganizationID, id)
	if e != nil {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	sh, e := s.GetShift(ctx, t, v.ShiftID)
	if e != nil || !t.BranchPermitted(sh.BranchID) {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	return v, nil
}
func (s *Service) AssignmentAction(ctx context.Context, t tenant.Context, id, to, source string, expected int64) (models.ShiftAssignment, error) {
	old, e := s.GetAssignment(ctx, t, id)
	if e != nil {
		return models.ShiftAssignment{}, e
	}
	if expected <= 0 || !models.ValidAssignmentTransition(old.Status, to) {
		return models.ShiftAssignment{}, apperrors.Conflict("Invalid assignment transition or version.", nil)
	}
	if (to == "CHECKED_IN" || to == "CHECKED_OUT") && t.OrganizationUserID != old.MembershipID && t.ActorType != tenant.ActorTypeSystem {
		supervisor, _ := s.repo.IsSupervisor(ctx, s.o.Pool, t.OrganizationID, old.MembershipID, t.OrganizationUserID, t.ActorID)
		if !supervisor {
			return models.ShiftAssignment{}, apperrors.Forbidden("Only the assigned person or an authorized supervisor may record attendance.", nil)
		}
	}
	var v models.ShiftAssignment
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.UpdateAssignmentStatus(ctx, tx, t.OrganizationID, id, expected, to, source)
		if x != nil {
			return apperrors.Conflict("Version conflict.", x)
		}
		_ = s.history(ctx, tx, t, "SHIFT_ASSIGNMENT", id, old.Status, to, "", v.Version, v)
		return s.record(ctx, tx, t, "shift_assignment."+strings.ToLower(to)+".v1", "shift_assignment", id, v.Version, v)
	})
	return v, e
}

type CreateLeaveInput struct {
	MembershipID string    `json:"membershipId"`
	LeaveType    string    `json:"leaveType"`
	Reason       string    `json:"reason"`
	StartAt      time.Time `json:"startAt"`
	EndAt        time.Time `json:"endAt"`
}

func (s *Service) CreateLeave(ctx context.Context, t tenant.Context, in CreateLeaveInput) (models.Leave, error) {
	if e := s.gate(s.o.Flags.Leave, true); e != nil {
		return models.Leave{}, e
	}
	if !s.memberExists(ctx, s.o.Pool, t.OrganizationID, in.MembershipID) || !in.EndAt.After(in.StartAt) {
		return models.Leave{}, apperrors.BadRequest("Valid membership and leave period are required.", nil)
	}
	var v models.Leave
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.InsertLeave(ctx, tx, models.Leave{OrganizationID: t.OrganizationID, MembershipID: in.MembershipID,
			LeaveType: in.LeaveType, StartAt: in.StartAt, EndAt: in.EndAt, Reason: in.Reason})
		if x != nil {
			return x
		}
		_ = s.history(ctx, tx, t, "LEAVE", v.ID, "", v.Status, "", v.Version, v)
		return s.record(ctx, tx, t, "leave.created.v1", "leave", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetLeave(ctx context.Context, t tenant.Context, id string) (models.Leave, error) {
	if e := s.gate(s.o.Flags.Leave, false); e != nil {
		return models.Leave{}, e
	}
	v, e := s.repo.GetLeave(ctx, s.o.Pool, t.OrganizationID, id)
	if e != nil {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	return v, nil
}
func (s *Service) ListLeave(ctx context.Context, t tenant.Context, f Page) ([]models.Leave, error) {
	out, _, e := s.ListLeavePage(ctx, t, f)
	return out, e
}

// ListLeavePage returns one page of leave and, for keyset pages, whether a
// further page exists.
func (s *Service) ListLeavePage(ctx context.Context, t tenant.Context, f Page) ([]models.Leave, bool, error) {
	if e := s.gate(s.o.Flags.Leave, false); e != nil {
		return nil, false, e
	}
	rows, e := s.repo.ListLeave(ctx, s.o.Pool, t.OrganizationID, query(t, f))
	if e != nil {
		return nil, false, e
	}
	rows, more := trim(rows, f)
	return rows, more, nil
}
func (s *Service) LeaveAction(ctx context.Context, t tenant.Context, id, to string, expected int64) (models.Leave, error) {
	old, e := s.GetLeave(ctx, t, id)
	if e != nil {
		return models.Leave{}, e
	}
	if expected <= 0 || !models.ValidLeaveTransition(old.Status, to) {
		return models.Leave{}, apperrors.Conflict("Invalid leave transition or version.", nil)
	}
	var v models.Leave
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if to == "APPROVED" {
			if x := s.repo.LockMemberSchedule(ctx, tx, t.OrganizationID, old.MembershipID); x != nil {
				return x
			}
			conflict, x := s.repo.AssignmentConflict(ctx, tx, t.OrganizationID, old.MembershipID, old.StartAt, old.EndAt)
			if x != nil {
				return x
			}
			if conflict {
				return apperrors.Conflict("Approved leave conflicts with a shift assignment.", nil)
			}
		}
		var x error
		v, x = s.repo.UpdateLeaveStatus(ctx, tx, t.OrganizationID, id, expected, to, t.OrganizationUserID)
		if x != nil {
			return apperrors.Conflict("Version conflict.", x)
		}
		_ = s.history(ctx, tx, t, "LEAVE", id, old.Status, to, "", v.Version, v)
		return s.record(ctx, tx, t, "leave."+strings.ToLower(to)+".v1", "leave", id, v.Version, v)
	})
	return v, e
}

// Qualification/training APIs use JSON rule arrays and server-owned verification transitions.
func (s *Service) CreateQualification(ctx context.Context, t tenant.Context, v models.Qualification) (models.Qualification, error) {
	if e := s.gate(s.o.Flags.Qualifications, true); e != nil {
		return v, e
	}
	v.ID = ids.New("qlf")
	v.OrganizationID = t.OrganizationID
	v.Status = "ACTIVE"
	v.Version = 1
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var e error
		v, e = s.repo.InsertQualification(ctx, tx, v)
		if e != nil {
			return e
		}
		return s.record(ctx, tx, t, "qualification.created.v1", "qualification", v.ID, 1, v)
	})
	return v, e
}
func (s *Service) ListQualifications(ctx context.Context, t tenant.Context) ([]models.Qualification, error) {
	if e := s.gate(s.o.Flags.Qualifications, false); e != nil {
		return nil, e
	}
	return s.repo.ListQualifications(ctx, s.o.Pool, t.OrganizationID)
}

// ListQualificationsPage returns one keyset page of qualification
// definitions, by code, and whether a further page exists.
func (s *Service) ListQualificationsPage(ctx context.Context, t tenant.Context, f Page) ([]models.Qualification, bool, error) {
	if e := s.gate(s.o.Flags.Qualifications, false); e != nil {
		return nil, false, e
	}
	f.Keyset = true
	rows, e := s.repo.ListQualificationsPage(ctx, s.o.Pool, t.OrganizationID, query(t, f))
	if e != nil {
		return nil, false, e
	}
	rows, more := trim(rows, f)
	return rows, more, nil
}
func (s *Service) GetQualification(ctx context.Context, t tenant.Context, id string) (models.Qualification, error) {
	all, e := s.ListQualifications(ctx, t)
	if e != nil {
		return models.Qualification{}, e
	}
	for _, v := range all {
		if v.ID == id {
			return v, nil
		}
	}
	return models.Qualification{}, apperrors.NotFound("Resource not found.", nil)
}

// UpdateQualification applies a partial update. Fields left empty/nil keep
// their stored value, so a rename never clears the applicability lists or
// the evidence requirement.
func (s *Service) UpdateQualification(ctx context.Context, t tenant.Context, id string, expected int64, p models.QualificationPatch) (models.Qualification, error) {
	old, e := s.GetQualification(ctx, t, id)
	if e != nil {
		return models.Qualification{}, e
	}
	if expected <= 0 {
		return models.Qualification{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	v := old
	if p.Name != "" {
		v.Name = p.Name
	}
	if p.ApplicableRoles != nil {
		v.ApplicableRoles = p.ApplicableRoles
	}
	if p.ApplicableServices != nil {
		v.ApplicableServices = p.ApplicableServices
	}
	if p.ApplicableAssetTypes != nil {
		v.ApplicableAssetTypes = p.ApplicableAssetTypes
	}
	if p.EvidenceRequired != nil {
		v.EvidenceRequired = *p.EvidenceRequired
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		version, updated, e := s.repo.UpdateQualification(ctx, tx, t.OrganizationID, id, expected, v)
		if e != nil {
			return apperrors.Conflict("Version conflict.", e)
		}
		v.Version, v.UpdatedAt = version, updated
		return s.record(ctx, tx, t, "qualification.updated.v1", "qualification", id, v.Version, v)
	})
	return v, e
}

func (s *Service) AddProviderQualification(ctx context.Context, t tenant.Context, membership string, v models.ProviderQualification) (models.ProviderQualification, error) {
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
		var e error
		v, e = s.repo.InsertProviderQualification(ctx, tx, v)
		if e != nil {
			return e
		}
		return s.record(ctx, tx, t, "provider_qualification.created.v1", "provider_qualification", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) ListProviderQualifications(ctx context.Context, t tenant.Context, membership string) ([]models.ProviderQualification, error) {
	if e := s.gate(s.o.Flags.Qualifications, false); e != nil {
		return nil, e
	}
	return s.repo.ListProviderQualifications(ctx, s.o.Pool, t.OrganizationID, membership)
}
func (s *Service) QualificationAction(ctx context.Context, t tenant.Context, id, to, reference string, expected int64) (models.ProviderQualification, error) {
	if expected <= 0 {
		return models.ProviderQualification{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	if to != "VERIFIED" && to != "REJECTED" && to != "REVOKED" {
		return models.ProviderQualification{}, apperrors.BadRequest("Invalid verification action.", nil)
	}
	var v models.ProviderQualification
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var e error
		v, e = s.repo.UpdateProviderQualificationStatus(ctx, tx, t.OrganizationID, id, expected, to, reference, t.OrganizationUserID)
		if e != nil {
			return apperrors.Conflict("Verification race or invalid state.", e)
		}
		return s.record(ctx, tx, t, "provider_qualification."+strings.ToLower(to)+".v1", "provider_qualification", id, v.Version, v)
	})
	return v, e
}

func (s *Service) CreateCourse(ctx context.Context, t tenant.Context, v models.Course) (models.Course, error) {
	if e := s.gate(s.o.Flags.Training, true); e != nil {
		return v, e
	}
	v.ID = ids.New("crs")
	v.OrganizationID = t.OrganizationID
	v.Status = "ACTIVE"
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var e error
		v, e = s.repo.InsertCourse(ctx, tx, v)
		if e != nil {
			return e
		}
		return s.record(ctx, tx, t, "training.course_created.v1", "training_course", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) ListCourses(ctx context.Context, t tenant.Context) ([]models.Course, error) {
	if e := s.gate(s.o.Flags.Training, false); e != nil {
		return nil, e
	}
	return s.repo.ListCourses(ctx, s.o.Pool, t.OrganizationID)
}

// ListCoursesPage returns one keyset page of training courses, by code, and
// whether a further page exists.
func (s *Service) ListCoursesPage(ctx context.Context, t tenant.Context, f Page) ([]models.Course, bool, error) {
	if e := s.gate(s.o.Flags.Training, false); e != nil {
		return nil, false, e
	}
	f.Keyset = true
	rows, e := s.repo.ListCoursesPage(ctx, s.o.Pool, t.OrganizationID, query(t, f))
	if e != nil {
		return nil, false, e
	}
	rows, more := trim(rows, f)
	return rows, more, nil
}
func (s *Service) GetCourse(ctx context.Context, t tenant.Context, id string) (models.Course, error) {
	all, e := s.ListCourses(ctx, t)
	if e != nil {
		return models.Course{}, e
	}
	for _, v := range all {
		if v.ID == id {
			return v, nil
		}
	}
	return models.Course{}, apperrors.NotFound("Resource not found.", nil)
}

// UpdateCourse applies a partial update. A nil PassingScore keeps the stored
// threshold: a title-only change must never lower the pass mark to zero.
func (s *Service) UpdateCourse(ctx context.Context, t tenant.Context, id string, expected int64, p models.CoursePatch) (models.Course, error) {
	old, e := s.GetCourse(ctx, t, id)
	if e != nil {
		return models.Course{}, e
	}
	if expected <= 0 {
		return models.Course{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	v := old
	if p.Title != "" {
		v.Title = p.Title
	}
	if p.PassingScore != nil {
		if *p.PassingScore < 0 {
			return models.Course{}, apperrors.BadRequest("passingScore must not be negative.", nil)
		}
		v.PassingScore = *p.PassingScore
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		version, updated, e := s.repo.UpdateCourse(ctx, tx, t.OrganizationID, id, expected, v.Title, v.PassingScore)
		if e != nil {
			return apperrors.Conflict("Version conflict.", e)
		}
		v.Version, v.UpdatedAt = version, updated
		return s.record(ctx, tx, t, "training.course_updated.v1", "training_course", id, v.Version, v)
	})
	return v, e
}
func (s *Service) CompleteCourse(ctx context.Context, t tenant.Context, courseID string, v models.Completion) (models.Completion, error) {
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
		var e error
		v, e = s.repo.InsertCompletion(ctx, tx, v, t.OrganizationUserID)
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

func (s *Service) Evaluate(ctx context.Context, org, membership, service, assetType string, at time.Time) (models.Eligibility, error) {
	if s.o.Restrictions != nil {
		for _, subject := range [][2]string{{"ORGANIZATION", org}, {"PROVIDER", membership}} {
			restricted, err := s.o.Restrictions.Restricted(ctx, org, subject[0], subject[1])
			if err != nil {
				return models.Eligibility{Eligible: false, HardBlocks: []string{"RESTRICTION_AUTHORITY_UNAVAILABLE"}}, err
			}
			if restricted {
				return models.Eligibility{Eligible: false, HardBlocks: []string{"COMPLIANCE_RESTRICTION_ACTIVE"}}, nil
			}
		}
	}
	if !s.o.Flags.Workforce && !s.o.Flags.Qualifications && !s.o.Flags.Training {
		return models.Eligibility{Eligible: true, Reasons: []string{"workforce_policy_disabled"}}, nil
	}
	res := models.Eligibility{Eligible: true, Reasons: []string{}}
	role, status, e := s.repo.ProfileRoleStatus(ctx, s.o.Pool, org, membership)
	if e != nil || status != "ACTIVE" {
		res.HardBlocks = append(res.HardBlocks, "WORKFORCE_RELATIONSHIP_INACTIVE")
	}
	if leave, _ := s.repo.OnApprovedLeave(ctx, s.o.Pool, org, membership, at); leave {
		res.HardBlocks = append(res.HardBlocks, "APPROVED_LEAVE")
	}
	if shiftPolicy, shiftOK, _ := s.repo.ShiftCoverage(ctx, s.o.Pool, org, membership, at); shiftPolicy && !shiftOK {
		res.HardBlocks = append(res.HardBlocks, "SHIFT_NOT_ASSIGNED")
	}
	qualifications, e := s.repo.ApplicableQualificationIDs(ctx, s.o.Pool, org, service, assetType, role)
	if e != nil {
		return res, e
	}
	for _, id := range qualifications {
		if ok, _ := s.repo.HasVerifiedQualification(ctx, s.o.Pool, org, membership, id, at); !ok {
			res.HardBlocks = append(res.HardBlocks, "REQUIRED_QUALIFICATION_MISSING_OR_EXPIRED")
		}
	}
	courses, e := s.repo.ApplicableCourseIDs(ctx, s.o.Pool, org, service, role)
	if e != nil {
		return res, e
	}
	for _, id := range courses {
		if ok, _ := s.repo.HasPassedCourse(ctx, s.o.Pool, org, membership, id, at); !ok {
			res.HardBlocks = append(res.HardBlocks, "REQUIRED_TRAINING_MISSING_OR_EXPIRED")
		}
	}
	res.Eligible = len(res.HardBlocks) == 0
	if res.Eligible {
		res.Reasons = append(res.Reasons, "WORKFORCE_REQUIREMENTS_SATISFIED")
	}
	return res, nil
}

func (s *Service) ListPerformance(ctx context.Context, t tenant.Context, membership string) ([]models.PerformanceSnapshot, error) {
	if e := s.gate(s.o.Flags.Performance, false); e != nil {
		return nil, e
	}
	return s.repo.ListPerformance(ctx, s.o.Pool, t.OrganizationID, membership)
}
