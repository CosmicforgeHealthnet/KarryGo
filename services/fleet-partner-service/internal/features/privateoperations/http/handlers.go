// Package http exposes corporate and HMAC-authenticated provider routes for
// Section 15 private assignments. Provider responses are never mounted on the
// corporate session surface.
package http

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

const (
	PermissionRead            = "private_assignments.read"
	PermissionManage          = "private_assignments.manage"
	PermissionRespond         = "private_assignments.respond"
	PermissionExecutionRead   = "private_execution.read"
	PermissionExecutionReplay = "private_execution.replay"
)

var callerExecutionService = map[string]string{
	"driver-dispatch-delivery-service": "DISPATCH",
	"driver-hauling-service":           "HAULING",
}

type Handler struct {
	service     *usecases.Service
	authorizer  *accesscontrol.Authorizer
	idempotency *idempotency.Store
}

func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer, store *idempotency.Store) *Handler {
	return &Handler{service: service, authorizer: authorizer, idempotency: store}
}

func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	if h.service == nil {
		return
	}
	handlers := map[string]gin.HandlerFunc{
		"createPrivateAssignment": h.create, "listPrivateAssignments": h.list,
		"getPrivateAssignment": h.get, "cancelPrivateAssignment": h.cancel,
		"reassignPrivateAssignment": h.reassign, "getPrivateExecution": h.getExecution,
		"replayPrivateExecution": h.replayExecution, "replayPrivateReconciliation": h.replayReconciliation,
	}
	for _, route := range h.TenantRoutes() {
		group.Handle(route.Method, route.Path, handlers[route.OperationID])
	}
}

func (h *Handler) RegisterInternalRoutes(group *gin.RouterGroup) {
	if h.service == nil {
		return
	}
	handlers := map[string]gin.HandlerFunc{
		"getPrivateAssignmentForProvider": h.providerGet,
		"acceptPrivateAssignment":         h.accept,
		"declinePrivateAssignment":        h.decline,
	}
	for _, route := range h.InternalRoutes() {
		group.Handle(route.Method, route.Path, handlers[route.OperationID])
	}
}

func tenantContext(c *gin.Context) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
	}
	return t, ok
}

func roleVersion(c *gin.Context) int64 {
	value, _ := c.Get(middleware.TenantRoleVersionKey)
	version, _ := value.(int64)
	if version < 1 {
		return 1
	}
	return version
}

func (h *Handler) authorize(c *gin.Context, t tenant.Context, permission string) bool {
	if h.authorizer == nil {
		respond.Error(c, apperrors.Forbidden("Permission denied.", nil))
		return false
	}
	err := h.authorizer.Require(c.Request.Context(), t.OrganizationID, t.OrganizationUserID, roleVersion(c), permission, accesscontrol.Target{BranchID: c.Query("branchId")})
	if err != nil {
		respond.Error(c, err)
		return false
	}
	return true
}

func bindJSON(c *gin.Context, value any) bool {
	if err := c.ShouldBindJSON(value); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", err))
		return false
	}
	return true
}

func expectedVersion(c *gin.Context, body int64) int64 {
	if body > 0 {
		return body
	}
	value, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`), 10, 64)
	return value
}

type responseMeta struct {
	RequestID string `json:"requestId"`
}
type successResponse struct {
	Data any          `json:"data"`
	Meta responseMeta `json:"meta"`
}
type errorResponse struct {
	Error respond.ErrorBody `json:"error"`
}

func (h *Handler) command(c *gin.Context, organizationID, endpoint string, body any, run func() (int, any, error)) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	raw, _ := json.Marshal(body)
	fingerprint := idempotency.Fingerprint(c.Request.Method, endpoint, raw)
	record, err := h.idempotency.Begin(c.Request.Context(), organizationID, endpoint, key, fingerprint)
	if err != nil {
		respond.Error(c, idempotency.AsAppError(err))
		return
	}
	if record != nil {
		c.Data(record.ResponseStatus, "application/json; charset=utf-8", record.ResponseBody)
		return
	}
	status, data, runErr := run()
	requestID := httpx.GetRequestID(c)
	var payload []byte
	if runErr != nil {
		appErr := apperrors.From(runErr)
		details := appErr.Fields
		if details == nil {
			details = []apperrors.FieldViolation{}
		}
		payload, _ = json.Marshal(errorResponse{respond.ErrorBody{Code: appErr.Code, Message: appErr.Message, Details: details, RequestID: requestID}})
		status = appErr.Status
	} else {
		payload, _ = json.Marshal(successResponse{Data: data, Meta: responseMeta{RequestID: requestID}})
	}
	if err = h.idempotency.Complete(c.Request.Context(), organizationID, endpoint, key, status, payload); err != nil {
		respond.Error(c, apperrors.Internal("Failed to persist idempotent response.", err))
		return
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}

type createRequest struct {
	BranchID                string     `json:"branchId"`
	WorkOrigin              string     `json:"workOrigin"`
	WorkID                  string     `json:"workId"`
	ProviderMembershipID    string     `json:"providerMembershipId"`
	ProviderID              string     `json:"providerId"`
	VehicleOrTruckReference string     `json:"vehicleOrTruckReference"`
	ExecutionService        string     `json:"executionService"`
	ResponseDeadline        *time.Time `json:"responseDeadline"`
}

func (h *Handler) create(c *gin.Context) {
	t, ok := tenantContext(c)
	if !ok || !h.authorize(c, t, PermissionManage) {
		return
	}
	var body createRequest
	if !bindJSON(c, &body) {
		return
	}
	h.command(c, t.OrganizationID, "POST /private-assignments", body, func() (int, any, error) {
		assignment, err := h.service.Create(c.Request.Context(), t, usecases.CreateInput{
			BranchID: body.BranchID, WorkOrigin: body.WorkOrigin, WorkID: body.WorkID,
			ProviderMembershipID: body.ProviderMembershipID, ProviderID: body.ProviderID,
			VehicleOrTruckReference: body.VehicleOrTruckReference,
			ExecutionService:        body.ExecutionService, ResponseDeadline: body.ResponseDeadline,
		})
		return http.StatusCreated, assignment, err
	})
}

func (h *Handler) list(c *gin.Context) {
	t, ok := tenantContext(c)
	if !ok || !h.authorize(c, t, PermissionRead) {
		return
	}
	items, err := h.service.List(c.Request.Context(), t, c.Query("status"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, items)
}

func (h *Handler) get(c *gin.Context) {
	t, ok := tenantContext(c)
	if !ok || !h.authorize(c, t, PermissionRead) {
		return
	}
	item, err := h.service.Get(c.Request.Context(), t, c.Param("assignmentId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, item)
}

type versionRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

func (h *Handler) cancel(c *gin.Context) {
	t, ok := tenantContext(c)
	if !ok || !h.authorize(c, t, PermissionManage) {
		return
	}
	var body versionRequest
	if !bindJSON(c, &body) {
		return
	}
	body.ExpectedVersion = expectedVersion(c, body.ExpectedVersion)
	h.command(c, t.OrganizationID, "POST /private-assignments/:assignmentId/cancel", body, func() (int, any, error) {
		item, err := h.service.Cancel(c.Request.Context(), t, c.Param("assignmentId"), body.ExpectedVersion)
		return http.StatusOK, item, err
	})
}

type reassignRequest struct {
	ProviderMembershipID    string     `json:"providerMembershipId"`
	ProviderID              string     `json:"providerId"`
	VehicleOrTruckReference string     `json:"vehicleOrTruckReference"`
	ExecutionService        string     `json:"executionService"`
	ResponseDeadline        *time.Time `json:"responseDeadline"`
	ExpectedVersion         int64      `json:"expectedVersion"`
}

func (h *Handler) reassign(c *gin.Context) {
	t, ok := tenantContext(c)
	if !ok || !h.authorize(c, t, PermissionManage) {
		return
	}
	var body reassignRequest
	if !bindJSON(c, &body) {
		return
	}
	body.ExpectedVersion = expectedVersion(c, body.ExpectedVersion)
	h.command(c, t.OrganizationID, "POST /private-assignments/:assignmentId/reassign", body, func() (int, any, error) {
		item, err := h.service.Reassign(c.Request.Context(), t, c.Param("assignmentId"), usecases.ReassignInput{
			ProviderMembershipID: body.ProviderMembershipID, ProviderID: body.ProviderID,
			VehicleOrTruckReference: body.VehicleOrTruckReference, ExecutionService: body.ExecutionService,
			ExpectedVersion: body.ExpectedVersion, ResponseDeadline: body.ResponseDeadline,
		})
		return http.StatusCreated, item, err
	})
}

func (h *Handler) getExecution(c *gin.Context) {
	t, ok := tenantContext(c)
	if !ok || !h.authorize(c, t, PermissionExecutionRead) {
		return
	}
	item, err := h.service.GetExecution(c.Request.Context(), t, c.Param("executionId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, item)
}

func (h *Handler) replayExecution(c *gin.Context) {
	t, ok := tenantContext(c)
	if !ok || !h.authorize(c, t, PermissionExecutionReplay) {
		return
	}
	body := struct{}{}
	h.command(c, t.OrganizationID, "POST /private-executions/:executionId/replay", body,
		func() (int, any, error) {
			err := h.service.ReplayExecution(c.Request.Context(), t, c.Param("executionId"))
			return http.StatusAccepted, gin.H{"executionId": c.Param("executionId"), "status": "PENDING"}, err
		})
}

func (h *Handler) replayReconciliation(c *gin.Context) {
	t, ok := tenantContext(c)
	if !ok || !h.authorize(c, t, PermissionExecutionReplay) {
		return
	}
	body := struct{}{}
	h.command(c, t.OrganizationID, "POST /private-execution-reconciliations/:caseId/replay", body,
		func() (int, any, error) {
			err := h.service.ReplayReconciliation(c.Request.Context(), t, c.Param("caseId"))
			return http.StatusAccepted, gin.H{"caseId": c.Param("caseId"), "status": "PENDING"}, err
		})
}

type providerView struct {
	AssignmentID            string    `json:"assignmentId"`
	RequestID               string    `json:"requestId"`
	WorkOrigin              string    `json:"workOrigin"`
	ProviderID              string    `json:"providerId"`
	VehicleOrTruckReference string    `json:"vehicleOrTruckReference"`
	ExecutionService        string    `json:"executionService"`
	Status                  string    `json:"status"`
	ResponseDeadline        time.Time `json:"responseDeadline"`
	Version                 int64     `json:"version"`
	AllowedActions          []string  `json:"allowedActions"`
}

func providerDTO(item models.Assignment) providerView {
	actions := []string{}
	if item.Status == models.StatusPending {
		actions = []string{"ACCEPT", "DECLINE"}
	}
	requestID := item.PrivateOrderID
	if requestID == "" {
		requestID = item.ShipmentID
	}
	if requestID == "" {
		requestID = item.RouteStopID
	}
	return providerView{item.ID, requestID, item.WorkOrigin, item.ProviderID, item.VehicleOrTruckReference, item.ExecutionService, item.Status, item.ResponseDeadline, item.Version, actions}
}

func internalScope(c *gin.Context) (string, string, bool) {
	callerRaw, _ := c.Get("service_name")
	caller, _ := callerRaw.(string)
	service, ok := callerExecutionService[caller]
	providerID := strings.TrimSpace(c.Query("providerId"))
	if !ok || providerID == "" {
		respond.Error(c, apperrors.Forbidden("Authenticated provider scope is required.", nil))
		return "", "", false
	}
	return service, providerID, true
}

func (h *Handler) providerGet(c *gin.Context) {
	service, providerID, ok := internalScope(c)
	if !ok {
		return
	}
	item, err := h.service.GetProviderAssignment(c.Request.Context(), c.Param("assignmentId"), providerID, service)
	if err != nil {
		respond.Error(c, apperrors.NotFound("Private assignment not found.", err))
		return
	}
	respond.OK(c, providerDTO(item))
}

type providerResponseRequest struct {
	OrganizationID  string `json:"organizationId"`
	ProviderID      string `json:"providerId"`
	ReasonCode      string `json:"reasonCode"`
	Note            string `json:"note"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func responseFingerprint(assignmentID, providerID, decision, reason, note string, version int64) string {
	raw := strings.Join([]string{assignmentID, providerID, decision, reason, note, strconv.FormatInt(version, 10)}, "\x00")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (h *Handler) respondProvider(c *gin.Context, decision string) {
	callerRaw, _ := c.Get("service_name")
	caller, _ := callerRaw.(string)
	service, callerOK := callerExecutionService[caller]
	var body providerResponseRequest
	if !callerOK || !bindJSON(c, &body) {
		return
	}
	body.ExpectedVersion = expectedVersion(c, body.ExpectedVersion)
	if body.ProviderID == "" {
		respond.Error(c, apperrors.BadRequest("providerId is required.", nil))
		return
	}
	assignment, err := h.service.GetProviderAssignment(c.Request.Context(), c.Param("assignmentId"), body.ProviderID, service)
	if err != nil {
		respond.Error(c, apperrors.NotFound("Private assignment not found.", err))
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	fingerprint := responseFingerprint(assignment.ID, body.ProviderID, decision, body.ReasonCode, body.Note, body.ExpectedVersion)
	item, err := h.service.Respond(c.Request.Context(), assignment.OrganizationID, assignment.ID, usecases.RespondInput{
		ProviderID: body.ProviderID, Decision: decision, ReasonCode: body.ReasonCode,
		SafeNote: body.Note, IdempotencyKey: key, RequestFingerprint: fingerprint,
		ExpectedVersion: body.ExpectedVersion,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, providerDTO(item))
}

func (h *Handler) accept(c *gin.Context)  { h.respondProvider(c, "ACCEPT") }
func (h *Handler) decline(c *gin.Context) { h.respondProvider(c, "DECLINE") }
