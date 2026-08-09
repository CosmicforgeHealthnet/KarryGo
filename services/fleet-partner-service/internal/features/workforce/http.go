package workforce

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
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
		{"GET", "/performance/providers/:membershipId", "getProviderPerformance", "performance.read", "PERFORMANCE_SNAPSHOTS_ENABLED", "corporate", false, false},
	}
}

type Handler struct {
	s    *Service
	a    *accesscontrol.Authorizer
	idem *idempotency.Store
}

func NewHandler(s *Service, a *accesscontrol.Authorizer, i *idempotency.Store) *Handler {
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
	if e := h.a.Require(c, t.OrganizationID, t.OrganizationUserID, rv, p, accesscontrol.Target{BranchID: c.Query("branchId")}); e != nil {
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
func page(c *gin.Context) Page {
	l, _ := strconv.Atoi(c.Query("limit"))
	o, _ := strconv.Atoi(c.Query("offset"))
	return Page{BranchID: c.Query("branchId"), Status: strings.ToUpper(c.Query("status")), Limit: l, Offset: o}
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
	respond.OK(c, v)
}

type versionBody struct {
	ExpectedVersion           int64 `json:"expectedVersion"`
	Reason, Source, Reference string
}
type profilePatch struct {
	ExpectedVersion                      int64
	Role, SupervisorMembershipID, Status string
}
type shiftPatch struct {
	ExpectedVersion int64
	Name, Status    string
	StartAt, EndAt  *time.Time
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
		id = c.Param("id")
	}
	run := func() (any, error) {
		switch r.OperationID {
		case "listWorkforceProfiles":
			return h.s.ListProfiles(c, t, page(c))
		case "getWorkforceProfile":
			return h.s.GetProfile(c, t, id)
		case "createWorkforceProfile":
			var b CreateProfileInput
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
			return h.s.ListShifts(c, t, page(c))
		case "getShift":
			return h.s.GetShift(c, t, id)
		case "createShift":
			var b CreateShiftInput
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
			var b AssignShiftInput
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
			return h.s.ListLeave(c, t, page(c))
		case "getLeave":
			return h.s.GetLeave(c, t, id)
		case "createLeave":
			var b CreateLeaveInput
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
			return h.s.ListQualifications(c, t)
		case "getQualification":
			return h.s.GetQualification(c, t, id)
		case "createQualification":
			var b Qualification
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateQualification(c, t, b)
		case "updateQualification":
			var b Qualification
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateQualification(c, t, id, expected(c, b.Version), b)
		case "listProviderQualifications":
			return h.s.ListProviderQualifications(c, t, c.Param("membershipId"))
		case "addProviderQualification":
			var b ProviderQualification
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
			return h.s.ListCourses(c, t)
		case "getTrainingCourse":
			return h.s.GetCourse(c, t, id)
		case "createTrainingCourse":
			var b Course
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CreateCourse(c, t, b)
		case "updateTrainingCourse":
			var b Course
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.UpdateCourse(c, t, id, expected(c, b.Version), b)
		case "completeTrainingCourse":
			var b Completion
			if !bindJSON(c, &b) {
				return nil, errors.New("bound")
			}
			return h.s.CompleteCourse(c, t, id, b)
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
