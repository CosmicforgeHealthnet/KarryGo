package http

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/usecases"
)

// registerProviderRoutes mounts the Section 13 provider sub-wallet surface on
// the already HMAC-authenticated, allowlisted, replay-protected group.
func registerProviderRoutes(group *gin.RouterGroup, h *Handler) {
	group.POST("/provider-subwallets/ensure", h.ensureProviderSubWallet)
	group.POST("/provider-allocations", h.postProviderAllocation)
	group.GET("/organizations/:organizationId/provider-allocations/:id", h.getProviderAllocation)
	group.POST("/provider-withdrawals", h.requestProviderWithdrawal)
	group.POST("/provider-withdrawals/:id/approve", h.approveProviderWithdrawal)
	group.POST("/provider-withdrawals/:id/reject", h.rejectProviderWithdrawal)
	group.POST("/provider-withdrawals/:id/replay", h.replayProviderWithdrawal)
	group.GET("/organizations/:organizationId/provider-withdrawals/:id", h.getProviderWithdrawal)
	group.POST("/provider-statements", h.providerStatement)
}

func (h *Handler) ensureProviderSubWallet(c *gin.Context) {
	var req struct {
		base
		OrganizationWalletID     string `json:"organizationWalletId"`
		ProviderID               string `json:"providerId"`
		OrganizationMembershipID string `json:"organizationMembershipId"`
		ProviderService          string `json:"providerService"`
		ProviderServiceCategory  string `json:"providerServiceCategory"`
		ProviderReference        string `json:"providerReference"`
		Currency                 string `json:"currency"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.EnsureProviderSubWallet(c, usecases.EnsureProviderSubWalletInput{
		Meta: h.meta(c, req.base), OrganizationID: req.OrganizationID,
		OrganizationWalletID: req.OrganizationWalletID, ProviderID: req.ProviderID,
		OrganizationMembershipID: req.OrganizationMembershipID,
		ProviderService:          req.ProviderService,
		ProviderServiceCategory:  req.ProviderServiceCategory,
		ProviderReference:        req.ProviderReference, Currency: req.Currency})
	if err != nil {
		fail(c, err)
		return
	}
	// Only the safe projection is returned. Balances are deliberately absent:
	// Fleet stores an opaque reference and a status, never a balance it could
	// later treat as authoritative.
	respond(c, gin.H{"id": value.ID, "organizationId": value.OrganizationID,
		"currency": value.Currency, "status": value.Status, "version": value.Version,
		"createdAt": value.CreatedAt, "updatedAt": value.UpdatedAt})
}

func (h *Handler) postProviderAllocation(c *gin.Context) {
	var req struct {
		base
		EarningAccountID         string `json:"earningAccountId"`
		OrganizationMembershipID string `json:"organizationMembershipId"`
		ProviderID               string `json:"providerId"`
		AllocationType           string `json:"allocationType"`
		BusinessReferenceType    string `json:"businessReferenceType"`
		BusinessReferenceID      string `json:"businessReferenceId"`
		AmountMinor              int64  `json:"amountMinor"`
		Currency                 string `json:"currency"`
		ReasonCode               string `json:"reasonCode"`
		ProviderExplanation      string `json:"providerExplanation"`
		ApprovalReference        string `json:"approvalReference"`
		SourceService            string `json:"sourceService"`
		SourceReference          string `json:"sourceReference"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.PostProviderAllocation(c, usecases.ProviderAllocationInput{
		Meta: h.meta(c, req.base), OrganizationID: req.OrganizationID,
		EarningAccountID:         req.EarningAccountID,
		OrganizationMembershipID: req.OrganizationMembershipID, ProviderID: req.ProviderID,
		AllocationType: req.AllocationType, BusinessReferenceType: req.BusinessReferenceType,
		BusinessReferenceID: req.BusinessReferenceID, AmountMinor: req.AmountMinor,
		Currency: req.Currency, ReasonCode: req.ReasonCode,
		ProviderExplanation: req.ProviderExplanation, ApprovalReference: req.ApprovalReference,
		SourceService: req.SourceService, SourceReference: req.SourceReference,
		ExpectedVersion: req.ExpectedVersion})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}

func (h *Handler) getProviderAllocation(c *gin.Context) {
	organizationID := c.Param("organizationId")
	value, err := h.service.GetProviderAllocation(c, h.readMeta(c, organizationID),
		organizationID, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}

func (h *Handler) requestProviderWithdrawal(c *gin.Context) {
	var req struct {
		base
		EarningAccountID         string `json:"earningAccountId"`
		OrganizationMembershipID string `json:"organizationMembershipId"`
		ProviderID               string `json:"providerId"`
		AmountMinor              int64  `json:"amountMinor"`
		Currency                 string `json:"currency"`
		DestinationReference     string `json:"destinationReference"`
		PolicyMode               string `json:"policyMode"`
		RequiresApproval         bool   `json:"requiresApproval"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.RequestProviderWithdrawal(c, usecases.ProviderWithdrawalInput{
		Meta: h.meta(c, req.base), OrganizationID: req.OrganizationID,
		EarningAccountID:         req.EarningAccountID,
		OrganizationMembershipID: req.OrganizationMembershipID, ProviderID: req.ProviderID,
		AmountMinor: req.AmountMinor, Currency: req.Currency,
		DestinationReference: req.DestinationReference, PolicyMode: req.PolicyMode,
		RequiresApproval: req.RequiresApproval, ExpectedVersion: req.ExpectedVersion})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}

func (h *Handler) approveProviderWithdrawal(c *gin.Context) { h.decideProviderWithdrawal(c, true) }
func (h *Handler) rejectProviderWithdrawal(c *gin.Context)  { h.decideProviderWithdrawal(c, false) }

func (h *Handler) decideProviderWithdrawal(c *gin.Context, approve bool) {
	var req struct {
		base
		ApprovalReference string `json:"approvalReference"`
		ReasonCode        string `json:"reasonCode"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.DecideProviderWithdrawal(c, h.meta(c, req.base), req.OrganizationID,
		c.Param("id"), req.ExpectedVersion, approve, req.ApprovalReference, req.ReasonCode)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}

func (h *Handler) replayProviderWithdrawal(c *gin.Context) {
	var req struct{ base }
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	if err := h.service.ReplayProviderWithdrawal(c, h.meta(c, req.base), req.OrganizationID,
		c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	respond(c, gin.H{"status": "APPROVED"})
}

func (h *Handler) getProviderWithdrawal(c *gin.Context) {
	organizationID := c.Param("organizationId")
	value, err := h.service.GetProviderWithdrawal(c, h.readMeta(c, organizationID),
		organizationID, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}

func (h *Handler) providerStatement(c *gin.Context) {
	var req struct {
		base
		EarningAccountID string    `json:"earningAccountId"`
		PeriodStart      time.Time `json:"periodStart"`
		PeriodEnd        time.Time `json:"periodEnd"`
		Limit            int       `json:"limit"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	if strings.TrimSpace(req.OrganizationID) == "" || strings.TrimSpace(req.EarningAccountID) == "" {
		c.JSON(http.StatusBadRequest, errorBody("organization_required"))
		return
	}
	value, err := h.service.GenerateProviderStatement(c, usecases.ProviderStatementInput{
		Meta: h.meta(c, req.base), OrganizationID: req.OrganizationID,
		EarningAccountID: req.EarningAccountID, From: req.PeriodStart, To: req.PeriodEnd,
		Limit: req.Limit})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
