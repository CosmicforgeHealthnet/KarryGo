package usecases

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

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Shift templates and training enrolments (spec Sections 17 and 46).

var localTimePattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

func validateTemplate(v *models.ShiftTemplate) error {
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
	return s.repo.BranchActive(ctx, s.o.Pool, org, branch)
}

// CreateShiftTemplate stores a weekly shift pattern.
func (s *Service) CreateShiftTemplate(ctx context.Context, t tenant.Context, v models.ShiftTemplate) (models.ShiftTemplate, error) {
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
	var out models.ShiftTemplate
	e := s.tx(ctx, func(tx pgx.Tx) error {
		v.OrganizationID = t.OrganizationID
		var x error
		out, x = s.repo.InsertTemplate(ctx, tx, v, rules, t.OrganizationUserID)
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
func (s *Service) ListShiftTemplates(ctx context.Context, t tenant.Context, f Page) ([]models.ShiftTemplate, error) {
	out, _, e := s.ListShiftTemplatesPage(ctx, t, f)
	return out, e
}

// ListShiftTemplatesPage returns one page of templates and, for keyset pages,
// whether a further page exists.
func (s *Service) ListShiftTemplatesPage(ctx context.Context, t tenant.Context, f Page) ([]models.ShiftTemplate, bool, error) {
	if e := s.gate(s.o.Flags.Shifts, false); e != nil {
		return nil, false, e
	}
	if e := validPage(f); e != nil {
		return nil, false, e
	}
	rows, e := s.repo.ListTemplates(ctx, s.o.Pool, t.OrganizationID, query(t, f))
	if e != nil {
		return nil, false, e
	}
	rows, more := trim(rows, f)
	out := []models.ShiftTemplate{}
	for _, v := range rows {
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, more, nil
}

// GetShiftTemplate loads one template inside the tenant and branch scope.
func (s *Service) GetShiftTemplate(ctx context.Context, t tenant.Context, id string) (models.ShiftTemplate, error) {
	if e := s.gate(s.o.Flags.Shifts, false); e != nil {
		return models.ShiftTemplate{}, e
	}
	v, e := s.repo.GetTemplate(ctx, s.o.Pool, t.OrganizationID, id)
	if e != nil || !t.BranchPermitted(v.BranchID) {
		return models.ShiftTemplate{}, apperrors.NotFound("Resource not found.", nil)
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
func (s *Service) UpdateShiftTemplate(ctx context.Context, t tenant.Context, id string, expected int64, p TemplatePatch) (models.ShiftTemplate, error) {
	if e := s.gate(s.o.Flags.Shifts, true); e != nil {
		return models.ShiftTemplate{}, e
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
	var out models.ShiftTemplate
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		out, x = s.repo.UpdateTemplate(ctx, tx, t.OrganizationID, id, expected, next, rules)
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
func (s *Service) GenerateShifts(ctx context.Context, t tenant.Context, id string, in GenerateInput) (models.GeneratedShifts, error) {
	out := models.GeneratedShifts{TemplateID: id, Created: []models.Shift{}, AlreadyExist: []string{}, SkippedDays: []string{}}
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
	tpl.OrganizationID = t.OrganizationID
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
			v, x := s.repo.InsertTemplateShift(ctx, tx, tpl, start.UTC(), end.UTC(), t.OrganizationUserID, label)
			if errors.Is(x, pgx.ErrNoRows) {
				out.AlreadyExist = append(out.AlreadyExist, label)
				continue
			}
			if x != nil {
				return x
			}
			_ = s.history(ctx, tx, t, "SHIFT", v.ID, "", v.Status, "generated_from_template", v.Version, v)
			if x = s.record(ctx, tx, t, "shift.created.v1", "shift", v.ID, v.Version, v); x != nil {
				return x
			}
			out.Created = append(out.Created, v)
		}
		return nil
	})
	return out, e
}

// EnrollInput assigns a course.
type EnrollInput struct {
	CourseID     string     `json:"courseId"`
	MembershipID string     `json:"membershipId"`
	DueAt        *time.Time `json:"dueAt"`
}

// Enroll assigns an active course to an active member, once.
func (s *Service) Enroll(ctx context.Context, t tenant.Context, in EnrollInput) (models.Enrollment, error) {
	if e := s.gate(s.o.Flags.Training, true); e != nil {
		return models.Enrollment{}, e
	}
	course, e := s.GetCourse(ctx, t, in.CourseID)
	if e != nil {
		return models.Enrollment{}, e
	}
	if course.Status != "ACTIVE" {
		return models.Enrollment{}, apperrors.Conflict("This course is not active.", nil)
	}
	if !s.memberExists(ctx, s.o.Pool, t.OrganizationID, in.MembershipID) {
		return models.Enrollment{}, apperrors.NotFound("Membership not found.", nil)
	}
	if in.DueAt != nil && !in.DueAt.After(s.o.Now()) {
		return models.Enrollment{}, apperrors.Validation("The due date must be in the future.", []apperrors.FieldViolation{{Field: "dueAt", Message: "In the past."}})
	}
	var v models.Enrollment
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.InsertEnrollment(ctx, tx, t.OrganizationID, course, in.MembershipID, t.OrganizationUserID, in.DueAt)
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
func (s *Service) ListEnrollments(ctx context.Context, t tenant.Context, membershipID string, f Page) ([]models.Enrollment, error) {
	out, _, e := s.ListEnrollmentsPage(ctx, t, membershipID, f)
	return out, e
}

// ListEnrollmentsPage returns one page of enrolments and, for keyset pages,
// whether a further page exists.
func (s *Service) ListEnrollmentsPage(ctx context.Context, t tenant.Context, membershipID string, f Page) ([]models.Enrollment, bool, error) {
	if e := s.gate(s.o.Flags.Training, false); e != nil {
		return nil, false, e
	}
	if e := validPage(f); e != nil {
		return nil, false, e
	}
	q := query(t, f)
	q.MembershipID = membershipID
	rows, e := s.repo.ListEnrollments(ctx, s.o.Pool, t.OrganizationID, q)
	if e != nil {
		return nil, false, e
	}
	rows, more := trim(rows, f)
	return rows, more, nil
}

func (s *Service) getEnrollment(ctx context.Context, t tenant.Context, id string) (models.Enrollment, error) {
	v, e := s.repo.GetEnrollment(ctx, s.o.Pool, t.OrganizationID, id)
	if e != nil {
		return v, apperrors.NotFound("Resource not found.", nil)
	}
	return v, nil
}

// CompleteEnrollment records the result through the existing course
// completion (which keeps evidence and certificate expiry) and closes the
// enrolment with it.
func (s *Service) CompleteEnrollment(ctx context.Context, t tenant.Context, id string, c models.Completion) (models.Enrollment, error) {
	if e := s.gate(s.o.Flags.Training, true); e != nil {
		return models.Enrollment{}, e
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
	var v models.Enrollment
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.CompleteEnrollment(ctx, tx, t.OrganizationID, id, completion.Status, completion.CompletedAt, completion.Score, completion.ID)
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
func (s *Service) CancelEnrollment(ctx context.Context, t tenant.Context, id string) (models.Enrollment, error) {
	if e := s.gate(s.o.Flags.Training, true); e != nil {
		return models.Enrollment{}, e
	}
	var v models.Enrollment
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.CancelEnrollment(ctx, tx, t.OrganizationID, id)
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
