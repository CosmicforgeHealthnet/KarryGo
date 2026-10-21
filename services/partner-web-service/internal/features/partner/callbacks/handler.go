package callbacks

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/partner-web-service/internal/features/partner/billing"
	"cosmicforge/logistics/services/partner-web-service/internal/features/partner/delivery"
)

// maxCallbackBody bounds internal callback payloads (1 MiB).
const maxCallbackBody = 1 << 20

type handler struct {
	dispatch DispatchProcessor
	billing  ChargeResultHandler
}

// dispatchCallback returns a handler that processes one dispatch lifecycle
// callback through the live webhooks pipeline: find the partner booking by
// dispatch booking id, update status (+ rider snapshot on trip-started), append
// the timeline source, and enqueue partner webhook deliveries. Unknown/non-
// partner bookings are answered with 200 skipped=true to avoid retry storms.
func (h *handler) dispatchCallback(topic string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.dispatch == nil {
			writeError(c, http.StatusServiceUnavailable, "webhooks_unavailable",
				"Webhook processing is not configured on this gateway instance.")
			return
		}
		raw, err := io.ReadAll(io.LimitReader(c.Request.Body, maxCallbackBody))
		if err != nil {
			writeError(c, http.StatusBadRequest, "invalid_body", "Could not read the request body.")
			return
		}
		var probe struct {
			BookingID string `json:"booking_id"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			writeError(c, http.StatusBadRequest, "invalid_json", "Request body is not valid JSON.")
			return
		}
		if strings.TrimSpace(probe.BookingID) == "" {
			writeError(c, http.StatusBadRequest, "missing_booking_id", "booking_id is required.")
			return
		}

		matched, err := h.dispatch.ProcessDispatchEventResult(c.Request.Context(), topic, raw)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		if !matched {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"skipped": true}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"processed": true}})
	}
}

// chargeResult applies a payment-wallet charge-result callback to the matching
// partner invoice.
func (h *handler) chargeResult(c *gin.Context) {
	if h.billing == nil {
		writeError(c, http.StatusServiceUnavailable, "billing_unavailable",
			"Billing is not configured on this gateway instance.")
		return
	}
	var in billing.ChargeResultInput
	if err := c.ShouldBindJSON(&in); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", "Request body is not valid JSON.")
		return
	}
	outcome, err := h.billing.HandleChargeResult(c.Request.Context(), in)
	if err != nil {
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
			"error":   gin.H{"code": apiErr.Code, "message": apiErr.Message, "fields": fields},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": outcome})
}

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"success": false,
		"error":   gin.H{"code": code, "message": message, "fields": []delivery.FieldError{}},
	})
}
