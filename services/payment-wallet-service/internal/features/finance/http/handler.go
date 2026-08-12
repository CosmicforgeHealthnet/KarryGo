package http

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/repositories"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/usecases"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/financeauth"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const nonceHeader = "X-Service-Nonce"

type Handler struct{ service *usecases.Service }

func NewHandler(service *usecases.Service) *Handler { return &Handler{service: service} }

// RegisterPrivateRoutes exposes versioned, HMAC-authenticated finance commands.
func RegisterPrivateRoutes(group *gin.RouterGroup, service *usecases.Service,
	verifier *serviceauth.Verifier, allowedCallers []string) {
	h := NewHandler(service)
	allowed := map[string]bool{}
	for _, caller := range allowedCallers {
		allowed[strings.TrimSpace(caller)] = true
	}
	group.Use(serviceauth.Middleware(verifier), func(c *gin.Context) {
		raw, _ := c.Get("service_name")
		name, _ := raw.(string)
		if !allowed[name] {
			c.AbortWithStatusJSON(http.StatusForbidden, errorBody("caller_not_allowed"))
			return
		}
		nonce := strings.TrimSpace(c.GetHeader(nonceHeader))
		if nonce == "" || len(nonce) > 128 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errorBody("replay_nonce_required"))
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, errorBody("invalid_body"))
			return
		}
		c.Request.Body = io.NopCloser(strings.NewReader(string(body)))
		sum := sha256.Sum256(append([]byte(c.Request.Method+"\x00"+c.Request.URL.RequestURI()+"\x00"), body...))
		if err := repositories.ClaimServiceNonce(c.Request.Context(), service.LedgerDB(), name, nonce, hex.EncodeToString(sum[:]), time.Now().UTC().Add(10*time.Minute)); err != nil {
			c.AbortWithStatusJSON(http.StatusConflict, errorBody("replayed_request"))
			return
		}
		c.Next()
	})
	group.POST("/wallets/ensure", h.ensureWallet)
	group.POST("/wallets/:id/status", h.setWalletStatus)
	group.GET("/organizations/:organizationId/wallets/:currency", h.getWallet)
	group.GET("/wallets/:id", h.getWalletByID)
	group.POST("/earning-accounts/ensure", h.ensureEarningAccount)
	group.GET("/earning-accounts/:id", h.getEarningAccount)
	group.POST("/subscription-invoices", h.createSubscriptionInvoice)
	group.POST("/plans", h.createPlan)
	group.POST("/commission-rules", h.createCommissionRule)
	group.POST("/invoice-payments/confirm", h.confirmInvoicePayment)
	group.GET("/organizations/:organizationId/invoices/:id", h.getInvoice)
	group.POST("/earning-allocations", h.postAllocation)
	group.POST("/holds", h.createHold)
	group.POST("/holds/:id/release", h.releaseHold)
	group.GET("/organizations/:organizationId/holds/:id", h.getHold)
	group.POST("/settlements", h.requestSettlement)
	group.POST("/settlements/:id/approve", h.approveSettlement)
	group.GET("/organizations/:organizationId/settlements/:id", h.getSettlement)
	group.POST("/refunds", h.requestRefund)
	group.POST("/refunds/:id/approve", h.approveRefund)
	group.GET("/organizations/:organizationId/refunds/:id", h.getRefund)
	group.POST("/reversals", h.requestReversal)
	group.POST("/reversals/:id/approve", h.approveReversal)
	group.POST("/reversals/:id/reject", h.rejectReversal)
	group.GET("/organizations/:organizationId/reversals", h.listReversals)
	group.GET("/organizations/:organizationId/reversals/:id", h.getReversal)
	group.GET("/organizations/:organizationId/reversals/:id/history", h.reversalHistory)
	group.POST("/job-payments", h.recordJobPayment)
	group.GET("/job-payments/lookup", h.lookupJobPayment)
	group.GET("/organizations/:organizationId/payments/:id", h.getPayment)
	group.GET("/operations/:operationType/:id", h.readOperation)
	group.GET("/operations/:operationType/by-idempotency", h.readOperationByKey)
	group.POST("/operations/:operationType/:id/replay", h.replayOperation)
	group.POST("/reconciliation", h.reconcile)
	group.POST("/statements", h.statement)
	registerProviderRoutes(group, h)
}

func errorBody(code string) gin.H       { return gin.H{"success": false, "error": gin.H{"code": code}} }
func respond(c *gin.Context, value any) { c.JSON(http.StatusOK, gin.H{"success": true, "data": value}) }
func fail(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"
	switch {
	case errors.Is(err, financeauth.ErrForbidden), errors.Is(err, financeauth.ErrSelfApproval):
		status = http.StatusForbidden
		code = "forbidden"
	case errors.Is(err, repositories.ErrNotFound):
		status = http.StatusNotFound
		code = "not_found"
	case errors.Is(err, repositories.ErrConflict), errors.Is(err, repositories.ErrVersionMismatch), errors.Is(err, usecases.ErrIdempotencyReuse):
		status = http.StatusConflict
		code = "conflict"
	case errors.Is(err, usecases.ErrInvalid), errors.Is(err, usecases.ErrPricingMissing),
		errors.Is(err, usecases.ErrInsufficientFunds), errors.Is(err, usecases.ErrReversalNotReversible):
		status = http.StatusUnprocessableEntity
		code = "validation_failed"
	}
	c.JSON(status, errorBody(code))
}

type base struct {
	PayloadVersion  int    `json:"payloadVersion"`
	OrganizationID  string `json:"organizationId"`
	IdempotencyKey  string `json:"idempotencyKey"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handler) meta(c *gin.Context, b base) usecases.Meta {
	raw, _ := c.Get("service_name")
	caller, _ := raw.(string)
	return usecases.Meta{Actor: financeauth.Actor{Type: financeauth.ActorService, ID: caller, OrganizationID: b.OrganizationID, CorrelationID: c.GetHeader("X-Correlation-ID"), RequestID: c.GetHeader("X-Request-ID")}, IdempotencyKey: b.IdempotencyKey, CorrelationID: c.GetHeader("X-Correlation-ID"), CausationID: c.GetHeader("X-Causation-ID"), RequestID: c.GetHeader("X-Request-ID")}
}
func (h *Handler) readMeta(c *gin.Context, organizationID string) usecases.Meta {
	return h.meta(c, base{OrganizationID: organizationID})
}
func (h *Handler) authorizeRead(c *gin.Context, organizationID string, permissions ...string) bool {
	if err := h.service.AuthorizeRead(c, h.readMeta(c, organizationID), permissions...); err != nil {
		fail(c, err)
		return false
	}
	return true
}
func bind(c *gin.Context, value any) bool {
	if err := c.ShouldBindJSON(value); err != nil {
		c.JSON(http.StatusBadRequest, errorBody("invalid_body"))
		return false
	}
	return true
}
func validVersion(c *gin.Context, v int) bool {
	if v != 1 {
		c.JSON(http.StatusBadRequest, errorBody("unsupported_payload_version"))
		return false
	}
	return true
}

func (h *Handler) ensureWallet(c *gin.Context) {
	var req struct {
		base
		Currency string `json:"currency"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.EnsureOrganizationWallet(c, h.meta(c, req.base), req.OrganizationID, req.Currency)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) getWallet(c *gin.Context) {
	organizationID := c.Param("organizationId")
	if !h.authorizeRead(c, organizationID, financeauth.PermWalletView) {
		return
	}
	value, err := h.service.Accounts().GetWallet(c, nil, organizationID, strings.ToUpper(c.Param("currency")))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, gin.H{"id": value.ID, "organizationId": value.OrganizationID, "currency": value.Currency, "status": value.Status, "restrictions": value.Restrictions, "version": value.Version, "updatedAt": value.UpdatedAt})
}
func (h *Handler) getWalletByID(c *gin.Context) {
	organizationID := strings.TrimSpace(c.Query("organizationId"))
	if organizationID == "" {
		c.JSON(http.StatusBadRequest, errorBody("organization_required"))
		return
	}
	if !h.authorizeRead(c, organizationID, financeauth.PermWalletView) {
		return
	}
	value, err := h.service.Accounts().GetWalletByID(c, nil, organizationID, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, gin.H{"id": value.ID, "organizationId": value.OrganizationID, "currency": value.Currency, "status": value.Status, "restrictions": value.Restrictions, "version": value.Version, "updatedAt": value.UpdatedAt})
}
func (h *Handler) setWalletStatus(c *gin.Context) {
	var req struct {
		base
		Status       string   `json:"status"`
		Restrictions []string `json:"restrictions"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.SetWalletStatus(c, h.meta(c, req.base), req.OrganizationID, c.Param("id"), req.Status, req.ExpectedVersion, req.Restrictions)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) ensureEarningAccount(c *gin.Context) {
	var req struct {
		base
		ProviderService   string `json:"providerService"`
		ProviderReference string `json:"providerReference"`
		Currency          string `json:"currency"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.EnsureEarningAccount(c, h.meta(c, req.base), req.ProviderService, req.ProviderReference, req.OrganizationID, req.Currency)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) getEarningAccount(c *gin.Context) {
	organizationID := strings.TrimSpace(c.Query("organizationId"))
	if organizationID == "" || !h.authorizeRead(c, organizationID, financeauth.PermWalletView) {
		if organizationID == "" {
			c.JSON(http.StatusBadRequest, errorBody("organization_required"))
		}
		return
	}
	value, err := h.service.Accounts().GetEarningAccountByID(c, nil, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	if value.OrganizationID != organizationID {
		fail(c, repositories.ErrNotFound)
		return
	}
	respond(c, gin.H{"id": value.ID, "providerReference": value.ProviderReference, "providerService": value.ProviderService, "organizationId": value.OrganizationID, "currency": value.Currency, "status": value.Status, "restrictions": value.Restrictions, "version": value.Version, "updatedAt": value.UpdatedAt})
}
func (h *Handler) createSubscriptionInvoice(c *gin.Context) {
	var req struct {
		base
		PlanCode string `json:"planCode"`
		DueDays  int    `json:"dueDays"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	sub, invoice, err := h.service.CreateSubscriptionInvoice(c, usecases.SubscribeInput{Meta: h.meta(c, req.base), OrganizationID: req.OrganizationID, PlanCode: req.PlanCode, DueDays: req.DueDays})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, gin.H{"subscription": sub, "invoice": invoice})
}
func (h *Handler) createPlan(c *gin.Context) {
	var req struct {
		base
		Code            string `json:"code"`
		Name            string `json:"name"`
		Description     string `json:"description"`
		Currency        string `json:"currency"`
		PriceMinor      *int64 `json:"priceMinor"`
		BillingInterval string `json:"billingInterval"`
		IntervalCount   int    `json:"intervalCount"`
		GraceDays       int    `json:"graceDays"`
		Status          string `json:"status"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.CreatePlan(c, h.meta(c, req.base), &models.SubscriptionPlan{Code: req.Code, Name: req.Name, Description: req.Description, Currency: req.Currency, PriceMinor: req.PriceMinor, BillingInterval: req.BillingInterval, IntervalCount: req.IntervalCount, GraceDays: req.GraceDays, Status: req.Status})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) createCommissionRule(c *gin.Context) {
	var req struct {
		base
		ServiceType          string     `json:"serviceType"`
		Currency             string     `json:"currency"`
		CommissionBPS        int        `json:"commissionBps"`
		FleetFeeBPS          int        `json:"fleetFeeBps"`
		ProviderShareBPS     int        `json:"providerShareBps"`
		OrganizationShareBPS int        `json:"organizationShareBps"`
		Status               string     `json:"status"`
		EffectiveFrom        time.Time  `json:"effectiveFrom"`
		EffectiveTo          *time.Time `json:"effectiveTo"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.CreateCommissionRule(c, h.meta(c, req.base), &models.CommissionRule{OrganizationID: req.OrganizationID, ServiceType: req.ServiceType, Currency: req.Currency, CommissionBPS: req.CommissionBPS, FleetFeeBPS: req.FleetFeeBPS, ProviderShareBPS: req.ProviderShareBPS, OrganizationShareBPS: req.OrganizationShareBPS, Status: req.Status, EffectiveFrom: req.EffectiveFrom, EffectiveTo: req.EffectiveTo})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) getInvoice(c *gin.Context) {
	if !h.authorizeRead(c, c.Param("organizationId"), financeauth.PermTransactionView) {
		return
	}
	value, err := h.service.Billing().GetInvoice(c, nil, c.Param("organizationId"), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}

func (h *Handler) confirmInvoicePayment(c *gin.Context) {
	var req struct {
		base
		InvoiceID         string `json:"invoiceId"`
		AmountMinor       int64  `json:"amountMinor"`
		Currency          string `json:"currency"`
		Method            string `json:"method"`
		ExternalReference string `json:"externalReference"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.ConfirmInvoicePayment(c, usecases.InvoicePaymentInput{Meta: h.meta(c, req.base), OrganizationID: req.OrganizationID, InvoiceID: req.InvoiceID, AmountMinor: req.AmountMinor, Currency: req.Currency, Method: req.Method, ExternalReference: req.ExternalReference})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) postAllocation(c *gin.Context) {
	var req struct {
		base
		SourceService     string `json:"sourceService"`
		JobReference      string `json:"jobReference"`
		ServiceType       string `json:"serviceType"`
		ProviderReference string `json:"providerReference"`
		ProviderService   string `json:"providerService"`
		Currency          string `json:"currency"`
		GrossMinor        int64  `json:"grossMinor"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.PostEarningAllocation(c, usecases.AllocationInput{Meta: h.meta(c, req.base), OrganizationID: req.OrganizationID, SourceService: req.SourceService, JobReference: req.JobReference, ServiceType: req.ServiceType, ProviderReference: req.ProviderReference, ProviderService: req.ProviderService, Currency: req.Currency, GrossMinor: req.GrossMinor})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) createHold(c *gin.Context) {
	var req struct {
		base
		SubjectType        string     `json:"subjectType"`
		SubjectID          string     `json:"subjectId"`
		ReasonCode         string     `json:"reasonCode"`
		Reason             string     `json:"reason"`
		AmountMinor        int64      `json:"amountMinor"`
		Currency           string     `json:"currency"`
		AuthorityService   string     `json:"authorityService"`
		AuthorityReference string     `json:"authorityReference"`
		ReferenceVerified  bool       `json:"referenceVerified"`
		ExpiresAt          *time.Time `json:"expiresAt"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.CreateHold(c, usecases.HoldInput{Meta: h.meta(c, req.base), OrganizationID: req.OrganizationID, SubjectType: req.SubjectType, SubjectID: req.SubjectID, ReasonCode: req.ReasonCode, Reason: req.Reason, AmountMinor: req.AmountMinor, Currency: req.Currency, AuthorityService: req.AuthorityService, AuthorityReference: req.AuthorityReference, ReferenceVerified: req.ReferenceVerified, ExpiresAt: req.ExpiresAt, ExpectedVersion: req.ExpectedVersion})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) releaseHold(c *gin.Context) {
	var req base
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.ReleaseHold(c, h.meta(c, req), req.OrganizationID, c.Param("id"), req.ExpectedVersion, models.HoldReleased)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) getHold(c *gin.Context) {
	if !h.authorizeRead(c, c.Param("organizationId"), financeauth.PermTransactionView) {
		return
	}
	value, err := h.service.Holds().Get(c, nil, c.Param("organizationId"), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) requestSettlement(c *gin.Context) {
	var req struct {
		base
		BeneficiaryType      string `json:"beneficiaryType"`
		BeneficiaryReference string `json:"beneficiaryReference"`
		EarningAccountID     string `json:"earningAccountId"`
		WalletID             string `json:"walletId"`
		Currency             string `json:"currency"`
		Limit                int    `json:"limit"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.RequestSettlement(c, usecases.SettlementInput{Meta: h.meta(c, req.base), OrganizationID: req.OrganizationID, BeneficiaryType: req.BeneficiaryType, BeneficiaryReference: req.BeneficiaryReference, EarningAccountID: req.EarningAccountID, WalletID: req.WalletID, Currency: req.Currency, ExpectedVersion: req.ExpectedVersion, Limit: req.Limit})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) approveSettlement(c *gin.Context) {
	var req base
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.ApproveSettlement(c, h.meta(c, req), req.OrganizationID, c.Param("id"), req.ExpectedVersion)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) getSettlement(c *gin.Context) {
	if !h.authorizeRead(c, c.Param("organizationId"), financeauth.PermTransactionView) {
		return
	}
	value, err := h.service.Operations().GetSettlementBatch(c, nil, c.Param("organizationId"), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) requestRefund(c *gin.Context) {
	var req struct {
		base
		OriginalJournalID string `json:"originalJournalId"`
		AmountMinor       int64  `json:"amountMinor"`
		ReasonCode        string `json:"reasonCode"`
		Reason            string `json:"reason"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.RequestRefund(c, usecases.RefundInput{Meta: h.meta(c, req.base), OrganizationID: req.OrganizationID, OriginalJournalID: req.OriginalJournalID, AmountMinor: req.AmountMinor, ReasonCode: req.ReasonCode, Reason: req.Reason, ExpectedVersion: req.ExpectedVersion})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) approveRefund(c *gin.Context) {
	var req base
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.ApproveRefund(c, h.meta(c, req), req.OrganizationID, c.Param("id"), req.ExpectedVersion)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) getRefund(c *gin.Context) {
	if !h.authorizeRead(c, c.Param("organizationId"), financeauth.PermTransactionView) {
		return
	}
	value, err := h.service.Operations().GetRefund(c, nil, c.Param("organizationId"), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) readOperation(c *gin.Context) {
	org := c.Query("organizationId")
	if strings.TrimSpace(org) == "" {
		c.JSON(http.StatusBadRequest, errorBody("organization_required"))
		return
	}
	if !h.authorizeRead(c, org, financeauth.PermTransactionView) {
		return
	}
	switch strings.ToUpper(c.Param("operationType")) {
	case "WALLET":
		value, err := h.service.Accounts().GetWalletByID(c, nil, org, c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, value)
	case "EARNING_ACCOUNT":
		value, err := h.service.Accounts().GetEarningAccountByID(c, nil, c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		if value.OrganizationID != org {
			fail(c, financeauth.ErrForbidden)
			return
		}
		respond(c, value)
	case "SUBSCRIPTION":
		value, err := h.service.Billing().GetSubscription(c, nil, org, c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, value)
	case "INVOICE", "PAYMENT_VERIFICATION":
		value, err := h.service.Billing().GetInvoice(c, nil, org, c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, value)
	case "COMMISSION", "EARNING", "EARNING_ALLOCATION":
		value, err := h.service.Marketplace().GetAllocation(c, nil, org, c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, value)
	case "SETTLEMENT":
		value, err := h.service.Operations().GetSettlementBatch(c, nil, org, c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, value)
	case "REFUND":
		value, err := h.service.Operations().GetRefund(c, nil, org, c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, value)
	case "HOLD":
		value, err := h.service.Holds().Get(c, nil, org, c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, value)
	case "REVERSAL":
		value, err := h.service.GetReversal(c, h.readMeta(c, org), org, c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, reversalView(value))
	case "PAYMENT_VERIFICATION_LOOKUP":
		view, err := h.service.LookupJobPayment(c, h.readMeta(c, org),
			org, c.Query("referenceType"), c.Param("id"))
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, view)
	default:
		c.JSON(http.StatusBadRequest, errorBody("unsupported_operation"))
	}
}
func (h *Handler) readOperationByKey(c *gin.Context) {
	org, key := strings.TrimSpace(c.Query("organizationId")), strings.TrimSpace(c.Query("idempotencyKey"))
	if org == "" || key == "" {
		c.JSON(http.StatusBadRequest, errorBody("organization_and_key_required"))
		return
	}
	if !h.authorizeRead(c, org, financeauth.PermTransactionView) {
		return
	}
	var value any
	var err error
	switch strings.ToUpper(c.Param("operationType")) {
	case "SETTLEMENT", "SETTLEMENT_REQUEST":
		value, err = h.service.Operations().FindSettlementBatchByKey(c, nil, org, key)
	case "REFUND", "REFUND_REQUEST":
		value, err = h.service.Operations().FindRefundByKey(c, nil, org, key)
	case "HOLD", "HOLD_CREATE":
		value, err = h.service.Holds().FindByIdempotencyKey(c, nil, org, key)
	case "EARNING_ALLOCATION", "COMMISSION_POST", "FLEET_FEE_POST":
		value, err = h.service.Marketplace().FindAllocationByIdempotencyKey(c, nil, org, key)
	case "PROVIDER_ALLOCATION":
		value, err = h.service.FindProviderAllocationByKey(c, h.readMeta(c, org), org, key)
	case "SUBSCRIPTION_INVOICE":
		value, err = h.service.Billing().FindInvoiceByIdempotencyKey(c, nil, org, key)
	case "REVERSAL", "REVERSAL_REQUEST":
		// The durable reversal row — not the compensating journal — is the
		// authoritative answer to "what happened to my reversal request?".
		reversal, readErr := h.service.FindReversalByKey(c, h.readMeta(c, org), org, key)
		if readErr != nil {
			err = readErr
		} else {
			value = reversalView(reversal)
		}
	default:
		c.JSON(http.StatusBadRequest, errorBody("unsupported_operation"))
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}

// reversalView is the allowlisted reversal response. It carries the operation's
// own state and references, never a balance or a ledger account.
func reversalView(value *models.Reversal) gin.H {
	return gin.H{
		"id": value.ID, "organizationId": value.OrganizationID,
		"originalJournalId": value.OriginalJournalID,
		"reasonCode":        value.ReasonCode,
		"amountMinor":       value.AmountMinor, "currency": value.Currency,
		"status": value.Status, "attempts": value.Attempts,
		"externalOperationReference": value.ExternalOperationReference,
		"resultingJournalId":         value.ResultingJournalID,
		"approvalReference":          value.ApprovalReference,
		"approvedByActorType":        value.ApprovedByActorType,
		"approvedAt":                 value.ApprovedAt,
		"createdByActorType":         value.CreatedByActorType,
		"correlationId":              value.CorrelationID,
		"causationId":                value.CausationID,
		"expectedVersion":            value.ExpectedVersion,
		"safeErrorCode":              value.SafeErrorCode,
		"version":                    value.Version,
		"createdAt":                  value.CreatedAt, "updatedAt": value.UpdatedAt,
		"completedAt": value.CompletedAt,
	}
}

func (h *Handler) requestReversal(c *gin.Context) {
	var req struct {
		base
		OriginalJournalID string `json:"originalJournalId"`
		AmountMinor       int64  `json:"amountMinor"`
		Currency          string `json:"currency"`
		ReasonCode        string `json:"reasonCode"`
		Reason            string `json:"reason"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.RequestReversal(c, usecases.ReversalRequest{
		Meta:              h.meta(c, req.base),
		OrganizationID:    req.OrganizationID,
		OriginalJournalID: req.OriginalJournalID,
		AmountMinor:       req.AmountMinor,
		Currency:          req.Currency,
		ReasonCode:        req.ReasonCode,
		Reason:            req.Reason,
		ExpectedVersion:   req.ExpectedVersion,
	})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, reversalView(value))
}

func (h *Handler) approveReversal(c *gin.Context) {
	var req base
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.ApproveReversal(c, h.meta(c, req), req.OrganizationID,
		c.Param("id"), req.ExpectedVersion)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, reversalView(value))
}

func (h *Handler) rejectReversal(c *gin.Context) {
	var req struct {
		base
		ReasonCode string `json:"reasonCode"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.RejectReversal(c, h.meta(c, req.base), req.OrganizationID,
		c.Param("id"), req.ExpectedVersion, req.ReasonCode)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, reversalView(value))
}

func (h *Handler) getReversal(c *gin.Context) {
	organizationID := c.Param("organizationId")
	value, err := h.service.GetReversal(c, h.readMeta(c, organizationID), organizationID, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, reversalView(value))
}

func (h *Handler) listReversals(c *gin.Context) {
	organizationID := c.Param("organizationId")
	limit, _ := strconv.Atoi(c.Query("limit"))
	values, err := h.service.ListReversals(c, h.readMeta(c, organizationID), organizationID,
		repositories.ListFilter{Status: strings.ToUpper(c.Query("status")),
			OriginalJournalID: c.Query("originalJournalId"), Limit: limit})
	if err != nil {
		fail(c, err)
		return
	}
	views := make([]gin.H, 0, len(values))
	for index := range values {
		views = append(views, reversalView(&values[index]))
	}
	respond(c, gin.H{"reversals": views})
}

func (h *Handler) reversalHistory(c *gin.Context) {
	organizationID := c.Param("organizationId")
	entries, err := h.service.ReversalHistory(c, h.readMeta(c, organizationID),
		organizationID, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, gin.H{"history": entries})
}

func (h *Handler) recordJobPayment(c *gin.Context) {
	var req struct {
		base
		SourceService            string     `json:"sourceService"`
		DispatchRequestReference string     `json:"dispatchRequestReference"`
		TripReference            string     `json:"tripReference"`
		BookingReference         string     `json:"bookingReference"`
		PaymentReference         string     `json:"paymentReference"`
		AmountMinor              int64      `json:"amountMinor"`
		Currency                 string     `json:"currency"`
		Status                   string     `json:"status"`
		PaidAt                   *time.Time `json:"paidAt"`
		RefundState              string     `json:"refundState"`
		ReversalState            string     `json:"reversalState"`
		OperationReference       string     `json:"operationReference"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.RecordJobPayment(c, usecases.JobPaymentInput{
		Meta: h.meta(c, req.base), OrganizationID: req.OrganizationID,
		SourceService: req.SourceService, DispatchRequestReference: req.DispatchRequestReference,
		TripReference: req.TripReference, BookingReference: req.BookingReference,
		PaymentReference: req.PaymentReference, AmountMinor: req.AmountMinor,
		Currency: req.Currency, Status: req.Status, PaidAt: req.PaidAt,
		RefundState: req.RefundState, ReversalState: req.ReversalState,
		OperationReference: req.OperationReference,
	})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, gin.H{"id": value.ID, "organizationId": value.OrganizationID,
		"status": value.Status, "version": value.Version})
}

// lookupJobPayment is the authoritative job payment verification another
// service calls before it allows money to move.
func (h *Handler) lookupJobPayment(c *gin.Context) {
	organizationID := strings.TrimSpace(c.Query("organizationId"))
	if organizationID == "" {
		c.JSON(http.StatusBadRequest, errorBody("organization_required"))
		return
	}
	view, err := h.service.LookupJobPayment(c, h.readMeta(c, organizationID), organizationID,
		c.Query("referenceType"), c.Query("reference"))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, view)
}
func (h *Handler) getPayment(c *gin.Context) {
	if !h.authorizeRead(c, c.Param("organizationId"), financeauth.PermTransactionView) {
		return
	}
	value, err := h.service.Billing().GetPayment(c, nil, c.Param("organizationId"), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) replayOperation(c *gin.Context) {
	var req base
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	if err := h.service.ReplayOperation(c, h.meta(c, req), c.Param("operationType"), c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	respond(c, gin.H{"replayed": true})
}
func (h *Handler) reconcile(c *gin.Context) {
	var req struct {
		base
		Scope string `json:"scope"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.Reconcile(c, h.meta(c, req.base), req.Scope)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
func (h *Handler) statement(c *gin.Context) {
	var req struct {
		base
		OwnerType      string    `json:"ownerType"`
		OwnerReference string    `json:"ownerReference"`
		Currency       string    `json:"currency"`
		StatementType  string    `json:"statementType"`
		PeriodStart    time.Time `json:"periodStart"`
		PeriodEnd      time.Time `json:"periodEnd"`
	}
	if !bind(c, &req) || !validVersion(c, req.PayloadVersion) {
		return
	}
	value, err := h.service.GenerateStatement(c, h.meta(c, req.base), req.OrganizationID, req.OwnerType, req.OwnerReference, req.Currency, req.StatementType, req.PeriodStart, req.PeriodEnd)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, value)
}
