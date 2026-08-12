// Package http exposes the corporate approval API.
//
// Requesting, deciding, cancelling and executing are four separate
// permissions. That split is the whole point of an approval control: someone
// who may ask for a payout must not thereby be able to approve it, and someone
// who may approve must not be able to quietly cancel the record that they did.
package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the routes.
const (
	PermissionRead    = "approvals.requests.read"
	PermissionCreate  = "approvals.requests.create"
	PermissionDecide  = "approvals.requests.decide"
	PermissionCancel  = "approvals.requests.cancel"
	PermissionExecute = "approvals.requests.execute"
)

// Handler serves the corporate approval routes.
type Handler struct {
	service    *usecases.Service
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// RegisterTenantRoutes mounts routes that require resolved organization scope.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.POST("/approvals/requests", h.request)
	group.GET("/approvals/requests", h.list)
	group.GET("/approvals/requests/:approvalId", h.get)
	group.GET("/approvals/requests/:approvalId/decisions", h.decisions)
	group.GET("/approvals/requests/:approvalId/history", h.history)
	group.POST("/approvals/requests/:approvalId/approve", h.approve)
	group.POST("/approvals/requests/:approvalId/reject", h.reject)
	group.POST("/approvals/requests/:approvalId/cancel", h.cancel)
	group.POST("/approvals/requests/:approvalId/execute", h.execute)
}

func requireTenant(c *gin.Context) (tenant.Context, bool) {
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

func (h *Handler) authorize(c *gin.Context, tenantCtx tenant.Context, permission string) bool {
	if h.authorizer == nil {
		respond.Error(c, apperrors.Forbidden("You do not have permission to perform this action.", nil))
		return false
	}
	if err := h.authorizer.Require(c.Request.Context(), tenantCtx.OrganizationID,
		tenantCtx.OrganizationUserID, roleVersionFrom(c), permission,
		accesscontrol.Target{}); err != nil {
		respond.Error(c, err)
		return false
	}
	return true
}

func roleVersionFrom(c *gin.Context) int64 {
	if value, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if version, ok := value.(int64); ok {
			return version
		}
	}
	return 1
}

func actorFrom(tenantCtx tenant.Context, branchID string) usecases.Actor {
	return usecases.Actor{
		OrganizationID:     tenantCtx.OrganizationID,
		OrganizationUserID: tenantCtx.OrganizationUserID,
		BranchID:           branchID,
		ActorType:          models.ActorOrganizationUser,
		ActorID:            tenantCtx.OrganizationUserID,
	}
}

// RequestView is the allowlisted response shape.
//
// The payload snapshot is deliberately absent. A reader who needs to know what
// was approved gets the fingerprint, which is enough to prove that what is
// being executed is the same thing, without republishing operational detail to
// every holder of the read permission.
type RequestView struct {
	ID                     string `json:"id"`
	ResourceType           string `json:"resourceType"`
	ResourceID             string `json:"resourceId"`
	Action                 string `json:"action"`
	Status                 string `json:"status"`
	BranchID               string `json:"branchId,omitempty"`
	RequesterID            string `json:"requesterId"`
	RequestFingerprint     string `json:"requestFingerprint"`
	PayloadVersion         int    `json:"payloadVersion"`
	ExpectedSubjectVersion int64  `json:"expectedSubjectVersion"`
	MinimumApprovals       int    `json:"minimumApprovals"`
	SelfApprovalAllowed    bool   `json:"selfApprovalAllowed"`
	SeparationOfDuties     bool   `json:"separationOfDuties"`
	PolicyVersion          int64  `json:"policyVersion"`
	EligibilityDecisionID  string `json:"eligibilityDecisionId,omitempty"`
	ExpiresAt              string `json:"expiresAt"`
	ExecutedAt             string `json:"executedAt,omitempty"`
	ExecutionReference     string `json:"executionReference,omitempty"`
	InvalidationReason     string `json:"invalidationReason,omitempty"`
	Version                int64  `json:"version"`
	CreatedAt              string `json:"createdAt"`
}

const timeFormat = "2006-01-02T15:04:05Z07:00"

func newRequestView(request *models.Request) RequestView {
	view := RequestView{
		ID:                     request.ID,
		ResourceType:           request.ResourceType,
		ResourceID:             request.ResourceID,
		Action:                 request.Action,
		Status:                 request.Status,
		RequesterID:            request.RequesterID,
		RequestFingerprint:     request.RequestFingerprint,
		PayloadVersion:         request.PayloadVersion,
		ExpectedSubjectVersion: request.ExpectedSubjectVersion,
		MinimumApprovals:       request.MinimumApprovals,
		SelfApprovalAllowed:    request.SelfApprovalAllowed,
		SeparationOfDuties:     request.SeparationOfDuties,
		PolicyVersion:          request.PolicyVersion,
		InvalidationReason:     request.InvalidationReason,
		ExpiresAt:              request.ExpiresAt.UTC().Format(timeFormat),
		Version:                request.Version,
		CreatedAt:              request.CreatedAt.UTC().Format(timeFormat),
	}
	if request.BranchID != nil {
		view.BranchID = *request.BranchID
	}
	if request.EligibilityDecisionID != nil {
		view.EligibilityDecisionID = *request.EligibilityDecisionID
	}
	if request.ExecutedAt != nil {
		view.ExecutedAt = request.ExecutedAt.UTC().Format(timeFormat)
	}
	if request.ExecutionReference != nil {
		view.ExecutionReference = *request.ExecutionReference
	}
	return view
}

// requestBody is the corporate approval request body.
type requestBody struct {
	ResourceType           string         `json:"resourceType"`
	ResourceID             string         `json:"resourceId"`
	Action                 string         `json:"action"`
	OperationType          string         `json:"operationType"`
	BranchID               string         `json:"branchId"`
	Payload                map[string]any `json:"payload"`
	ExpectedSubjectVersion int64          `json:"expectedSubjectVersion"`
	EligibilityDecisionID  string         `json:"eligibilityDecisionId"`
}

// request raises an approval request.
func (h *Handler) request(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionCreate) {
		return
	}

	var body requestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if body.BranchID != "" && !tenantCtx.BranchPermitted(body.BranchID) {
		respond.Error(c, apperrors.NotFound("Branch not found.", nil))
		return
	}
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if idempotencyKey == "" {
		respond.Error(c, apperrors.BadRequest("An Idempotency-Key header is required.", nil))
		return
	}

	result, err := h.service.Request(c.Request.Context(), usecases.RequestInput{
		Actor:                  actorFrom(tenantCtx, body.BranchID),
		ResourceType:           body.ResourceType,
		ResourceID:             body.ResourceID,
		Action:                 body.Action,
		OperationType:          body.OperationType,
		Payload:                body.Payload,
		ExpectedSubjectVersion: body.ExpectedSubjectVersion,
		EligibilityDecisionID:  body.EligibilityDecisionID,
		IdempotencyKey:         idempotencyKey,
		CorrelationID:          middleware.GetCorrelationID(c),
		CausationID:            c.GetHeader("X-Causation-ID"),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}

	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"data": newRequestView(result.Request),
		"meta": gin.H{"replayed": result.Replayed,
			"requestId": c.GetHeader("X-Request-ID")}})
}

// list returns approval requests inside the tenant and branch scope.
func (h *Handler) list(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionRead) {
		return
	}

	limit, _ := strconv.Atoi(c.Query("limit"))
	requests, err := h.service.List(c.Request.Context(), tenant.Query{
		OrganizationID:     tenantCtx.OrganizationID,
		PermittedBranchIDs: tenantCtx.PermittedBranchIDs,
		Limit:              limit,
	}, repositories.ListFilter{
		ResourceType: c.Query("resourceType"),
		ResourceID:   c.Query("resourceId"),
		Action:       c.Query("action"),
		Status:       c.Query("status"),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]RequestView, 0, len(requests))
	for index := range requests {
		views = append(views, newRequestView(&requests[index]))
	}
	respond.OK(c, gin.H{"approvals": views})
}

// get returns one approval request.
func (h *Handler) get(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionRead) {
		return
	}
	request, err := h.service.Get(c.Request.Context(), tenantCtx.OrganizationID,
		c.Param("approvalId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	// Out-of-branch identifiers answer as missing, never as forbidden.
	if request.BranchID != nil && !tenantCtx.BranchPermitted(*request.BranchID) {
		respond.Error(c, apperrors.NotFound("Approval request not found.", nil))
		return
	}
	respond.OK(c, newRequestView(request))
}

// decisions returns the immutable decisions on one request.
func (h *Handler) decisions(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionRead) {
		return
	}
	decisions, err := h.service.Decisions(c.Request.Context(), tenantCtx.OrganizationID,
		c.Param("approvalId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(decisions))
	for _, decision := range decisions {
		views = append(views, gin.H{
			"id":                  decision.ID,
			"approverId":          decision.ApproverID,
			"decision":            decision.Decision,
			"reason":              decision.Reason,
			"requestVersion":      decision.RequestVersion,
			"approvedFingerprint": decision.ApprovedFingerprint,
			"decidedAt":           decision.DecidedAt,
		})
	}
	respond.OK(c, gin.H{"decisions": views})
}

// history returns the append-only history of one request.
func (h *Handler) history(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionRead) {
		return
	}
	entries, err := h.service.History(c.Request.Context(), tenantCtx.OrganizationID,
		c.Param("approvalId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(entries))
	for _, entry := range entries {
		views = append(views, gin.H{
			"id":         entry.ID,
			"fromStatus": entry.FromStatus,
			"toStatus":   entry.ToStatus,
			"actorType":  entry.ActorType,
			"reasonCode": entry.ReasonCode,
			"occurredAt": entry.OccurredAt,
		})
	}
	respond.OK(c, gin.H{"history": views})
}

// decideBody is the corporate approve/reject body.
type decideBody struct {
	Reason              string `json:"reason"`
	ClientDecisionID    string `json:"clientDecisionId"`
	ExpectedFingerprint string `json:"expectedFingerprint"`
}

func (h *Handler) approve(c *gin.Context) { h.decide(c, models.DecisionApproved) }

func (h *Handler) reject(c *gin.Context) { h.decide(c, models.DecisionRejected) }

// decide records one approver decision.
func (h *Handler) decide(c *gin.Context, decision string) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionDecide) {
		return
	}

	var body decideBody
	if err := c.ShouldBindJSON(&body); err != nil && c.Request.ContentLength > 0 {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	request, err := h.service.Decide(c.Request.Context(), usecases.DecideInput{
		Actor:               actorFrom(tenantCtx, ""),
		ApprovalRequestID:   c.Param("approvalId"),
		Decision:            decision,
		Reason:              body.Reason,
		ClientDecisionID:    body.ClientDecisionID,
		ExpectedFingerprint: body.ExpectedFingerprint,
		CorrelationID:       middleware.GetCorrelationID(c),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, newRequestView(request))
}

// cancelBody carries the withdrawal reason.
type cancelBody struct {
	Reason string `json:"reason"`
}

// cancel withdraws a pending request.
func (h *Handler) cancel(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionCancel) {
		return
	}

	var body cancelBody
	if err := c.ShouldBindJSON(&body); err != nil && c.Request.ContentLength > 0 {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	request, err := h.service.Cancel(c.Request.Context(), usecases.CancelInput{
		Actor:             actorFrom(tenantCtx, ""),
		ApprovalRequestID: c.Param("approvalId"),
		Reason:            body.Reason,
		CorrelationID:     middleware.GetCorrelationID(c),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, newRequestView(request))
}

// executeBody is the corporate execution body.
type executeBody struct {
	Payload            map[string]any `json:"payload"`
	ExecutionReference string         `json:"executionReference"`
}

// execute spends an approval on the protected domain operation.
//
// The protected operation is supplied by the domain that owns it; this route
// runs the generic half — fingerprint equality, subject version, exactly-once
// — and passes a nil operation, which records the spend without performing a
// side effect. Domains whose operation must run atomically call
// Service.Execute directly with their own run function rather than routing
// through here.
func (h *Handler) execute(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionExecute) {
		return
	}

	var body executeBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if idempotencyKey == "" {
		respond.Error(c, apperrors.BadRequest("An Idempotency-Key header is required.", nil))
		return
	}

	result, err := h.service.Execute(c.Request.Context(), usecases.ExecuteInput{
		Actor:              actorFrom(tenantCtx, ""),
		ApprovalRequestID:  c.Param("approvalId"),
		Payload:            body.Payload,
		ExecutionReference: body.ExecutionReference,
		IdempotencyKey:     idempotencyKey,
		CorrelationID:      middleware.GetCorrelationID(c),
		CausationID:        c.GetHeader("X-Causation-ID"),
	}, func(pgx.Tx, *models.Request) error { return nil })
	if err != nil {
		respond.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": newRequestView(result.Request),
		"meta": gin.H{"replayed": result.Replayed}})
}
