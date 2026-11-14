package workforce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Shift templates and training enrolments (spec Sections 17 and 46).

// ShiftTemplate is a weekly repeating shift pattern in a branch timezone.
type ShiftTemplate struct {
	ID               string         `json:"id"`
	OrganizationID   string         `json:"organizationId"`
	BranchID         string         `json:"branchId"`
	Name             string         `json:"name"`
	Role             string         `json:"role"`
	Timezone         string         `json:"timezone"`
	Weekdays         []int          `json:"weekdays"`
	StartLocalTime   string         `json:"startLocalTime"`
	EndLocalTime     string         `json:"endLocalTime"`
	RequiredCapacity int            `json:"requiredCapacity"`
	BreakRules       map[string]any `json:"breakRules"`
	Status           string         `json:"status"`
	Version          int64          `json:"version"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

// GeneratedShifts reports what a template generation produced. Days whose
// local start or end does not exist (a DST gap) are skipped and listed:
// silently shifting a shift by an hour is the bug spec 46.2 warns about.
type GeneratedShifts struct {
	TemplateID   string   `json:"templateId"`
	Created      []Shift  `json:"created"`
	AlreadyExist []string `json:"alreadyExist"`
	SkippedDays  []string `json:"skippedDays"`
}

var localTimePattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

const templateCols = `id,organization_id,branch_id,name,role,timezone,weekdays,start_local_time,end_local_time,required_capacity,break_rules,status,version,created_at,updated_at`

func scanTemplate(row pgx.Row) (ShiftTemplate, error) {
	var v ShiftTemplate
	var days []int16
	var rules []byte
	e := row.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.Name, &v.Role, &v.Timezone, &days, &v.StartLocalTime,
		&v.EndLocalTime, &v.RequiredCapacity, &rules, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	for _, d := range days {
		v.Weekdays = append(v.Weekdays, int(d))
	}
	_ = json.Unmarshal(rules, &v.BreakRules)
	return v, e
}

func validateTemplate(v *ShiftTemplate) error {
	var bad []apperrors.FieldViolation
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" || len(v.Name) > 120 {
		bad = append(bad, apperrors.FieldViolation{Field: "name", Message: "Required."})
	}
	if _, e := time.LoadLocation(v.Timezone); e != nil || v.Timezone == "" || v.Timezone == "Local" {
		bad = append(bad, apperrors.FieldViolation{Field: "timezone", Message: "An IANA timezone such as Africa/Lagos."})
	}
	if !localTimePattern.MatchString(v.StartLocalTime) || !localTimePattern.MatchString(v.EndLocalTime) || v.StartLocalTime == v.EndLocalTime {
		bad = append(bad, apperrors.FieldViolation{Field: "startLocalTime", Message: "HH:MM start and a different HH:MM end."})
	}
	seen := map[int]bool{}
	days := []int{}
	for _, d := range v.Weekdays {
		if d < 1 || d > 7 {
			bad = append(bad, apperrors.FieldViolation{Field: "weekdays", Message: "Use 1 (Monday) to 7 (Sunday)."})
			break
		}
		if !seen[d] {
			seen[d] = true
			days = append(days, d)
		}
	}
	if len(days) == 0 {
		bad = append(bad, apperrors.FieldViolation{Field: "weekdays", Message: "Pick at least one day."})
	}
	sort.Ints(days)
	v.Weekdays = days
	if v.RequiredCapacity == 0 {
		v.RequiredCapacity = 1
	}
	if v.RequiredCapacity < 0 {
		bad = append(bad, apperrors.FieldViolation{Field: "requiredCapacity", Message: "Must be positive."})
	}
	if len(bad) > 0 {
		return apperrors.Validation("Check the shift template.", bad)
	}
	return nil
}

func (s *Service) branchActive(ctx context.Context, org, branch string) bool {
	var ok bool
	_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_branches WHERE organization_id=$1 AND id=$2 AND status='ACTIVE')`, org, branch).Scan(&ok)
	return ok
}

// CreateShiftTemplate stores a weekly shift pattern.
func (s *Service) CreateShiftTemplate(ctx context.Context, t tenant.Context, v ShiftTemplate) (ShiftTemplate, error) {
	if e := s.gate(s.o.Flags.Shifts, true); e != nil {
		return v, e
	}
	if e := validateTemplate(&v); e != nil {
		return v, e
	}
	if !t.BranchPermitted(v.BranchID) || !s.branchActive(ctx, t.OrganizationID, v.BranchID) {
		return v, apperrors.Validation("The branch is not available.", []apperrors.FieldViolation{{Field: "branchId", Message: "Unknown branch."}})
	}
	rules, _ := json.Marshal(orEmptyMap(v.BreakRules))
	var out ShiftTemplate
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		out, x = scanTemplate(tx.QueryRow(ctx, `INSERT INTO fleet_partner_shift_templates(id,organization_id,branch_id,name,role,timezone,weekdays,start_local_time,end_local_time,required_capacity,break_rules,created_by)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING `+templateCols,
			ids.New("sht"), t.OrganizationID, v.BranchID, v.Name, v.Role, v.Timezone, v.Weekdays, v.StartLocalTime, v.EndLocalTime, v.RequiredCapacity, rules, t.OrganizationUserID))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "shift_template.created.v1", "shift_template", out.ID, out.Version, out)
	})
	return out, e
}

func orEmptyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// ListShiftTemplates returns templates in branches the caller may see.
func (s *Service) ListShiftTemplates(ctx context.Context, t tenant.Context, f Page) ([]ShiftTemplate, error) {
	if e := s.gate(s.o.Flags.Shifts, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+templateCols+` FROM fleet_partner_shift_templates WHERE organization_id=$1 AND($2=''OR branch_id=$2) AND($3=''OR status=$3) ORDER BY name,id LIMIT $4 OFFSET $5`,
		t.OrganizationID, f.BranchID, f.Status, limit(f.Limit), pagination.ClampOffset(f.Offset))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []ShiftTemplate{}
	for rows.Next() {
		v, e := scanTemplate(rows)
		if e != nil {
			return nil, e
		}
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, rows.Err()
}

// GetShiftTemplate loads one template inside the tenant and branch scope.
func (s *Service) GetShiftTemplate(ctx context.Context, t tenant.Context, id string) (ShiftTemplate, error) {
	if e := s.gate(s.o.Flags.Shifts, false); e != nil {
		return ShiftTemplate{}, e
	}
	v, e := scanTemplate(s.o.Pool.QueryRow(ctx, `SELECT `+templateCols+` FROM fleet_partner_shift_templates WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil || !t.BranchPermitted(v.BranchID) {
		return ShiftTemplate{}, apperrors.NotFound("Resource not found.", nil)
	}
	return v, nil
}

// TemplatePatch carries changeable template fields.
type TemplatePatch struct {
	ExpectedVersion  int64          `json:"expectedVersion"`
	Name             string         `json:"name"`
	Role             *string        `json:"role"`
	Weekdays         []int          `json:"weekdays"`
	StartLocalTime   string         `json:"startLocalTime"`
	EndLocalTime     string         `json:"endLocalTime"`
	RequiredCapacity int            `json:"requiredCapacity"`
	BreakRules       map[string]any `json:"breakRules"`
	Status           string         `json:"status"`
}

// UpdateShiftTemplate edits a template. Shifts already generated keep the
// times they were published with.
func (s *Service) UpdateShiftTemplate(ctx context.Context, t tenant.Context, id string, expected int64, p TemplatePatch) (ShiftTemplate, error) {
	if e := s.gate(s.o.Flags.Shifts, true); e != nil {
		return ShiftTemplate{}, e
	}
	old, e := s.GetShiftTemplate(ctx, t, id)
	if e != nil {
		return old, e
	}
	if expected <= 0 {
		expected = old.Version
	}
	next := old
	if p.Name != "" {
		next.Name = p.Name
	}
	if p.Role != nil {
		next.Role = *p.Role
	}
	if p.Weekdays != nil {
		next.Weekdays = p.Weekdays
	}
	if p.StartLocalTime != "" {
		next.StartLocalTime = p.StartLocalTime
	}
	if p.EndLocalTime != "" {
		next.EndLocalTime = p.EndLocalTime
	}
	if p.RequiredCapacity != 0 {
		next.RequiredCapacity = p.RequiredCapacity
	}
	if p.BreakRules != nil {
		next.BreakRules = p.BreakRules
	}
	if p.Status != "" {
		next.Status = strings.ToUpper(p.Status)
		if next.Status != "ACTIVE" && next.Status != "ARCHIVED" {
			return old, apperrors.Validation("Status must be ACTIVE or ARCHIVED.", []apperrors.FieldViolation{{Field: "status", Message: "Invalid."}})
		}
	}
	if e := validateTemplate(&next); e != nil {
		return old, e
	}
	rules, _ := json.Marshal(orEmptyMap(next.BreakRules))
	var out ShiftTemplate
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		out, x = scanTemplate(tx.QueryRow(ctx, `UPDATE fleet_partner_shift_templates SET name=$4,role=$5,weekdays=$6,start_local_time=$7,end_local_time=$8,required_capacity=$9,break_rules=$10,status=$11,version=version+1,updated_at=now()
			WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+templateCols,
			t.OrganizationID, id, expected, next.Name, next.Role, next.Weekdays, next.StartLocalTime, next.EndLocalTime, next.RequiredCapacity, rules, next.Status))
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Version conflict.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "shift_template.changed.v1", "shift_template", id, out.Version, out)
	})
	return out, e
}

// GenerateInput asks for shifts over an inclusive local date range.
type GenerateInput struct {
	From string `json:"from"` // YYYY-MM-DD, template timezone
	To   string `json:"to"`
}

// localInstant builds a wall-clock time and reports whether it exists
// exactly (false inside a DST gap, where Go would silently normalize).
func localInstant(day time.Time, hhmm string, loc *time.Location) (time.Time, bool) {
	var h, m int
	_, _ = fmt.Sscanf(hhmm, "%d:%d", &h, &m)
	v := time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, loc)
	return v, v.Hour() == h && v.Minute() == m
}

// GenerateShifts turns a template into DRAFT shifts for each matching day.
// Re-running for an overlapping range never duplicates a day.
func (s *Service) GenerateShifts(ctx context.Context, t tenant.Context, id string, in GenerateInput) (GeneratedShifts, error) {
	out := GeneratedShifts{TemplateID: id, Created: []Shift{}, AlreadyExist: []string{}, SkippedDays: []string{}}
	if e := s.gate(s.o.Flags.Shifts, true); e != nil {
		return out, e
	}
	tpl, e := s.GetShiftTemplate(ctx, t, id)
	if e != nil {
		return out, e
	}
	if tpl.Status != "ACTIVE" {
		return out, apperrors.Conflict("An archived template cannot generate shifts.", nil)
	}
	loc, _ := time.LoadLocation(tpl.Timezone)
	from, e1 := time.ParseInLocation("2006-01-02", in.From, loc)
	to, e2 := time.ParseInLocation("2006-01-02", in.To, loc)
	if e1 != nil || e2 != nil || to.Before(from) || to.Sub(from) > 62*24*time.Hour {
		return out, apperrors.Validation("Give from and to dates (YYYY-MM-DD), at most 62 days apart.",
			[]apperrors.FieldViolation{{Field: "from", Message: "Invalid range."}})
	}
	wanted := map[int]bool{}
	for _, d := range tpl.Weekdays {
		wanted[d] = true
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
			iso := int(day.Weekday())
			if iso == 0 {
				iso = 7
			}
			if !wanted[iso] {
				continue
			}
			label := day.Format("2006-01-02")
			start, okStart := localInstant(day, tpl.StartLocalTime, loc)
			endDay := day
			if tpl.EndLocalTime <= tpl.StartLocalTime { // overnight
				endDay = day.AddDate(0, 0, 1)
			}
			end, okEnd := localInstant(endDay, tpl.EndLocalTime, loc)
			if !okStart || !okEnd || !end.After(start) {
				out.SkippedDays = append(out.SkippedDays, label)
				continue
			}
			v, x := scanShift(tx.QueryRow(ctx, `INSERT INTO fleet_partner_shifts(id,organization_id,branch_id,name,role,start_at,end_at,timezone,required_capacity,planner_user_id,template_id,template_local_date)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
				ON CONFLICT (template_id, template_local_date) WHERE template_id IS NOT NULL DO NOTHING
				RETURNING `+shiftCols,
				ids.New("sft"), t.OrganizationID, tpl.BranchID, tpl.Name, tpl.Role, start.UTC(), end.UTC(), tpl.Timezone, tpl.RequiredCapacity, t.OrganizationUserID, tpl.ID, label))
			if errors.Is(x, pgx.ErrNoRows) {
				out.AlreadyExist = append(out.AlreadyExist, label)
				continue
			}
			if x != nil {
				return x
			}
			_ = history(ctx, tx, t, "SHIFT", v.ID, "", v.Status, "generated_from_template", v.Version, v)
			if x = s.record(ctx, tx, t, "shift.created.v1", "shift", v.ID, v.Version, v); x != nil {
				return x
			}
			out.Created = append(out.Created, v)
		}
		return nil
	})
	return out, e
}

// Enrollment assigns a training course to a staff member or provider.
type Enrollment struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organizationId"`
	CourseID       string     `json:"courseId"`
	CourseVersion  int64      `json:"courseVersion"`
	MembershipID   string     `json:"membershipId"`
	Status         string     `json:"status"`
	AssignedBy     string     `json:"assignedBy"`
	AssignedAt     time.Time  `json:"assignedAt"`
	DueAt          *time.Time `json:"dueAt"`
	CompletedAt    *time.Time `json:"completedAt"`
	Score          *float64   `json:"score"`
	CompletionID   *string    `json:"completionId"`
	Version        int64      `json:"version"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

const enrollmentCols = `id,organization_id,course_id,course_version,membership_id,status,assigned_by,assigned_at,due_at,completed_at,score::float8,completion_id,version,created_at,updated_at`

func scanEnrollment(row pgx.Row) (Enrollment, error) {
	var v Enrollment
	e := row.Scan(&v.ID, &v.OrganizationID, &v.CourseID, &v.CourseVersion, &v.MembershipID, &v.Status, &v.AssignedBy,
		&v.AssignedAt, &v.DueAt, &v.CompletedAt, &v.Score, &v.CompletionID, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

// EnrollInput assigns a course.
type EnrollInput struct {
	CourseID     string     `json:"courseId"`
	MembershipID string     `json:"membershipId"`
	DueAt        *time.Time `json:"dueAt"`
}

// Enroll assigns an active course to an active member, once.
func (s *Service) Enroll(ctx context.Context, t tenant.Context, in EnrollInput) (Enrollment, error) {
	if e := s.gate(s.o.Flags.Training, true); e != nil {
		return Enrollment{}, e
	}
	course, e := s.GetCourse(ctx, t, in.CourseID)
	if e != nil {
		return Enrollment{}, e
	}
	if course.Status != "ACTIVE" {
		return Enrollment{}, apperrors.Conflict("This course is not active.", nil)
	}
	if !s.memberExists(ctx, s.o.Pool, t.OrganizationID, in.MembershipID) {
		return Enrollment{}, apperrors.NotFound("Membership not found.", nil)
	}
	if in.DueAt != nil && !in.DueAt.After(s.o.Now()) {
		return Enrollment{}, apperrors.Validation("The due date must be in the future.", []apperrors.FieldViolation{{Field: "dueAt", Message: "In the past."}})
	}
	var v Enrollment
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanEnrollment(tx.QueryRow(ctx, `INSERT INTO fleet_partner_training_enrollments(id,organization_id,course_id,course_version,membership_id,assigned_by,due_at)
			VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+enrollmentCols,
			ids.New("tre"), t.OrganizationID, course.ID, course.Version, in.MembershipID, t.OrganizationUserID, in.DueAt))
		var pgErr *pgconn.PgError
		if errors.As(x, &pgErr) && pgErr.Code == "23505" {
			return apperrors.Conflict("This person already has an open enrolment for this course.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "training.assigned.v1", "training_enrollment", v.ID, v.Version, v)
	})
	return v, e
}

// ListEnrollments returns enrolments, optionally for one member or status.
func (s *Service) ListEnrollments(ctx context.Context, t tenant.Context, membershipID string, f Page) ([]Enrollment, error) {
	if e := s.gate(s.o.Flags.Training, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+enrollmentCols+` FROM fleet_partner_training_enrollments
		WHERE organization_id=$1 AND($2=''OR membership_id=$2) AND($3=''OR status=$3) ORDER BY assigned_at DESC,id LIMIT $4 OFFSET $5`,
		t.OrganizationID, membershipID, f.Status, limit(f.Limit), pagination.ClampOffset(f.Offset))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Enrollment{}
	for rows.Next() {
		v, e := scanEnrollment(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) getEnrollment(ctx context.Context, t tenant.Context, id string) (Enrollment, error) {
	v, e := scanEnrollment(s.o.Pool.QueryRow(ctx, `SELECT `+enrollmentCols+` FROM fleet_partner_training_enrollments WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil {
		return v, apperrors.NotFound("Resource not found.", nil)
	}
	return v, nil
}

// CompleteEnrollment records the result through the existing course
// completion (which keeps evidence and certificate expiry) and closes the
// enrolment with it.
func (s *Service) CompleteEnrollment(ctx context.Context, t tenant.Context, id string, c Completion) (Enrollment, error) {
	if e := s.gate(s.o.Flags.Training, true); e != nil {
		return Enrollment{}, e
	}
	enrollment, e := s.getEnrollment(ctx, t, id)
	if e != nil {
		return enrollment, e
	}
	if enrollment.Status != "ASSIGNED" {
		return enrollment, apperrors.Conflict("This enrolment is already closed.", nil)
	}
	c.MembershipID = enrollment.MembershipID
	completion, e := s.CompleteCourse(ctx, t, enrollment.CourseID, c)
	if e != nil {
		return enrollment, e
	}
	var v Enrollment
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanEnrollment(tx.QueryRow(ctx, `UPDATE fleet_partner_training_enrollments SET status=$3,completed_at=$4,score=$5,completion_id=$6,version=version+1,updated_at=now()
			WHERE organization_id=$1 AND id=$2 AND status='ASSIGNED' RETURNING `+enrollmentCols,
			t.OrganizationID, id, completion.Status, completion.CompletedAt, completion.Score, completion.ID))
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("This enrolment is already closed.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "training.completed.v1", "training_enrollment", v.ID, v.Version, v)
	})
	return v, e
}

// CancelEnrollment withdraws an open enrolment.
func (s *Service) CancelEnrollment(ctx context.Context, t tenant.Context, id string) (Enrollment, error) {
	if e := s.gate(s.o.Flags.Training, true); e != nil {
		return Enrollment{}, e
	}
	var v Enrollment
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = scanEnrollment(tx.QueryRow(ctx, `UPDATE fleet_partner_training_enrollments SET status='CANCELLED',version=version+1,updated_at=now()
			WHERE organization_id=$1 AND id=$2 AND status='ASSIGNED' RETURNING `+enrollmentCols, t.OrganizationID, id))
		if errors.Is(x, pgx.ErrNoRows) {
			if _, getErr := s.getEnrollment(ctx, t, id); getErr != nil {
				return getErr
			}
			return apperrors.Conflict("This enrolment is already closed.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "training.enrollment_cancelled.v1", "training_enrollment", v.ID, v.Version, v)
	})
	return v, e
}
