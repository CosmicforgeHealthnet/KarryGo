// Package finance also exposes the corporate finance operator API.
//
// Every route here is tenant-scoped, permission-gated and branch-filtered.
// Reading a stuck payout, re-running it, and asking for money to move are three
// separate permissions: someone who may investigate is not automatically
// someone who may act, and neither is automatically someone who may spend.
package finance

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the finance operator routes.
const (
	PermissionOperationsRead       = "finance.operations.read"
	PermissionOperationsReplay     = "finance.operations.replay"
	PermissionReconciliationRead   = "finance.reconciliation.read"
	PermissionReconciliationManage = "finance.reconciliation.manage"
	PermissionSettlementsRequest   = "finance.settlements.request"
	PermissionRefundsRequest       = "finance.refunds.request"
	PermissionReversalsRequest     = "finance.reversals.request"
)

// OperatorHandler serves the corporate finance operator routes.
type OperatorHandler struct {
	service    *Service
	authorizer *accesscontrol.Authorizer
}

// NewOperatorHandler builds the handler.
func NewOperatorHandler(service *Service, authorizer *accesscontrol.Authorizer) *OperatorHandler {
	return &OperatorHandler{service: service, authorizer: authorizer}
}

// RegisterTenantRoutes mounts routes that require resolved organization scope.
func (h *OperatorHandler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.GET("/finance/operations", h.listOperations)
	group.GET("/finance/operations/:operationId", h.getOperation)
	group.GET("/finance/operations/:operationId/history", h.operationHistory)
	group.GET("/finance/operations/:operationId/audit", h.operationAudit)
	group.POST("/finance/operations/:operationId/replay", h.replayOperation)

	group.GET("/finance/projections", h.listProjections)
	group.GET("/finance/projections/:projectionId", h.getProjection)

	group.GET("/finance/reconciliation-cases", h.listReconciliationCases)
	group.GET("/finance/reconciliation-cases/:projectionId", h.getProjection)
	group.POST("/finance/reconciliation-cases/:projectionId/replay", h.replayReconciliation)
	group.POST("/finance/reconciliation-cases/:projectionId/read-back", h.requestReadBack)

	group.POST("/finance/settlements", h.requestSettlement)
	group.POST("/finance/refunds", h.requestRefund)
	group.POST("/finance/reversals", h.requestReversal)
}

// gate resolves the tenant, enforces the permission and returns the read scope.
//
// The scope is derived from the resolved membership, never from the request:
// a caller cannot widen what it may see by asking for a different organization.
func (h *OperatorHandler) gate(c *gin.Context, permission string) (tenant.Context, Scope, bool) {
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, Scope{}, false
	}
	if h.authorizer == nil {
		// A missing gate must never read as an open one.
		respond.Error(c, apperrors.Forbidden("You do not have permission to perform this action.", nil))
		return tenant.Context{}, Scope{}, false
	}
	if err := h.authorizer.Require(c.Request.Context(), tenantCtx.OrganizationID,
		tenantCtx.OrganizationUserID, roleVersionFrom(c), permission,
		accesscontrol.Target{BranchID: c.Query("branchId")}); err != nil {
		respond.Error(c, err)
		return tenant.Context{}, Scope{}, false
	}
	if h.service == nil {
		respond.Error(c, apperrors.Unavailable("Financial integration is not available.", nil))
		return tenant.Context{}, Scope{}, false
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	return tenantCtx, Scope{OrganizationID: tenantCtx.OrganizationID,
		BranchIDs: tenantCtx.PermittedBranchIDs, Limit: limit}, true
}

func roleVersionFrom(c *gin.Context) int64 {
	if value, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if version, ok := value.(int64); ok {
			return version
		}
	}
	return 1
}

// failFinance maps a domain error onto the corporate error envelope.
//
// A record outside the caller's scope is reported as not found, never as
// forbidden: telling an operator that an identifier exists but belongs to
// someone else is itself a disclosure.
func failFinance(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		respond.Error(c, apperrors.NotFound("The requested financial record was not found.", nil))
	case errors.Is(err, ErrSeparationOfDuties):
		respond.Error(c, apperrors.Forbidden("You cannot act on a request you made yourself.", nil))
	case errors.Is(err, ErrFingerprintConflict):
		respond.Error(c, apperrors.Conflict("This idempotency key was already used with different values.", nil))
	case errors.Is(err, ErrFinancialStateBlocked):
		respond.Error(c, apperrors.Conflict("Authoritative financial state does not currently permit this action.", nil))
	case errors.Is(err, ErrReadBackNotAvailable):
		respond.Error(c, apperrors.Conflict("An authoritative read-back is already in progress.", nil))
	default:
		respond.Error(c, err)
	}
}

func idempotencyKey(c *gin.Context) (string, bool) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("An Idempotency-Key header is required.", nil))
		return "", false
	}
	return key, true
}

func actorFrom(c *gin.Context, tenantCtx tenant.Context) Actor {
	return Actor{Type: string(tenantCtx.ActorType), ID: tenantCtx.OrganizationUserID,
		CorrelationID: middleware.GetCorrelationID(c), CausationID: c.GetHeader("X-Causation-ID")}
}

func (h *OperatorHandler) listOperations(c *gin.Context) {
	_, scope, ok := h.gate(c, PermissionOperationsRead)
	if !ok {
		return
	}
	values, err := h.service.ListOperations(c.Request.Context(), scope, OperationFilter{
		Status: c.Query("status"), OperationType: c.Query("operationType"),
		SubjectID: c.Query("subjectId")})
	if err != nil {
		failFinance(c, err)
		return
	}
	respond.OK(c, gin.H{"operations": values})
}

func (h *OperatorHandler) getOperation(c *gin.Context) {
	_, scope, ok := h.gate(c, PermissionOperationsRead)
	if !ok {
		return
	}
	value, err := h.service.GetOperation(c.Request.Context(), scope, c.Param("operationId"))
	if err != nil {
		failFinance(c, err)
		return
	}
	respond.OK(c, value)
}

func (h *OperatorHandler) operationHistory(c *gin.Context) {
	_, scope, ok := h.gate(c, PermissionOperationsRead)
	if !ok {
		return
	}
	entries, err := h.service.OperationHistory(c.Request.Context(), scope, c.Param("operationId"))
	if err != nil {
		failFinance(c, err)
		return
	}
	respond.OK(c, gin.H{"history": entries})
}

func (h *OperatorHandler) operationAudit(c *gin.Context) {
	_, scope, ok := h.gate(c, PermissionOperationsRead)
	if !ok {
		return
	}
	entries, err := h.service.OperationAudit(c.Request.Context(), scope, c.Param("operationId"))
	if err != nil {
		failFinance(c, err)
		return
	}
	respond.OK(c, gin.H{"audit": entries})
}

func (h *OperatorHandler) replayOperation(c *gin.Context) {
	tenantCtx, scope, ok := h.gate(c, PermissionOperationsReplay)
	if !ok {
		return
	}
	// Ownership is proved before the replay so an identifier from another
	// tenant cannot reach the mutation at all.
	if _, err := h.service.GetOperation(c.Request.Context(), scope, c.Param("operationId")); err != nil {
		failFinance(c, err)
		return
	}
	if err := h.service.ReplayOperation(c.Request.Context(), tenantCtx.OrganizationID,
		c.Param("operationId"), tenantCtx.OrganizationUserID, middleware.GetCorrelationID(c)); err != nil {
		failFinance(c, err)
		return
	}
	respond.OK(c, gin.H{"status": "PENDING"})
}

func (h *OperatorHandler) listProjections(c *gin.Context) {
	_, scope, ok := h.gate(c, PermissionOperationsRead)
	if !ok {
		return
	}
	values, err := h.service.ListProjections(c.Request.Context(), scope, ProjectionFilter{
		ProjectionType: c.Query("projectionType"), Freshness: c.Query("freshness"),
		SubjectID: c.Query("subjectId")})
	if err != nil {
		failFinance(c, err)
		return
	}
	respond.OK(c, gin.H{"projections": values})
}

func (h *OperatorHandler) getProjection(c *gin.Context) {
	permission := PermissionOperationsRead
	if strings.Contains(c.FullPath(), "reconciliation-cases") {
		permission = PermissionReconciliationRead
	}
	_, scope, ok := h.gate(c, permission)
	if !ok {
		return
	}
	value, err := h.service.GetProjection(c.Request.Context(), scope, c.Param("projectionId"))
	if err != nil {
		failFinance(c, err)
		return
	}
	respond.OK(c, value)
}

func (h *OperatorHandler) listReconciliationCases(c *gin.Context) {
	_, scope, ok := h.gate(c, PermissionReconciliationRead)
	if !ok {
		return
	}
	values, err := h.service.ListProjections(c.Request.Context(), scope, ProjectionFilter{
		ProjectionType: c.Query("projectionType"), SubjectID: c.Query("subjectId"),
		ReconciliationOnly: true})
	if err != nil {
		failFinance(c, err)
		return
	}
	respond.OK(c, gin.H{"cases": values})
}

func (h *OperatorHandler) replayReconciliation(c *gin.Context) {
	tenantCtx, scope, ok := h.gate(c, PermissionReconciliationManage)
	if !ok {
		return
	}
	if _, err := h.service.GetProjection(c.Request.Context(), scope, c.Param("projectionId")); err != nil {
		failFinance(c, err)
		return
	}
	if err := h.service.ReplayProjection(c.Request.Context(), tenantCtx.OrganizationID,
		c.Param("projectionId"), tenantCtx.OrganizationUserID, middleware.GetCorrelationID(c)); err != nil {
		failFinance(c, err)
		return
	}
	respond.OK(c, gin.H{"status": "PENDING"})
}

func (h *OperatorHandler) requestReadBack(c *gin.Context) {
	tenantCtx, scope, ok := h.gate(c, PermissionReconciliationManage)
	if !ok {
		return
	}
	if err := h.service.RequestReadBack(c.Request.Context(), scope, c.Param("projectionId"),
		string(tenantCtx.ActorType), tenantCtx.OrganizationUserID,
		middleware.GetCorrelationID(c)); err != nil {
		failFinance(c, err)
		return
	}
	respond.OK(c, gin.H{"status": "PENDING"})
}

func (h *OperatorHandler) requestSettlement(c *gin.Context) {
	tenantCtx, _, ok := h.gate(c, PermissionSettlementsRequest)
	if !ok {
		return
	}
	key, ok := idempotencyKey(c)
	if !ok {
		return
	}
	var body struct {
		BeneficiaryType      string `json:"beneficiaryType"`
		BeneficiaryReference string `json:"beneficiaryReference"`
		EarningAccountID     string `json:"earningAccountId"`
		WalletID             string `json:"walletId"`
		BranchID             string `json:"branchId"`
		Currency             string `json:"currency"`
		ExpectedVersion      int64  `json:"expectedVersion"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if !h.branchPermitted(c, tenantCtx, body.BranchID) {
		return
	}
	operation, err := h.service.RequestSettlement(c.Request.Context(), SettlementRequest{
		Actor: actorFrom(c, tenantCtx), OrganizationID: tenantCtx.OrganizationID,
		BranchID: body.BranchID, BeneficiaryType: body.BeneficiaryType,
		BeneficiaryReference: body.BeneficiaryReference, EarningAccountID: body.EarningAccountID,
		WalletID: body.WalletID, Currency: body.Currency, IdempotencyKey: key,
		ExpectedVersion: body.ExpectedVersion})
	if err != nil {
		failFinance(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": operationAccepted(operation)})
}

func (h *OperatorHandler) requestRefund(c *gin.Context) {
	tenantCtx, _, ok := h.gate(c, PermissionRefundsRequest)
	if !ok {
		return
	}
	key, ok := idempotencyKey(c)
	if !ok {
		return
	}
	var body struct {
		OriginalJournalID string `json:"originalJournalId"`
		PaymentReference  string `json:"paymentReference"`
		BranchID          string `json:"branchId"`
		AmountMinor       int64  `json:"amountMinor"`
		Currency          string `json:"currency"`
		ReasonCode        string `json:"reasonCode"`
		Reason            string `json:"reason"`
		ExpectedVersion   int64  `json:"expectedVersion"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if !h.branchPermitted(c, tenantCtx, body.BranchID) {
		return
	}
	operation, err := h.service.RequestRefund(c.Request.Context(), RefundRequest{
		Actor: actorFrom(c, tenantCtx), OrganizationID: tenantCtx.OrganizationID,
		BranchID: body.BranchID, OriginalJournalID: body.OriginalJournalID,
		PaymentReference: body.PaymentReference, AmountMinor: body.AmountMinor,
		Currency: body.Currency, ReasonCode: body.ReasonCode, Reason: body.Reason,
		IdempotencyKey: key, ExpectedVersion: body.ExpectedVersion})
	if err != nil {
		failFinance(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": operationAccepted(operation)})
}

func (h *OperatorHandler) requestReversal(c *gin.Context) {
	tenantCtx, _, ok := h.gate(c, PermissionReversalsRequest)
	if !ok {
		return
	}
	key, ok := idempotencyKey(c)
	if !ok {
		return
	}
	var body struct {
		OriginalJournalID string `json:"originalJournalId"`
		SourceOperationID string `json:"sourceOperationId"`
		BranchID          string `json:"branchId"`
		AmountMinor       int64  `json:"amountMinor"`
		Currency          string `json:"currency"`
		ReasonCode        string `json:"reasonCode"`
		Reason            string `json:"reason"`
		ExpectedVersion   int64  `json:"expectedVersion"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if !h.branchPermitted(c, tenantCtx, body.BranchID) {
		return
	}
	operation, err := h.service.RequestReversal(c.Request.Context(), ReversalRequest{
		Actor: actorFrom(c, tenantCtx), OrganizationID: tenantCtx.OrganizationID,
		BranchID: body.BranchID, OriginalJournalID: body.OriginalJournalID,
		SourceOperationID: body.SourceOperationID, AmountMinor: body.AmountMinor,
		Currency: body.Currency, ReasonCode: body.ReasonCode, Reason: body.Reason,
		IdempotencyKey: key, ExpectedVersion: body.ExpectedVersion})
	if err != nil {
		failFinance(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": operationAccepted(operation)})
}

// branchPermitted refuses a write aimed at a branch the caller has no authority
// over. An organization-scoped membership carries no branch restriction.
func (h *OperatorHandler) branchPermitted(c *gin.Context, tenantCtx tenant.Context, branchID string) bool {
	if branchID == "" || len(tenantCtx.PermittedBranchIDs) == 0 {
		return true
	}
	for _, permitted := range tenantCtx.PermittedBranchIDs {
		if permitted == branchID {
			return true
		}
	}
	respond.Error(c, apperrors.Forbidden("You do not have authority over this branch.", nil))
	return false
}

// operationAccepted is the queued-command acknowledgement. It never leaks the
// Wallet reference before the operation has actually reached Wallet.
func operationAccepted(operation *Operation) gin.H {
	if operation == nil {
		return gin.H{}
	}
	return gin.H{"id": operation.ID, "operationType": operation.OperationType,
		"status": operation.Status, "subjectId": operation.SubjectID,
		"version": operation.Version, "nextAttemptAt": operation.NextAttemptAt}
}
