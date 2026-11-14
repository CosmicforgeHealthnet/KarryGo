// Package http serves the workforce tenant routes and the internal provider
// self-view routes.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

func TenantRoutes() []Route {
	return []Route{
		{"GET", "/workforce/profiles", "listWorkforceProfiles", "workforce.read", "WORKFORCE_ENABLED", "corporate", false, false},
		{"POST", "/workforce/profiles", "createWorkforceProfile", "workforce.manage", "WORKFORCE_ENABLED", "corporate", true, false},
		{"GET", "/workforce/profiles/:profileId", "getWorkforceProfile", "workforce.read", "WORKFORCE_ENABLED", "corporate", false, false},
		{"PATCH", "/workforce/profiles/:profileId", "updateWorkforceProfile", "workforce.manage", "WORKFORCE_ENABLED", "corporate", false, true},
		{"POST", "/workforce/profiles/:profileId/archive", "archiveWorkforceProfile", "workforce.manage", "WORKFORCE_ENABLED", "corporate", true, true},
		{"GET", "/shift-templates", "listShiftTemplates", "shifts.read", "SHIFTS_ENABLED", "corporate", false, false},
		{"POST", "/shift-templates", "createShiftTemplate", "shifts.manage", "SHIFTS_ENABLED", "corporate", true, false},
		{"GET", "/shift-templates/:templateId", "getShiftTemplate", "shifts.read", "SHIFTS_ENABLED", "corporate", false, false},
		{"PATCH", "/shift-templates/:templateId", "updateShiftTemplate", "shifts.manage", "SHIFTS_ENABLED", "corporate", false, true},
		{"POST", "/shift-templates/:templateId/generate", "generateShiftsFromTemplate", "shifts.manage", "SHIFTS_ENABLED", "corporate", true, false},
		{"GET", "/shifts", "listShifts", "shifts.read", "SHIFTS_ENABLED", "corporate", false, false},
		{"POST", "/shifts", "createShift", "shifts.manage", "SHIFTS_ENABLED", "corporate", true, false},
		{"GET", "/shifts/:shiftId", "getShift", "shifts.read", "SHIFTS_ENABLED", "corporate", false, false},
		{"PATCH", "/shifts/:shiftId", "updateShift", "shifts.manage", "SHIFTS_ENABLED", "corporate", false, true},
		{"POST", "/shifts/:shiftId/publish", "publishShift", "shifts.manage", "SHIFTS_ENABLED", "corporate", true, true},
		{"POST", "/shifts/:shiftId/cancel", "cancelShift", "shifts.manage", "SHIFTS_ENABLED", "corporate", true, true},
		{"POST", "/shifts/:shiftId/assignments", "assignShift", "shifts.assign", "SHIFTS_ENABLED", "corporate", true, false},
		{"GET", "/shift-assignments/:assignmentId", "getShiftAssignment", "shifts.read", "SHIFTS_ENABLED", "corporate", false, false},
		{"POST", "/shift-assignments/:assignmentId/check-in", "checkInShiftAssignment", "shifts.check_in", "SHIFTS_ENABLED", "corporate", true, true},
		{"POST", "/shift-assignments/:assignmentId/check-out", "checkOutShiftAssignment", "shifts.check_out", "SHIFTS_ENABLED", "corporate", true, true},
		{"POST", "/shift-assignments/:assignmentId/cancel", "cancelShiftAssignment", "shifts.assign", "SHIFTS_ENABLED", "corporate", true, true},
		{"GET", "/leave", "listLeave", "leave.read", "LEAVE_ENABLED", "corporate", false, false},
		{"POST", "/leave", "createLeave", "leave.manage", "LEAVE_ENABLED", "corporate", true, false},
		{"GET", "/leave/:leaveId", "getLeave", "leave.read", "LEAVE_ENABLED", "corporate", false, false},
		{"POST", "/leave/:leaveId/approve", "approveLeave", "leave.approve", "LEAVE_ENABLED", "corporate", true, true},
		{"POST", "/leave/:leaveId/reject", "rejectLeave", "leave.approve", "LEAVE_ENABLED", "corporate", true, true},
		{"POST", "/leave/:leaveId/cancel", "cancelLeave", "leave.manage", "LEAVE_ENABLED", "corporate", true, true},
		{"GET", "/qualifications", "listQualifications", "qualifications.read", "QUALIFICATIONS_ENABLED", "corporate", false, false},
		{"POST", "/qualifications", "createQualification", "qualifications.manage", "QUALIFICATIONS_ENABLED", "corporate", true, false},
		{"GET", "/qualifications/:qualificationId", "getQualification", "qualifications.read", "QUALIFICATIONS_ENABLED", "corporate", false, false},
		{"PATCH", "/qualifications/:qualificationId", "updateQualification", "qualifications.manage", "QUALIFICATIONS_ENABLED", "corporate", false, true},
		{"GET", "/providers/:membershipId/qualifications", "listProviderQualifications", "qualifications.read", "QUALIFICATIONS_ENABLED", "corporate", false, false},
		{"POST", "/providers/:membershipId/qualifications", "addProviderQualification", "qualifications.manage", "QUALIFICATIONS_ENABLED", "corporate", true, false},
		{"POST", "/provider-qualifications/:id/verify", "verifyProviderQualification", "qualifications.verify", "QUALIFICATIONS_ENABLED", "corporate", true, true},
		{"POST", "/provider-qualifications/:id/reject", "rejectProviderQualification", "qualifications.verify", "QUALIFICATIONS_ENABLED", "corporate", true, true},
		{"POST", "/provider-qualifications/:id/revoke", "revokeProviderQualification", "qualifications.verify", "QUALIFICATIONS_ENABLED", "corporate", true, true},
		{"GET", "/training/courses", "listTrainingCourses", "training.read", "TRAINING_ENABLED", "corporate", false, false},
		{"POST", "/training/courses", "createTrainingCourse", "training.manage", "TRAINING_ENABLED", "corporate", true, false},
		{"GET", "/training/courses/:courseId", "getTrainingCourse", "training.read", "TRAINING_ENABLED", "corporate", false, false},
		{"PATCH", "/training/courses/:courseId", "updateTrainingCourse", "training.manage", "TRAINING_ENABLED", "corporate", false, true},
		{"POST", "/training/courses/:courseId/complete", "completeTrainingCourse", "training.record_completion", "TRAINING_ENABLED", "corporate", true, false},
		{"GET", "/training/enrollments", "listTrainingEnrollments", "training.read", "TRAINING_ENABLED", "corporate", false, false},
		{"POST", "/training/enrollments", "createTrainingEnrollment", "training.manage", "TRAINING_ENABLED", "corporate", true, false},
		{"POST", "/training/enrollments/:enrollmentId/complete", "completeTrainingEnrollment", "training.record_completion", "TRAINING_ENABLED", "corporate", true, false},
		{"POST", "/training/enrollments/:enrollmentId/cancel", "cancelTrainingEnrollment", "training.manage", "TRAINING_ENABLED", "corporate", true, false},
		{"GET", "/performance/providers/:membershipId", "getProviderPerformance", "performance.read", "PERFORMANCE_SNAPSHOTS_ENABLED", "corporate", false, false},
	}
}

// UseCases is the workforce use-case surface the tenant handler needs.
// *usecases.Service satisfies it; the composition root passes the concrete
// service.
type UseCases interface {
	ListProfilesPage(ctx context.Context, t tenant.Context, f usecases.Page) ([]models.Profile, bool, error)
	GetProfile(ctx context.Context, t tenant.Context, id string) (models.Profile, error)
	CreateProfile(ctx context.Context, t tenant.Context, in usecases.CreateProfileInput) (models.Profile, error)
	UpdateProfile(ctx context.Context, t tenant.Context, id string, expected int64, role, supervisor, status string) (models.Profile, error)
	ListShiftsPage(ctx context.Context, t tenant.Context, f usecases.Page) ([]models.Shift, bool, error)
	GetShift(ctx context.Context, t tenant.Context, id string) (models.Shift, error)
	CreateShift(ctx context.Context, t tenant.Context, in usecases.CreateShiftInput) (models.Shift, error)
	UpdateShift(ctx context.Context, t tenant.Context, id string, expected int64, name string, start, end *time.Time, to string) (models.Shift, error)
	AssignShift(ctx context.Context, t tenant.Context, shiftID string, in usecases.AssignShiftInput) (models.ShiftAssignment, error)
	GetAssignment(ctx context.Context, t tenant.Context, id string) (models.ShiftAssignment, error)
	AssignmentAction(ctx context.Context, t tenant.Context, id, to, source string, expected int64) (models.ShiftAssignment, error)
	ListLeavePage(ctx context.Context, t tenant.Context, f usecases.Page) ([]models.Leave, bool, error)
	GetLeave(ctx context.Context, t tenant.Context, id string) (models.Leave, error)
	CreateLeave(ctx context.Context, t tenant.Context, in usecases.CreateLeaveInput) (models.Leave, error)
	LeaveAction(ctx context.Context, t tenant.Context, id, to string, expected int64) (models.Leave, error)
	ListQualifications(ctx context.Context, t tenant.Context) ([]models.Qualification, error)
	ListQualificationsPage(ctx context.Context, t tenant.Context, f usecases.Page) ([]models.Qualification, bool, error)
	GetQualification(ctx context.Context, t tenant.Context, id string) (models.Qualification, error)
	CreateQualification(ctx context.Context, t tenant.Context, v models.Qualification) (models.Qualification, error)
	UpdateQualification(ctx context.Context, t tenant.Context, id string, expected int64, p models.QualificationPatch) (models.Qualification, error)
	ListProviderQualifications(ctx context.Context, t tenant.Context, membership string) ([]models.ProviderQualification, error)
	AddProviderQualification(ctx context.Context, t tenant.Context, membership string, v models.ProviderQualification) (models.ProviderQualification, error)
	QualificationAction(ctx context.Context, t tenant.Context, id, to, reference string, expected int64) (models.ProviderQualification, error)
	ListCourses(ctx context.Context, t tenant.Context) ([]models.Course, error)
	ListCoursesPage(ctx context.Context, t tenant.Context, f usecases.Page) ([]models.Course, bool, error)
	GetCourse(ctx context.Context, t tenant.Context, id string) (models.Course, error)
	CreateCourse(ctx context.Context, t tenant.Context, v models.Course) (models.Course, error)
	UpdateCourse(ctx context.Context, t tenant.Context, id string, expected int64, p models.CoursePatch) (models.Course, error)
	CompleteCourse(ctx context.Context, t tenant.Context, courseID string, v models.Completion) (models.Completion, error)
	ListShiftTemplatesPage(ctx context.Context, t tenant.Context, f usecases.Page) ([]models.ShiftTemplate, bool, error)
	GetShiftTemplate(ctx context.Context, t tenant.Context, id string) (models.ShiftTemplate, error)
	CreateShiftTemplate(ctx context.Context, t tenant.Context, v models.ShiftTemplate) (models.ShiftTemplate, error)
	UpdateShiftTemplate(ctx context.Context, t tenant.Context, id string, expected int64, p usecases.TemplatePatch) (models.ShiftTemplate, error)
	GenerateShifts(ctx context.Context, t tenant.Context, id string, in usecases.GenerateInput) (models.GeneratedShifts, error)
	ListEnrollmentsPage(ctx context.Context, t tenant.Context, membershipID string, f usecases.Page) ([]models.Enrollment, bool, error)
	Enroll(ctx context.Context, t tenant.Context, in usecases.EnrollInput) (models.Enrollment, error)
	CompleteEnrollment(ctx context.Context, t tenant.Context, id string, c models.Completion) (models.Enrollment, error)
	CancelEnrollment(ctx context.Context, t tenant.Context, id string) (models.Enrollment, error)
	ListPerformance(ctx context.Context, t tenant.Context, membership string) ([]models.PerformanceSnapshot, error)
}

type Handler struct {
	s    UseCases
	a    *accesscontrol.Authorizer
	idem *idempotency.Store
}

func NewHandler(s UseCases, a *accesscontrol.Authorizer, i *idempotency.Store) *Handler {
	return &Handler{s: s, a: a, idem: i}
}
func (h *Handler) TenantRoutes() []Route { return TenantRoutes() }
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	for _, r := range TenantRoutes() {
		r := r
		g.Handle(r.Method, r.Path, func(c *gin.Context) { h.handle(c, r) })
	}
}
func (h *Handler) gate(c *gin.Context, p string) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok || h.a == nil {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return t, false
	}
	rv := int64(1)
	if v, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		rv, _ = v.(int64)
	}
	scoped, e := h.a.RequireScoped(c, t, rv, p)
	if e == nil {
		// Narrowed to the branches where the permission is granted;
		// a caller-chosen ?branchId never widens it.
		t = scoped
		middleware.SetTenant(c, scoped)
	}
	if e != nil {
		respond.Error(c, e)
		return t, false
	}
	return t, true
}
func bindJSON(c *gin.Context, v any) bool {
	if e := c.ShouldBindJSON(v); e != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", e))
		return false
	}
	return true
}
func expected(c *gin.Context, v int64) int64 {
	if v > 0 {
		return v
	}
	n, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`), 10, 64)
	return n
}

// page reads the list filter. Malformed, negative or oversized limit/offset
// values are a validation error rather than a SQL failure.
//
// FLEET-6f: without offset the list is served by keyset and the response
// carries meta.nextCursor while more rows exist; ?cursor= continues it. An
// explicit offset keeps the backward-compatible offset page. Sending both is
// refused, because the two describe different positions.
func page(c *gin.Context) (usecases.Page, error) {
	p, e := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if e != nil {
		return usecases.Page{}, e
	}
	out := usecases.Page{BranchID: c.Query("branchId"), Status: strings.ToUpper(c.Query("status")), Limit: p.Limit, Offset: p.Offset}
	_, hasOffset := c.GetQuery("offset")
	rawCursor := c.Query("cursor")
	if hasOffset && rawCursor != "" {
		return usecases.Page{}, apperrors.Validation("Use either cursor or offset, not both.",
			[]apperrors.FieldViolation{{Field: "cursor", Message: "Cannot be combined with offset."}})
	}
	if hasOffset {
		return out, nil
	}
	position, hasCursor, err := cursor.Decode(rawCursor)
	if err != nil {
		return usecases.Page{}, apperrors.Validation("The cursor is not valid.",
			[]apperrors.FieldViolation{{Field: "cursor", Message: "Malformed."}})
	}
	out.Keyset = true
	if hasCursor {
		after := position.SortTime()
		out.After, out.AfterID = &after, position.LastID
	}
	return out, nil
}

// pagedRequested reports whether an unpaginated catalogue list (qualifications,
// training courses) was asked for a page. Without limit or cursor those lists
// keep returning every row, as they always have.
func pagedRequested(c *gin.Context) bool {
	return c.Query("limit") != "" || c.Query("cursor") != ""
}

// listResult is a list page with its continuation cursor.
type listResult struct {
	data any
	next string
}

// keysetPage wraps rows with the cursor that continues after the last row.
func keysetPage[T any](rows []T, more bool, err error, key func(T) (time.Time, string)) (any, error) {
	if err != nil {
		return nil, err
	}
	next := ""
	if more && len(rows) > 0 {
		sortedAt, id := key(rows[len(rows)-1])
		next = cursor.EncodeTime(sortedAt, id)
	}
	return listResult{data: rows, next: next}, nil
}

func (h *Handler) command(c *gin.Context, t tenant.Context, r Route, body any, run func() (any, error)) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	if h.idem == nil {
		respond.Error(c, apperrors.Internal("Idempotency store is unavailable.", nil))
		return
	}
	raw, readErr := c.GetRawData()
	if readErr != nil {
		respond.Error(c, apperrors.BadRequest("Request body could not be read.", readErr))
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if len(raw) == 0 || !json.Valid(raw) {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	ep := r.Method + " " + r.Path
	rec, e := h.idem.Begin(c, t.OrganizationID, ep, key, idempotency.Fingerprint(r.Method, ep, raw))
	if e != nil {
		respond.Error(c, idempotency.AsAppError(e))
		return
	}
	if rec != nil {
		c.Data(rec.ResponseStatus, "application/json; charset=utf-8", rec.ResponseBody)
		return
	}
	v, e := run()
	status := http.StatusOK
	if r.Method == "POST" && strings.Contains(r.OperationID, "create") {
		status = http.StatusCreated
	}
	var payload []byte
	if e != nil {
		ae := apperrors.From(e)
		payload, _ = json.Marshal(gin.H{"error": respond.ErrorBody{Code: ae.Code, Message: ae.Message, Details: []apperrors.FieldViolation{}, RequestID: httpx.GetRequestID(c)}})
		status = ae.Status
	} else {
		payload, _ = json.Marshal(gin.H{"data": v, "meta": gin.H{"requestId": httpx.GetRequestID(c)}})
	}
	if x := h.idem.Complete(c, t.OrganizationID, ep, key, status, payload); x != nil {
		respond.Error(c, x)
		return
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}
func output(c *gin.Context, v any, e error) {
	if e != nil {
		respond.Error(c, e)
		return
	}
	if page, ok := v.(listResult); ok {
		respond.List(c, page.data, page.next)
		return
	}
	respond.OK(c, v)
}

type versionBody struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
	Source          string `json:"source"`
	Reference       string `json:"reference"`
}
type profilePatch struct {
	ExpectedVersion        int64  `json:"expectedVersion"`
	Role                   string `json:"role"`
	SupervisorMembershipID string `json:"supervisorMembershipId"`
	Status                 string `json:"status"`
}
type shiftPatch struct {
	ExpectedVersion int64      `json:"expectedVersion"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	StartAt         *time.Time `json:"startAt"`
	EndAt           *time.Time `json:"endAt"`
}

func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := h.gate(c, r.Permission)
	if !ok {
		return
	}
	id := c.Param("profileId")
	if id == "" {
		id = c.Param("shiftId")
	}
	if id == "" {
		id = c.Param("assignmentId")
	}
	if id == "" {
		id = c.Param("leaveId")
	}
	if id == "" {
		id = c.Param("qualificationId")
	}
	if id == "" {
		id = c.Param("courseId")
	}
	if id == "" {
		id = c.Param("templateId")
	}
	if id == "" {
		id = c.Param("enrollmentId")
	}
	if id == "" {
		id = c.Param("id")
	}
	run := func() (any, error) {
		switch r.OperationID {
		case "listWorkforceProfiles":
			p, e := page(c)
			if e != nil {
				return nil, e
			}
			rows, more, e := h.s.ListProfilesPage(c, t, p)
			return keysetPage(rows, more, e, func(v models.Profile) (time.Time, string) { return v.CreatedAt, v.ID })
		case "getWorkforceProfile":
			return h.s.GetProfile(c, t, id)
		case "createWorkforceProfile":
			var b usecases.CreateProfileInput
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateProfile(c, t, b)
		case "updateWorkforceProfile":
			var b profilePatch
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateProfile(c, t, id, expected(c, b.ExpectedVersion), b.Role, b.SupervisorMembershipID, b.Status)
		case "archiveWorkforceProfile":
			var b versionBody
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateProfile(c, t, id, expected(c, b.ExpectedVersion), "", "", "ARCHIVED")
		case "listShifts":
			p, e := page(c)
			if e != nil {
				return nil, e
			}
			rows, more, e := h.s.ListShiftsPage(c, t, p)
			return keysetPage(rows, more, e, func(v models.Shift) (time.Time, string) { return v.StartAt, v.ID })
		case "getShift":
			return h.s.GetShift(c, t, id)
		case "createShift":
			var b usecases.CreateShiftInput
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateShift(c, t, b)
		case "updateShift":
			var b shiftPatch
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateShift(c, t, id, expected(c, b.ExpectedVersion), b.Name, b.StartAt, b.EndAt, b.Status)
		case "publishShift", "cancelShift":
			var b versionBody
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			to := "PUBLISHED"
			if r.OperationID == "cancelShift" {
				to = "CANCELLED"
			}
			return h.s.UpdateShift(c, t, id, expected(c, b.ExpectedVersion), "", nil, nil, to)
		case "assignShift":
			var b usecases.AssignShiftInput
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.AssignShift(c, t, id, b)
		case "getShiftAssignment":
			return h.s.GetAssignment(c, t, id)
		case "checkInShiftAssignment", "checkOutShiftAssignment", "cancelShiftAssignment":
			var b versionBody
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			to := "CHECKED_IN"
			if r.OperationID == "checkOutShiftAssignment" {
				to = "CHECKED_OUT"
			}
			if r.OperationID == "cancelShiftAssignment" {
				to = "CANCELLED"
			}
			return h.s.AssignmentAction(c, t, id, to, b.Source, expected(c, b.ExpectedVersion))
		case "listLeave":
			p, e := page(c)
			if e != nil {
				return nil, e
			}
			rows, more, e := h.s.ListLeavePage(c, t, p)
			return keysetPage(rows, more, e, func(v models.Leave) (time.Time, string) { return v.StartAt, v.ID })
		case "getLeave":
			return h.s.GetLeave(c, t, id)
		case "createLeave":
			var b usecases.CreateLeaveInput
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateLeave(c, t, b)
		case "approveLeave", "rejectLeave", "cancelLeave":
			var b versionBody
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			to := map[string]string{"approveLeave": "APPROVED", "rejectLeave": "REJECTED", "cancelLeave": "CANCELLED"}[r.OperationID]
			return h.s.LeaveAction(c, t, id, to, expected(c, b.ExpectedVersion))
		case "listQualifications":
			if !pagedRequested(c) {
				return h.s.ListQualifications(c, t)
			}
			p, e := page(c)
			if e != nil {
				return nil, e
			}
			rows, more, e := h.s.ListQualificationsPage(c, t, p)
			return keysetPage(rows, more, e, func(v models.Qualification) (time.Time, string) { return v.CreatedAt, v.ID })
		case "getQualification":
			return h.s.GetQualification(c, t, id)
		case "createQualification":
			var b models.Qualification
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateQualification(c, t, b)
		case "updateQualification":
			var b models.QualificationPatch
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateQualification(c, t, id, expected(c, b.Version), b)
		case "listProviderQualifications":
			return h.s.ListProviderQualifications(c, t, c.Param("membershipId"))
		case "addProviderQualification":
			var b models.ProviderQualification
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.AddProviderQualification(c, t, c.Param("membershipId"), b)
		case "verifyProviderQualification", "rejectProviderQualification", "revokeProviderQualification":
			var b versionBody
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			to := map[string]string{"verifyProviderQualification": "VERIFIED", "rejectProviderQualification": "REJECTED", "revokeProviderQualification": "REVOKED"}[r.OperationID]
			return h.s.QualificationAction(c, t, id, to, b.Reference, expected(c, b.ExpectedVersion))
		case "listTrainingCourses":
			if !pagedRequested(c) {
				return h.s.ListCourses(c, t)
			}
			p, e := page(c)
			if e != nil {
				return nil, e
			}
			rows, more, e := h.s.ListCoursesPage(c, t, p)
			return keysetPage(rows, more, e, func(v models.Course) (time.Time, string) { return v.CreatedAt, v.ID })
		case "getTrainingCourse":
			return h.s.GetCourse(c, t, id)
		case "createTrainingCourse":
			var b models.Course
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateCourse(c, t, b)
		case "updateTrainingCourse":
			var b models.CoursePatch
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateCourse(c, t, id, expected(c, b.Version), b)
		case "completeTrainingCourse":
			var b models.Completion
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CompleteCourse(c, t, id, b)
		case "listShiftTemplates":
			p, e := page(c)
			if e != nil {
				return nil, e
			}
			rows, more, e := h.s.ListShiftTemplatesPage(c, t, p)
			return keysetPage(rows, more, e, func(v models.ShiftTemplate) (time.Time, string) { return v.CreatedAt, v.ID })
		case "getShiftTemplate":
			return h.s.GetShiftTemplate(c, t, id)
		case "createShiftTemplate":
			var b models.ShiftTemplate
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateShiftTemplate(c, t, b)
		case "updateShiftTemplate":
			var b usecases.TemplatePatch
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateShiftTemplate(c, t, id, expected(c, b.ExpectedVersion), b)
		case "generateShiftsFromTemplate":
			var b usecases.GenerateInput
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.GenerateShifts(c, t, id, b)
		case "listTrainingEnrollments":
			p, e := page(c)
			if e != nil {
				return nil, e
			}
			rows, more, e := h.s.ListEnrollmentsPage(c, t, c.Query("membershipId"), p)
			return keysetPage(rows, more, e, func(v models.Enrollment) (time.Time, string) { return v.AssignedAt, v.ID })
		case "createTrainingEnrollment":
			var b usecases.EnrollInput
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.Enroll(c, t, b)
		case "completeTrainingEnrollment":
			var b models.Completion
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CompleteEnrollment(c, t, id, b)
		case "cancelTrainingEnrollment":
			return h.s.CancelEnrollment(c, t, id)
		case "getProviderPerformance":
			return h.s.ListPerformance(c, t, c.Param("membershipId"))
		}
		return nil, apperrors.NotFound("Operation not found.", nil)
	}
	if r.IdempotencyRequired {
		h.command(c, t, r, nil, run)
		return
	}
	v, e := run()
	if e != nil && e.Error() == "bound" {
		return
	}
	output(c, v, e)
}
