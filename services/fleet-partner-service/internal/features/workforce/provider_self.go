package workforce

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Provider self-view (spec 46.2 "providers can view their own schedules,
// qualifications, training and explainable performance inputs",
// FLEET-17.15).
//
// Providers never call Fleet directly. Their owning provider service
// (driver-dispatch-delivery-service, driver-taxi-service,
// driver-hauling-service) authenticates the provider session and calls these
// INTERNAL, HMAC-authenticated routes on the provider's behalf. The
// membership is resolved from the verified caller service plus the provider
// id it vouches for -- never from request input naming a membership -- so a
// provider can only ever read their own records.
//
// Every response is an explicit, provider-facing DTO built field by field.
// No fare, customer price, commission, margin, settlement amount or other
// organization financial data is read, and staff identifiers (who verified,
// who assigned) are not returned.

// ProviderSelfResolver resolves the live membership a provider-service caller
// is speaking for. On failure it writes the error response itself and
// returns ok=false.
type ProviderSelfResolver interface {
	ResolveMembership(c *gin.Context) (organizationID string, membershipID string, ok bool)
}

// InternalRoute is one internal provider self-view operation.
type InternalRoute struct {
	Method, Path, OperationID string
}

// InternalProviderRoutes lists the provider self-view routes, mounted under
// /internal/fleet-partner/v1.
func InternalProviderRoutes() []InternalRoute {
	return []InternalRoute{
		{"GET", "/providers/:providerId/schedule", "getProviderOwnSchedule"},
		{"GET", "/providers/:providerId/qualifications", "getProviderOwnQualifications"},
		{"GET", "/providers/:providerId/training", "getProviderOwnTraining"},
		{"GET", "/providers/:providerId/performance", "getProviderOwnPerformance"},
	}
}

// InternalProviderHandler serves the provider self-view.
type InternalProviderHandler struct {
	s        *Service
	resolver ProviderSelfResolver
}

// NewInternalProviderHandler builds the handler.
func NewInternalProviderHandler(s *Service, resolver ProviderSelfResolver) *InternalProviderHandler {
	return &InternalProviderHandler{s: s, resolver: resolver}
}

// RegisterInternalProviderRoutes mounts the self-view routes. Without a
// resolver there is no trustworthy way to know whose records are being read,
// so nothing is mounted.
func RegisterInternalProviderRoutes(group *gin.RouterGroup, h *InternalProviderHandler) {
	if h == nil || h.resolver == nil {
		return
	}
	for _, route := range InternalProviderRoutes() {
		route := route
		group.Handle(route.Method, route.Path, func(c *gin.Context) { h.handle(c, route.OperationID) })
	}
}

func (h *InternalProviderHandler) handle(c *gin.Context, operation string) {
	org, membership, ok := h.resolver.ResolveMembership(c)
	if !ok {
		if !c.Writer.Written() {
			respond.Error(c, apperrors.Forbidden("Provider authentication is required.", nil))
		}
		return
	}
	ctx := c.Request.Context()
	var (
		data any
		err  error
	)
	switch operation {
	case "getProviderOwnSchedule":
		var from, to time.Time
		if from, to, err = scheduleWindow(c.Query("from"), c.Query("to"), h.s.o.Now().UTC()); err == nil {
			data, err = h.s.ProviderOwnSchedule(ctx, org, membership, from, to)
		}
	case "getProviderOwnQualifications":
		data, err = h.s.ProviderOwnQualifications(ctx, org, membership)
	case "getProviderOwnTraining":
		data, err = h.s.ProviderOwnTraining(ctx, org, membership)
	case "getProviderOwnPerformance":
		data, err = h.s.ProviderOwnPerformance(ctx, org, membership)
	default:
		err = apperrors.NotFound("Resource not found.", nil)
	}
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, data)
}

// maxScheduleWindow bounds one schedule read.
const maxScheduleWindow = 92 * 24 * time.Hour

func scheduleWindow(rawFrom, rawTo string, now time.Time) (time.Time, time.Time, error) {
	from, to := now.Add(-7*24*time.Hour), now.Add(30*24*time.Hour)
	invalid := apperrors.Validation("from and to must be RFC 3339 instants, from before to, at most 92 days apart.",
		[]apperrors.FieldViolation{{Field: "from", Message: "Invalid window."}})
	if v := strings.TrimSpace(rawFrom); v != "" {
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return from, to, invalid
		}
		from = parsed.UTC()
	}
	if v := strings.TrimSpace(rawTo); v != "" {
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return from, to, invalid
		}
		to = parsed.UTC()
	}
	if !to.After(from) || to.Sub(from) > maxScheduleWindow {
		return from, to, invalid
	}
	return from, to, nil
}

func selfScope(org, membership string) tenant.Context {
	return tenant.Context{OrganizationID: org, ActorType: tenant.ActorTypeSystem, ActorID: "provider:" + membership}
}

// ---- Schedule ----

// OwnShift is one of the provider's shift assignments.
type OwnShift struct {
	ShiftAssignmentID string     `json:"shiftAssignmentId"`
	ShiftID           string     `json:"shiftId"`
	ShiftName         string     `json:"shiftName"`
	Role              string     `json:"role"`
	BranchID          string     `json:"branchId"`
	BranchName        string     `json:"branchName"`
	Timezone          string     `json:"timezone"`
	AssignmentType    string     `json:"assignmentType"`
	Status            string     `json:"status"`
	ShiftStatus       string     `json:"shiftStatus"`
	ExpectedStart     time.Time  `json:"expectedStart"`
	ExpectedEnd       time.Time  `json:"expectedEnd"`
	CheckedInAt       *time.Time `json:"checkedInAt,omitempty"`
	CheckedOutAt      *time.Time `json:"checkedOutAt,omitempty"`
}

// OwnLeave is one of the provider's leave records.
type OwnLeave struct {
	LeaveID   string    `json:"leaveId"`
	LeaveType string    `json:"leaveType"`
	Status    string    `json:"status"`
	StartAt   time.Time `json:"startAt"`
	EndAt     time.Time `json:"endAt"`
}

// OwnSchedule is the provider's schedule over a window.
type OwnSchedule struct {
	MembershipID string     `json:"membershipId"`
	From         time.Time  `json:"from"`
	To           time.Time  `json:"to"`
	Shifts       []OwnShift `json:"shifts"`
	Leave        []OwnLeave `json:"leave"`
}

// ProviderOwnSchedule returns shift assignments (draft shifts excluded: a
// plan the company has not published is not yet the provider's schedule) and
// leave overlapping [from, to).
func (s *Service) ProviderOwnSchedule(ctx context.Context, org, membership string, from, to time.Time) (OwnSchedule, error) {
	if s.o.Flags.KillSwitch || (!s.o.Flags.Shifts && !s.o.Flags.Leave) {
		return OwnSchedule{}, s.gate(false, false)
	}
	out := OwnSchedule{MembershipID: membership, From: from, To: to, Shifts: []OwnShift{}, Leave: []OwnLeave{}}
	if s.o.Flags.Shifts {
		rows, err := s.o.Pool.Query(ctx, `
			SELECT a.id, s.id, s.name, s.role, s.branch_id, COALESCE(b.name, ''), s.timezone, a.assignment_type,
			       a.status, s.status, a.expected_start, a.expected_end, a.checked_in_at, a.checked_out_at
			FROM fleet_partner_shift_assignments a
			JOIN fleet_partner_shifts s ON s.organization_id = a.organization_id AND s.id = a.shift_id
			LEFT JOIN fleet_partner_branches b ON b.organization_id = s.organization_id AND b.id = s.branch_id
			WHERE a.organization_id = $1 AND a.membership_id = $2 AND s.status <> 'DRAFT'
			  AND a.expected_start < $4 AND a.expected_end > $3
			ORDER BY a.expected_start, a.id LIMIT 500`, org, membership, from, to)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var v OwnShift
			if err := rows.Scan(&v.ShiftAssignmentID, &v.ShiftID, &v.ShiftName, &v.Role, &v.BranchID, &v.BranchName,
				&v.Timezone, &v.AssignmentType, &v.Status, &v.ShiftStatus, &v.ExpectedStart, &v.ExpectedEnd,
				&v.CheckedInAt, &v.CheckedOutAt); err != nil {
				rows.Close()
				return out, err
			}
			out.Shifts = append(out.Shifts, v)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return out, err
		}
	}
	if s.o.Flags.Leave {
		rows, err := s.o.Pool.Query(ctx, `
			SELECT id, leave_type, status, start_at, end_at FROM fleet_partner_leave_records
			WHERE organization_id = $1 AND membership_id = $2 AND start_at < $4 AND end_at > $3
			ORDER BY start_at, id LIMIT 200`, org, membership, from, to)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var v OwnLeave
			if err := rows.Scan(&v.LeaveID, &v.LeaveType, &v.Status, &v.StartAt, &v.EndAt); err != nil {
				rows.Close()
				return out, err
			}
			out.Leave = append(out.Leave, v)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return out, err
		}
	}
	return out, nil
}

// ---- Qualifications ----

// OwnQualification is one qualification record of the provider, or a
// mandatory qualification they do not yet hold (Status MISSING).
type OwnQualification struct {
	ProviderQualificationID string     `json:"providerQualificationId,omitempty"`
	QualificationID         string     `json:"qualificationId"`
	Code                    string     `json:"code"`
	Name                    string     `json:"name"`
	Status                  string     `json:"status"`
	Mandatory               bool       `json:"mandatory"`
	EvidenceProvided        bool       `json:"evidenceProvided"`
	IssuedAt                *time.Time `json:"issuedAt,omitempty"`
	ExpiresAt               *time.Time `json:"expiresAt,omitempty"`
	VerifiedAt              *time.Time `json:"verifiedAt,omitempty"`
	Expired                 bool       `json:"expired"`
}

// ProviderOwnQualifications lists the provider's qualification records plus
// every ACTIVE qualification mandatory for their service category that they
// do not hold.
func (s *Service) ProviderOwnQualifications(ctx context.Context, org, membership string) ([]OwnQualification, error) {
	if e := s.gate(s.o.Flags.Qualifications, false); e != nil {
		return nil, e
	}
	now := s.o.Now().UTC()
	rows, err := s.o.Pool.Query(ctx, `
		WITH member AS (
			SELECT service_category FROM fleet_partner_organization_provider_memberships
			WHERE organization_id = $1 AND id = $2
		)
		SELECT pq.id, q.id, q.code, q.name, pq.status,
		       q.status = 'ACTIVE' AND q.applicable_services ? COALESCE((SELECT service_category FROM member), ''),
		       pq.evidence_media_id <> '', pq.issued_at, pq.expires_at, pq.verified_at
		FROM fleet_partner_provider_qualifications pq
		JOIN fleet_partner_qualifications q ON q.organization_id = pq.organization_id AND q.id = pq.qualification_id
		WHERE pq.organization_id = $1 AND pq.membership_id = $2
		UNION ALL
		SELECT '', q.id, q.code, q.name, 'MISSING', true, false, NULL, NULL, NULL
		FROM fleet_partner_qualifications q
		WHERE q.organization_id = $1 AND q.status = 'ACTIVE'
		  AND q.applicable_services ? COALESCE((SELECT service_category FROM member), '')
		  AND NOT EXISTS (SELECT 1 FROM fleet_partner_provider_qualifications pq
		      WHERE pq.organization_id = $1 AND pq.membership_id = $2 AND pq.qualification_id = q.id)
		ORDER BY 3, 1`, org, membership)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OwnQualification{}
	for rows.Next() {
		var v OwnQualification
		if err := rows.Scan(&v.ProviderQualificationID, &v.QualificationID, &v.Code, &v.Name, &v.Status, &v.Mandatory,
			&v.EvidenceProvided, &v.IssuedAt, &v.ExpiresAt, &v.VerifiedAt); err != nil {
			return nil, err
		}
		v.Expired = v.Status == "EXPIRED" || (v.ExpiresAt != nil && !v.ExpiresAt.After(now))
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---- Training ----

// OwnTrainingEnrollment is one course assigned to the provider.
type OwnTrainingEnrollment struct {
	EnrollmentID  string     `json:"enrollmentId"`
	CourseID      string     `json:"courseId"`
	CourseCode    string     `json:"courseCode"`
	CourseTitle   string     `json:"courseTitle"`
	CourseVersion int64      `json:"courseVersion"`
	Status        string     `json:"status"`
	AssignedAt    time.Time  `json:"assignedAt"`
	DueAt         *time.Time `json:"dueAt,omitempty"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
}

// OwnTrainingCompletion is one recorded course result of the provider.
type OwnTrainingCompletion struct {
	CompletionID       string     `json:"completionId"`
	CourseID           string     `json:"courseId"`
	CourseCode         string     `json:"courseCode"`
	CourseTitle        string     `json:"courseTitle"`
	Status             string     `json:"status"`
	Score              float64    `json:"score"`
	CertificateOnFile  bool       `json:"certificateOnFile"`
	CompletedAt        time.Time  `json:"completedAt"`
	CertificateExpires *time.Time `json:"certificateExpiresAt,omitempty"`
}

// OwnTraining is the provider's training record.
type OwnTraining struct {
	MembershipID string                  `json:"membershipId"`
	Enrollments  []OwnTrainingEnrollment `json:"enrollments"`
	Completions  []OwnTrainingCompletion `json:"completions"`
}

// ProviderOwnTraining returns the provider's enrolments and completions.
func (s *Service) ProviderOwnTraining(ctx context.Context, org, membership string) (OwnTraining, error) {
	out := OwnTraining{MembershipID: membership, Enrollments: []OwnTrainingEnrollment{}, Completions: []OwnTrainingCompletion{}}
	if e := s.gate(s.o.Flags.Training, false); e != nil {
		return out, e
	}
	rows, err := s.o.Pool.Query(ctx, `
		SELECT e.id, c.id, c.code, c.title, e.course_version, e.status, e.assigned_at, e.due_at, e.completed_at
		FROM fleet_partner_training_enrollments e
		JOIN fleet_partner_training_courses c ON c.organization_id = e.organization_id AND c.id = e.course_id
		WHERE e.organization_id = $1 AND e.membership_id = $2
		ORDER BY e.assigned_at DESC, e.id LIMIT 200`, org, membership)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v OwnTrainingEnrollment
		if err := rows.Scan(&v.EnrollmentID, &v.CourseID, &v.CourseCode, &v.CourseTitle, &v.CourseVersion, &v.Status,
			&v.AssignedAt, &v.DueAt, &v.CompletedAt); err != nil {
			rows.Close()
			return out, err
		}
		out.Enrollments = append(out.Enrollments, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	rows, err = s.o.Pool.Query(ctx, `
		SELECT t.id, c.id, c.code, c.title, t.status, t.score::float8, t.certificate_media_id <> '', t.completed_at, t.expires_at
		FROM fleet_partner_training_completions t
		JOIN fleet_partner_training_courses c ON c.organization_id = t.organization_id AND c.id = t.course_id
		WHERE t.organization_id = $1 AND t.membership_id = $2
		ORDER BY t.completed_at DESC, t.id LIMIT 200`, org, membership)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v OwnTrainingCompletion
		if err := rows.Scan(&v.CompletionID, &v.CourseID, &v.CourseCode, &v.CourseTitle, &v.Status, &v.Score,
			&v.CertificateOnFile, &v.CompletedAt, &v.CertificateExpires); err != nil {
			return out, err
		}
		out.Completions = append(out.Completions, v)
	}
	return out, rows.Err()
}

// ---- Performance ----

// ProviderOwnPerformance returns the provider's performance snapshots with
// their full inputs, explanation and source versions. Snapshots contain only
// counts and assignment references; no money is ever recorded in them.
func (s *Service) ProviderOwnPerformance(ctx context.Context, org, membership string) ([]PerformanceSnapshot, error) {
	return s.ListPerformance(ctx, selfScope(org, membership), membership)
}
