package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/webhooks"
	"cosmicforge/logistics/services/api-gateway/internal/features/partnerauth"
)

// RegisterWebhook handles POST /webhooks. The signing_secret is returned exactly
// once here and never again.
func (h *Handler) RegisterWebhook(c *gin.Context) {
	if h.webhooks == nil {
		notImplemented(c)
		return
	}
	var in webhooks.CreateWebhookInput
	if err := c.ShouldBindJSON(&in); err != nil {
		respondInvalidJSON(c)
		return
	}
	res, err := h.webhooks.CreateWebhook(c.Request.Context(), partnerauth.PartnerID(c), in)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    res,
		"note":    "Save your signing_secret now. It is not shown again.",
	})
}

// ListWebhooks handles GET /webhooks. It never returns signing secrets.
func (h *Handler) ListWebhooks(c *gin.Context) {
	if h.webhooks == nil {
		notImplemented(c)
		return
	}
	views, err := h.webhooks.ListWebhooks(c.Request.Context(), partnerauth.PartnerID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	respondData(c, http.StatusOK, gin.H{"webhooks": views})
}

// DeleteWebhook handles DELETE /webhooks/:id (partner-scoped soft delete).
func (h *Handler) DeleteWebhook(c *gin.Context) {
	if h.webhooks == nil {
		notImplemented(c)
		return
	}
	res, err := h.webhooks.DeleteWebhook(c.Request.Context(), partnerauth.PartnerID(c), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	respondData(c, http.StatusOK, res)
}

// TestWebhook handles POST /webhooks/:id/test. It sends a real signed test event
// and only succeeds on a 2xx response from the partner endpoint.
func (h *Handler) TestWebhook(c *gin.Context) {
	if h.webhooks == nil {
		notImplemented(c)
		return
	}
	if err := h.webhooks.TestWebhook(c.Request.Context(), partnerauth.PartnerID(c), c.Param("id")); err != nil {
		respondError(c, err)
		return
	}
	respondData(c, http.StatusOK, gin.H{"id": c.Param("id"), "delivered": true})
}

// ListWebhookDeliveries handles GET /webhooks/:id/deliveries - the delivery
// attempt history (timestamp, event, status, HTTP response code, attempts) for
// one partner webhook. This is the "webhook logs" screen; it is separate from
// ListWebhooks above, which only lists registered endpoint URLs, not delivery
// attempts against them.
func (h *Handler) ListWebhookDeliveries(c *gin.Context) {
	if h.webhooks == nil {
		notImplemented(c)
		return
	}
	page := atoiDefault(c.Query("page"), 0)
	limit := atoiDefault(c.Query("limit"), 0)
	resp, err := h.webhooks.ListDeliveries(c.Request.Context(), partnerauth.PartnerID(c), c.Param("id"), page, limit)
	if err != nil {
		respondError(c, err)
		return
	}
	respondData(c, http.StatusOK, resp)
}