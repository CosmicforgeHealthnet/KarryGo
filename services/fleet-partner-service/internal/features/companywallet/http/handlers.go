// Package http exposes the Section 13 corporate and provider wallet routes.
//
// Corporate staff and providers reach the same domain through different route
// classes with different identity resolution. A staff member can never act as
// a provider: the provider-scoped routes resolve the membership from the
// provider's own authenticated session, never from request input, so naming
// somebody else's membership in a path yields a not-found rather than access.
package http

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the Section 13 routes.
const (
	PermissionWalletRead         = "wallet.read"
	PermissionTransactionsRead   = "wallet.transactions.read"
	PermissionSubWalletRead      = "wallet.provider_subwallet.read"
	PermissionAllocationCreate   = "wallet.provider_allocations.create"
	PermissionAllocationApprove  = "wallet.provider_allocations.approve"
	PermissionWithdrawalsRead    = "wallet.provider_withdrawals.read"
	PermissionWithdrawalsApprove = "wallet.provider_withdrawals.approve"
	PermissionWithdrawalsReject  = "wallet.provider_withdrawals.reject"
	PermissionPoliciesManage     = "wallet.policies.manage"
)

// Handler serves the Section 13 routes.
type Handler struct {
	service    *usecases.Service
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// RegisterTenantRoutes mounts the corporate routes.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.GET("/wallet", h.getWallet)
	group.GET("/wallet/transactions", h.listTransactions)
	group.GET("/wallet/transactions/:transactionId", h.getTransaction)
	group.GET("/wallet/policy", h.getPolicy)
	group.PUT("/wallet/policy", h.setPolicy)

	group.GET("/providers/:membershipId/sub-wallet", h.getSubWallet)
	group.GET("/providers/:membershipId/sub-wallet/statement", h.getStatement)
	group.GET("/providers/:membershipId/sub-wallet/allocations", h.listAllocations)
	group.POST("/providers/:membershipId/sub-wallet/allocations", h.createAllocation)
	group.POST("/providers/:membershipId/sub-wallet/allocations/:allocationId/approve", h.approveAllocation)
	group.POST("/providers/:membershipId/sub-wallet/allocations/:allocationId/reject", h.rejectAllocation)
	group.POST("/providers/:membershipId/sub-wallet/allocations/:allocationId/replay", h.replayAllocation)

	group.GET("/providers/:membershipId/sub-wallet/withdrawals", h.listWithdrawals)
	group.POST("/providers/:membershipId/sub-wallet/withdrawals", h.createWithdrawal)
	group.POST("/providers/:membershipId/sub-wallet/withdrawals/:withdrawalId/approve", h.approveWithdrawal)
	group.POST("/providers/:membershipId/sub-wallet/withdrawals/:withdrawalId/reject", h.rejectWithdrawal)
	group.POST("/providers/:membershipId/sub-wallet/withdrawals/:withdrawalId/replay", h.replayWithdrawal)
}

// ProviderMembershipResolver resolves the membership belonging to the
// authenticated provider session.
//
// Provider routes go through this and never through the path parameter. A
// provider who names a colleague's membership is answered with their own, or
// with not-found; there is no code path where request input selects whose
// money is being read.
type ProviderMembershipResolver interface {
	ResolveMembership(c *gin.Context) (organizationID string, membershipID string, ok bool)
}

// RegisterProviderRoutes mounts the provider-authenticated routes.
func (h *Handler) RegisterProviderRoutes(group *gin.RouterGroup, resolver ProviderMembershipResolver) {
	if resolver == nil {
		// Without a resolver there is no trustworthy way to know whose
		// sub-wallet is being read, so the routes are not mounted at all
		// rather than mounted with a weaker check.
		return
	}
	group.GET("/provider/sub-wallet", h.providerSubWallet(resolver))
	group.GET("/provider/sub-wallet/statement", h.providerStatement(resolver))
	group.POST("/provider/sub-wallet/withdrawals", h.providerWithdrawal(resolver))
}

func requireTenant(c *gin.Context) (tenant.Context, bool) {
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

func (h *Handler) authorize(c *gin.Context, t tenant.Context, permission string) bool {
	if h.authorizer == nil {
		respond.Error(c, apperrors.Forbidden("You do not have permission to perform this action.", nil))
		return false
	}
	if err := h.authorizer.Require(c.Request.Context(), t.OrganizationID, t.OrganizationUserID,
		roleVersionFrom(c), permission, accesscontrol.Target{BranchID: c.Query("branchId")}); err != nil {
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

func idempotencyKey(c *gin.Context) string { return strings.TrimSpace(c.GetHeader("Idempotency-Key")) }

// expectedVersion reads either the body field or If-Match. A caller that
// supplies neither gets 0, which the service reads as "unstated" for reads and
// refuses where a version is mandatory.
func expectedVersion(c *gin.Context, bodyValue int64) int64 {
	if bodyValue != 0 {
		return bodyValue
	}
	raw := strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`)
	if raw == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func pageParams(c *gin.Context) (cursor.Cursor, bool, int, error) {
	decoded, ok, err := cursor.Decode(strings.TrimSpace(c.Query("cursor")))
	if err != nil {
		return decoded, false, 0, apperrors.BadRequest("The cursor is not valid.", nil)
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			return decoded, false, 0, apperrors.BadRequest("limit must be an integer.", nil)
		}
		limit = parsed
	}
	return decoded, ok, cursor.ClampLimit(limit), nil
}

// ---------------------------------------------------------------------------
// Organization wallet
// ---------------------------------------------------------------------------

func (h *Handler) getWallet(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionWalletRead) {
		return
	}
	// authoritative=true asks payment-wallet-service directly. The default is
	// the local projection, which is honest about its own freshness.
	view, err := h.service.GetWallet(c.Request.Context(), t, c.Query("authoritative") == "true")
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, view)
}

// TransactionView is the allowlisted transaction shape.
type TransactionView struct {
	ID                   string `json:"id"`
	TransactionReference string `json:"transactionReference"`
	SubjectType          string `json:"subjectType"`
	SubjectID            string `json:"subjectId,omitempty"`
	Direction            string `json:"direction"`
	Category             string `json:"category"`
	AmountMinor          int64  `json:"amountMinor"`
	Currency             string `json:"currency"`
	SafeStatus           string `json:"status"`
	OccurredAt           string `json:"occurredAt"`
	SourceVersion        int64  `json:"sourceVersion"`
	FreshnessState       string `json:"freshnessState"`
}

func transactionView(value models.TransactionProjection) TransactionView {
	return TransactionView{ID: value.ID, TransactionReference: value.TransactionReference,
		SubjectType: value.SubjectType, SubjectID: value.SubjectID, Direction: value.Direction,
		Category: value.Category, AmountMinor: value.AmountMinor, Currency: value.Currency,
		SafeStatus: value.SafeStatus, OccurredAt: value.OccurredAt.UTC().Format(timeLayout),
		SourceVersion: value.SourceVersion, FreshnessState: value.FreshnessState}
}

const timeLayout = "2006-01-02T15:04:05.000000000Z07:00"

func (h *Handler) listTransactions(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionTransactionsRead) {
		return
	}
	position, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	rows, err := h.service.Repository().ListTransactions(c.Request.Context(), t.OrganizationID,
		t.PermittedBranchIDs, repositories.TransactionFilter{
			WalletReference: strings.TrimSpace(c.Query("walletReference")),
			Category:        strings.TrimSpace(c.Query("category")),
			Direction:       strings.TrimSpace(c.Query("direction")),
			Cursor:          position, HasCursor: hasCursor, Limit: limit})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]TransactionView, 0, len(rows))
	for _, row := range rows {
		views = append(views, transactionView(row))
	}
	next := ""
	if len(rows) == limit && limit > 0 {
		last := rows[len(rows)-1]
		next = cursor.EncodeTime(last.OccurredAt, last.ID)
	}
	respond.List(c, views, next)
}

func (h *Handler) getTransaction(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionTransactionsRead) {
		return
	}
	row, err := h.service.Repository().GetTransaction(c.Request.Context(), t.OrganizationID,
		c.Param("transactionId"), t.PermittedBranchIDs)
	if err != nil {
		respond.Error(c, apperrors.NotFound("Transaction was not found.", nil))
		return
	}
	respond.OK(c, transactionView(row))
}

// ---------------------------------------------------------------------------
// Policy
// ---------------------------------------------------------------------------

func (h *Handler) getPolicy(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionWalletRead) {
		return
	}
	policy, err := h.service.GetPolicy(c.Request.Context(), t)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, policyView(policy))
}

type policyBody struct {
	PolicyMode                       string `json:"policyMode"`
	MinimumAmountMinor               int64  `json:"minimumAmountMinor"`
	ApprovalThresholdMinor           int64  `json:"approvalThresholdMinor"`
	AllocationApprovalThresholdMinor int64  `json:"allocationApprovalThresholdMinor"`
	Currency                         string `json:"currency"`
	ExpectedVersion                  int64  `json:"expectedVersion"`
}

func policyView(policy models.WithdrawalPolicy) gin.H {
	return gin.H{"policyMode": policy.PolicyMode,
		"minimumAmountMinor":               policy.MinimumAmountMinor,
		"approvalThresholdMinor":           policy.ApprovalThresholdMinor,
		"allocationApprovalThresholdMinor": policy.AllocationApprovalThresholdMinor,
		"currency":                         policy.Currency, "version": policy.Version}
}

func (h *Handler) setPolicy(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionPoliciesManage) {
		return
	}
	var body policyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	policy, err := h.service.SetPolicy(c.Request.Context(), usecases.SetPolicyInput{
		Tenant: t, PolicyMode: body.PolicyMode, MinimumAmountMinor: body.MinimumAmountMinor,
		ApprovalThresholdMinor:           body.ApprovalThresholdMinor,
		AllocationApprovalThresholdMinor: body.AllocationApprovalThresholdMinor,
		Currency:                         body.Currency, ExpectedVersion: expectedVersion(c, body.ExpectedVersion)})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, policyView(policy))
}

// ---------------------------------------------------------------------------
// Provider sub-wallet, corporate view
// ---------------------------------------------------------------------------

func (h *Handler) getSubWallet(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionSubWalletRead) {
		return
	}
	view, err := h.service.GetSubWallet(c.Request.Context(), t, c.Param("membershipId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, view)
}

func (h *Handler) getStatement(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionSubWalletRead) {
		return
	}
	statement, err := h.service.ProviderStatement(c.Request.Context(), usecases.StatementInput{
		Tenant: t, MembershipID: c.Param("membershipId")})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, statement)
}

// AllocationView is the allowlisted corporate allocation shape.
//
// The private note is present here and only here: it is company-internal
// commentary, visible to staff who hold the allocation permission and never
// serialized onto a provider-facing route.
type AllocationView struct {
	ID                    string `json:"id"`
	MembershipID          string `json:"membershipId"`
	AllocationType        string `json:"allocationType"`
	AmountMinor           int64  `json:"amountMinor"`
	Currency              string `json:"currency"`
	BusinessReferenceType string `json:"businessReferenceType"`
	BusinessReferenceID   string `json:"businessReferenceId"`
	ReasonCode            string `json:"reasonCode"`
	ProviderExplanation   string `json:"providerVisibleExplanation,omitempty"`
	PrivateNote           string `json:"privateNote,omitempty"`
	Status                string `json:"status"`
	ApprovalRequestID     string `json:"approvalRequestId,omitempty"`
	SafeErrorCode         string `json:"safeErrorCode,omitempty"`
	Version               int64  `json:"version"`
	CreatedAt             string `json:"createdAt"`
}

func allocationView(value models.AllocationInstruction) AllocationView {
	return AllocationView{ID: value.ID, MembershipID: value.ProviderMembershipID,
		AllocationType: value.AllocationType, AmountMinor: value.AmountMinor,
		Currency: value.Currency, BusinessReferenceType: value.BusinessReferenceType,
		BusinessReferenceID: value.BusinessReferenceID, ReasonCode: value.ReasonCode,
		ProviderExplanation: value.ProviderExplanation, PrivateNote: value.PrivateNote,
		Status: value.Status, ApprovalRequestID: value.ApprovalRequestID,
		SafeErrorCode: value.SafeErrorCode, Version: value.Version,
		CreatedAt: value.CreatedAt.UTC().Format(timeLayout)}
}

type allocationBody struct {
	AllocationType               string `json:"allocationType"`
	AmountMinor                  int64  `json:"amountMinor"`
	Currency                     string `json:"currency"`
	BusinessReferenceType        string `json:"businessReferenceType"`
	BusinessReferenceID          string `json:"businessReferenceId"`
	ReasonCode                   string `json:"reasonCode"`
	ProviderExplanation          string `json:"providerVisibleExplanation"`
	PrivateNote                  string `json:"privateNote"`
	LegalBasisReference          string `json:"legalBasisReference"`
	BranchID                     string `json:"branchId"`
	ExpectedVersion              int64  `json:"expectedVersion"`
	RequireRecipientConfirmation bool   `json:"requireRecipientConfirmation"`
}

func (h *Handler) createAllocation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionAllocationCreate) {
		return
	}
	var body allocationBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	// An approved deduction needs the approve permission on top of the create
	// permission. Holding one is not holding the other.
	if models.DebitsProvider(body.AllocationType) && !h.authorize(c, t, PermissionAllocationApprove) {
		return
	}
	instruction, err := h.service.RequestAllocation(c.Request.Context(), usecases.AllocationInput{
		Tenant: t, BranchID: body.BranchID, MembershipID: c.Param("membershipId"),
		AllocationType: body.AllocationType, AmountMinor: body.AmountMinor,
		Currency: body.Currency, BusinessReferenceType: body.BusinessReferenceType,
		BusinessReferenceID: body.BusinessReferenceID, ReasonCode: body.ReasonCode,
		ProviderExplanation: body.ProviderExplanation, PrivateNote: body.PrivateNote,
		LegalBasisReference: body.LegalBasisReference, IdempotencyKey: idempotencyKey(c),
		ExpectedVersion:              expectedVersion(c, body.ExpectedVersion),
		RequireRecipientConfirmation: body.RequireRecipientConfirmation})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.JSON(c, http.StatusCreated, allocationView(instruction))
}

type decisionBody struct {
	ReasonCode      string `json:"reasonCode"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handler) approveAllocation(c *gin.Context) { h.decideAllocation(c, true) }
func (h *Handler) rejectAllocation(c *gin.Context)  { h.decideAllocation(c, false) }

func (h *Handler) decideAllocation(c *gin.Context, approve bool) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionAllocationApprove) {
		return
	}
	var body decisionBody
	_ = c.ShouldBindJSON(&body)
	instruction, err := h.service.DecideAllocation(c.Request.Context(), t,
		c.Param("allocationId"), expectedVersion(c, body.ExpectedVersion), approve, body.ReasonCode)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, allocationView(instruction))
}

func (h *Handler) replayAllocation(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionAllocationApprove) {
		return
	}
	if err := h.service.ReplayAllocation(c.Request.Context(), t, c.Param("allocationId")); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"status": models.AllocationApproved})
}

func (h *Handler) listAllocations(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionSubWalletRead) {
		return
	}
	position, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	rows, err := h.service.ListAllocations(c.Request.Context(), t, repositories.AllocationFilter{
		MembershipID: c.Param("membershipId"), Status: strings.TrimSpace(c.Query("status")),
		AllocationType: strings.TrimSpace(c.Query("allocationType")),
		Cursor:         position, HasCursor: hasCursor, Limit: limit})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]AllocationView, 0, len(rows))
	for _, row := range rows {
		views = append(views, allocationView(row))
	}
	next := ""
	if len(rows) == limit && limit > 0 {
		last := rows[len(rows)-1]
		next = cursor.EncodeTime(last.CreatedAt, last.ID)
	}
	respond.List(c, views, next)
}

// ---------------------------------------------------------------------------
// Withdrawals
// ---------------------------------------------------------------------------

// WithdrawalView is the allowlisted withdrawal shape.
//
// The destination reference is deliberately absent even though it is opaque:
// a handle that resolves to somebody's bank account does not belong in a
// listing that a broad read permission can reach.
type WithdrawalView struct {
	ID                string `json:"id"`
	MembershipID      string `json:"membershipId"`
	AmountMinor       int64  `json:"amountMinor"`
	Currency          string `json:"currency"`
	PolicyMode        string `json:"policyMode"`
	Status            string `json:"status"`
	RequestedByType   string `json:"requestedByActorType"`
	ApprovalRequestID string `json:"approvalRequestId,omitempty"`
	SafeErrorCode     string `json:"safeErrorCode,omitempty"`
	Version           int64  `json:"version"`
	CreatedAt         string `json:"createdAt"`
}

func withdrawalView(value models.WithdrawalRequest) WithdrawalView {
	return WithdrawalView{ID: value.ID, MembershipID: value.ProviderMembershipID,
		AmountMinor: value.AmountMinor, Currency: value.Currency, PolicyMode: value.PolicyMode,
		Status: value.Status, RequestedByType: value.RequestedByActorType,
		ApprovalRequestID: value.ApprovalRequestID, SafeErrorCode: value.SafeErrorCode,
		Version: value.Version, CreatedAt: value.CreatedAt.UTC().Format(timeLayout)}
}

type withdrawalBody struct {
	AmountMinor          int64  `json:"amountMinor"`
	Currency             string `json:"currency"`
	DestinationReference string `json:"destinationBankAccountReference"`
	BranchID             string `json:"branchId"`
	ExpectedVersion      int64  `json:"expectedVersion"`
}

func (h *Handler) createWithdrawal(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionWithdrawalsApprove) {
		return
	}
	var body withdrawalBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	request, err := h.service.RequestWithdrawal(c.Request.Context(), usecases.WithdrawalInput{
		Tenant: t, BranchID: body.BranchID, MembershipID: c.Param("membershipId"),
		RequestedByActorType: usecases.RequesterStaff, AmountMinor: body.AmountMinor,
		Currency: body.Currency, DestinationReference: body.DestinationReference,
		IdempotencyKey: idempotencyKey(c), ExpectedVersion: expectedVersion(c, body.ExpectedVersion)})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.JSON(c, http.StatusCreated, withdrawalView(request))
}

func (h *Handler) approveWithdrawal(c *gin.Context) { h.decideWithdrawal(c, true) }
func (h *Handler) rejectWithdrawal(c *gin.Context)  { h.decideWithdrawal(c, false) }

func (h *Handler) decideWithdrawal(c *gin.Context, approve bool) {
	t, ok := requireTenant(c)
	if !ok {
		return
	}
	permission := PermissionWithdrawalsApprove
	if !approve {
		permission = PermissionWithdrawalsReject
	}
	if !h.authorize(c, t, permission) {
		return
	}
	var body decisionBody
	_ = c.ShouldBindJSON(&body)
	request, err := h.service.DecideWithdrawal(c.Request.Context(), t, c.Param("membershipId"),
		c.Param("withdrawalId"), expectedVersion(c, body.ExpectedVersion), approve, body.ReasonCode)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, withdrawalView(request))
}

func (h *Handler) replayWithdrawal(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionWithdrawalsApprove) {
		return
	}
	if err := h.service.ReplayWithdrawal(c.Request.Context(), t, c.Param("withdrawalId")); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"status": models.WithdrawalApproved})
}

func (h *Handler) listWithdrawals(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionWithdrawalsRead) {
		return
	}
	position, hasCursor, limit, err := pageParams(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	rows, err := h.service.ListWithdrawals(c.Request.Context(), t, repositories.WithdrawalFilter{
		MembershipID: c.Param("membershipId"), Status: strings.TrimSpace(c.Query("status")),
		Cursor: position, HasCursor: hasCursor, Limit: limit})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]WithdrawalView, 0, len(rows))
	for _, row := range rows {
		views = append(views, withdrawalView(row))
	}
	next := ""
	if len(rows) == limit && limit > 0 {
		last := rows[len(rows)-1]
		next = cursor.EncodeTime(last.CreatedAt, last.ID)
	}
	respond.List(c, views, next)
}

// ---------------------------------------------------------------------------
// Provider-authenticated routes
// ---------------------------------------------------------------------------

// providerTenant builds a tenant context from the provider's own session.
//
// ActorType is PLATFORM_SERVICE-adjacent rather than organization user: the
// provider is not a member of the company's staff, and treating them as one
// would give them the staff permission surface.
func providerTenant(organizationID, membershipID string, c *gin.Context) tenant.Context {
	return tenant.Context{OrganizationID: organizationID, ActorType: tenant.ActorTypeSystem,
		ActorID: membershipID, CorrelationID: middleware.GetCorrelationID(c)}
}

func (h *Handler) providerSubWallet(resolver ProviderMembershipResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, membershipID, ok := resolver.ResolveMembership(c)
		if !ok {
			respond.Error(c, apperrors.Forbidden("Provider authentication is required.", nil))
			return
		}
		view, err := h.service.GetSubWallet(c.Request.Context(),
			providerTenant(organizationID, membershipID, c), membershipID)
		if err != nil {
			respond.Error(c, err)
			return
		}
		respond.OK(c, view)
	}
}

func (h *Handler) providerStatement(resolver ProviderMembershipResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, membershipID, ok := resolver.ResolveMembership(c)
		if !ok {
			respond.Error(c, apperrors.Forbidden("Provider authentication is required.", nil))
			return
		}
		statement, err := h.service.ProviderStatement(c.Request.Context(), usecases.StatementInput{
			Tenant: providerTenant(organizationID, membershipID, c), MembershipID: membershipID,
			ProviderScoped: true})
		if err != nil {
			respond.Error(c, err)
			return
		}
		respond.OK(c, statement)
	}
}

func (h *Handler) providerWithdrawal(resolver ProviderMembershipResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		organizationID, membershipID, ok := resolver.ResolveMembership(c)
		if !ok {
			respond.Error(c, apperrors.Forbidden("Provider authentication is required.", nil))
			return
		}
		var body withdrawalBody
		if err := c.ShouldBindJSON(&body); err != nil {
			respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
			return
		}
		request, err := h.service.RequestWithdrawal(c.Request.Context(), usecases.WithdrawalInput{
			Tenant: providerTenant(organizationID, membershipID, c), MembershipID: membershipID,
			RequestedByActorType: usecases.RequesterProvider, AmountMinor: body.AmountMinor,
			Currency: body.Currency, DestinationReference: body.DestinationReference,
			IdempotencyKey:  idempotencyKey(c),
			ExpectedVersion: expectedVersion(c, body.ExpectedVersion)})
		if err != nil {
			respond.Error(c, err)
			return
		}
		respond.JSON(c, http.StatusCreated, withdrawalView(request))
	}
}
