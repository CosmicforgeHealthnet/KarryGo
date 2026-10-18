package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
	"cosmicforge/logistics/services/api-gateway/internal/features/partner/webhooks"
	"cosmicforge/logistics/services/api-gateway/internal/features/partneraccount"
	"cosmicforge/logistics/services/api-gateway/internal/features/partnerauth"
	"cosmicforge/logistics/services/api-gateway/internal/pkg/pricing"
)

// Handler holds the Partner API route handlers. Delivery routes are backed by the
// delivery service; the quote route uses the pricing engine; webhook routes are
// backed by the webhook service (or return 501 when webhooks are not configured).
type Handler struct {
	deliveries      *delivery.Service
	webhooks        *webhooks.Service
	accountService  *partneraccount.Service // ADD THIS
	pricing         pricing.Config
}

// NewHandler constructs a Partner API handler. webhooks may be nil, in which case
// the webhook routes return 501 not_implemented. pricingCfg is the fare engine config.
func NewHandler(deliveries *delivery.Service, webhookSvc *webhooks.Service, accountSvc *partneraccount.Service, pricingCfg pricing.Config) *Handler {
	return &Handler{
		deliveries:     deliveries,
		webhooks:       webhookSvc,
		accountService: accountSvc,
		pricing:        pricingCfg,
	}
}

// GetQuote (POST /quotes) lives in quote_handler.go.

// --- delivery (implemented) -------------------------------------------------

// CreateDelivery handles POST /deliveries.
func (h *Handler) CreateDelivery(c *gin.Context) {
	var req delivery.DeliveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondInvalidJSON(c)
		return
	}
	status, body, err := h.deliveries.CreateDelivery(c.Request.Context(), partnerauth.PartnerID(c), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Data(status, "application/json; charset=utf-8", body)
}

// ListDeliveries handles GET /deliveries.
func (h *Handler) ListDeliveries(c *gin.Context) {
	page := atoiDefault(c.Query("page"), 0)
	limit := atoiDefault(c.Query("limit"), 0)
	resp, err := h.deliveries.ListDeliveries(c.Request.Context(), partnerauth.PartnerID(c), page, limit, c.Query("status"))
	if err != nil {
		respondError(c, err)
		return
	}
	respondData(c, http.StatusOK, resp)
}

// GetDelivery handles GET /deliveries/:id (UUID or booking_ref).
func (h *Handler) GetDelivery(c *gin.Context) {
	detail, err := h.deliveries.GetDelivery(c.Request.Context(), partnerauth.PartnerID(c), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	respondData(c, http.StatusOK, detail)
}

// CancelDelivery handles DELETE /deliveries/:id.
func (h *Handler) CancelDelivery(c *gin.Context) {
	resp, err := h.deliveries.CancelDelivery(c.Request.Context(), partnerauth.PartnerID(c), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	respondData(c, http.StatusOK, resp)
}

// --- webhook log endpoints (new) --------------------------------------------

// ListWebhookLogs handles GET /webhook-logs - returns all deliveries for the partner.
func (h *Handler) ListWebhookLogs(c *gin.Context) {
	if h.webhooks == nil {
		notImplemented(c)
		return
	}
	page := atoiDefault(c.Query("page"), 1)
	limit := atoiDefault(c.Query("limit"), 20)
	resp, err := h.webhooks.ListAllDeliveries(c.Request.Context(), partnerauth.PartnerID(c), page, limit)
	if err != nil {
		respondError(c, err)
		return
	}
	respondData(c, http.StatusOK, resp)
}

// GetWebhookLog handles GET /webhook-logs/:id - returns a single delivery detail.
func (h *Handler) GetWebhookLog(c *gin.Context) {
	if h.webhooks == nil {
		notImplemented(c)
		return
	}
	detail, err := h.webhooks.GetDeliveryByID(c.Request.Context(), partnerauth.PartnerID(c), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	respondData(c, http.StatusOK, detail)
}

// --- API key management endpoints (NEW) -------------------------------------

// CreateAPIKey handles POST /api-keys - generates a new API key.
func (h *Handler) CreateAPIKey(c *gin.Context) {
	if h.accountService == nil {
		notImplemented(c)
		return
	}
	var req partneraccount.CreateKeyInput
	if err := c.ShouldBindJSON(&req); err != nil {
		respondInvalidJSON(c)
		return
	}
	partnerID := partnerauth.PartnerID(c)
	// Get account to check plan
	account, err := h.accountService.Me(c.Request.Context(), partnerID)
	if err != nil {
		respondError(c, err)
		return
	}
	issued, err := h.accountService.CreateKey(c.Request.Context(), partnerID, account.Plan, req)
	if err != nil {
		respondError(c, err)
		return
	}
	respondData(c, http.StatusCreated, issued)
}

// ListAPIKeys handles GET /api-keys - lists all API keys for the partner.
func (h *Handler) ListAPIKeys(c *gin.Context) {
	if h.accountService == nil {
		notImplemented(c)
		return
	}
	keys, err := h.accountService.ListKeys(c.Request.Context(), partnerauth.PartnerID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	respondData(c, http.StatusOK, keys)
}

// RevokeAPIKey handles DELETE /api-keys/:id - revokes an API key.
func (h *Handler) RevokeAPIKey(c *gin.Context) {
	if h.accountService == nil {
		notImplemented(c)
		return
	}
	err := h.accountService.RevokeKey(c.Request.Context(), partnerauth.PartnerID(c), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	respondData(c, http.StatusOK, gin.H{"id": c.Param("id"), "revoked": true})
}

// Webhook handlers live in webhook_handler.go.

// --- helpers ----------------------------------------------------------------

func notImplemented(c *gin.Context) {
	partnerauth.WriteError(c, http.StatusNotImplemented, "not_implemented",
		"This Partner API endpoint is not implemented yet.")
}

func respondData(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"success": true, "data": data})
}

func respondInvalidJSON(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
		"success": false,
		"error": gin.H{
			"code":    "validation_error",
			"message": "Request body is not valid JSON.",
			"fields":  []delivery.FieldError{},
		},
	})
}

func respondError(c *gin.Context, err error) {
	var apiErr *delivery.APIError
	if !errors.As(err, &apiErr) {
		apiErr = delivery.ErrInternal()
	}
	fields := apiErr.Fields
	if fields == nil {
		fields = []delivery.FieldError{}
	}
	c.AbortWithStatusJSON(apiErr.Status, gin.H{
		"success": false,
		"error": gin.H{
			"code":    apiErr.Code,
			"message": apiErr.Message,
			"fields":  fields,
		},
	})
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}