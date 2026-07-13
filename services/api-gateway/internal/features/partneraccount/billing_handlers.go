package partneraccount

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/billing"
)

// BillingHandlers holds the Billing & Usage dashboard HTTP handlers. Kept in
// its own file/type separate from Handlers (auth/keys/profile) since billing
// is a distinct feature area with its own service dependency, but lives in
// this package because - like /profile - it's dashboard-only and
// session-authenticated, not part of the partner API key surface.
type BillingHandlers struct {
	svc *billing.Service
}

// NewBillingHandlers builds the billing handlers.
func NewBillingHandlers(svc *billing.Service) *BillingHandlers { return &BillingHandlers{svc: svc} }

// GetOverview handles GET /billing/overview (session required). This is the
// whole Billing & Usage screen in one response: balance figures, plan/status,
// this-month usage, and saved payment methods.
func (h *BillingHandlers) GetOverview(c *gin.Context) {
	overview, err := h.svc.GetOverview(c.Request.Context(), SessionPartnerID(c), PartnerPlan(c), "")
	if err != nil {
		respondAPIError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": overview})
}

// ListInvoices handles GET /billing/invoices (session required) - the Billing
// History table.
func (h *BillingHandlers) ListInvoices(c *gin.Context) {
	page := billingAtoiDefault(c.Query("page"), 0)
	limit := billingAtoiDefault(c.Query("limit"), 0)
	views, total, err := h.svc.ListInvoices(c.Request.Context(), SessionPartnerID(c), page, limit)
	if err != nil {
		respondAPIError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"invoices": views, "meta": gin.H{"total": total}},
	})
}

// AttachPaymentMethod handles POST /billing/payment-methods (session
// required). The frontend must tokenize the card via Paystack Inline/Popup
// BEFORE calling this - only the resulting authorization_code is sent here,
// never a raw card number (see billing package doc comment).
func (h *BillingHandlers) AttachPaymentMethod(c *gin.Context) {
	var in billing.AttachPaymentMethodInput
	if !bindJSON(c, &in) {
		return
	}
	view, err := h.svc.AttachPaymentMethod(c.Request.Context(), SessionPartnerID(c), in)
	if err != nil {
		respondAPIError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": view})
}

func billingAtoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
