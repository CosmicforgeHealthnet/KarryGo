// Package operationshttp exposes the phase B10 internal ops surface: live
// operations, unmatched-booking triage, admin actions, route replay, and a
// minimal Prometheus-text /metrics endpoint. Every route except /metrics is
// mounted under /internal and HMAC-protected (see routes.go) -- this
// package is never reached by a customer or driver bearer token.
package operationshttp

import (
	nethttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	bookingusecases "cosmicforge/logistics/services/taxi-service/internal/features/booking/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct {
	bookings *bookingusecases.BookingService
}

func NewHandler(bookings *bookingusecases.BookingService) *Handler {
	return &Handler{bookings: bookings}
}

type successResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
}

func respond(c *gin.Context, status int, data interface{}) {
	c.JSON(status, successResponse{Success: true, Data: data})
}

// operatorID resolves who is performing an admin action: the request body's
// own operator_id when the calling service supplied one, otherwise the
// HMAC-verified calling service's own name -- see adminActionRequest's doc
// comment for why a specific staff identity is not always available.
func operatorID(c *gin.Context, bodyOperatorID string) string {
	if id := strings.TrimSpace(bodyOperatorID); id != "" {
		return id
	}
	return c.GetString("service_name")
}

func (h *Handler) GetLive(c *gin.Context) {
	view, err := h.bookings.ListLiveOperations(c.Request.Context(), c.Query("market_id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, view)
}

func (h *Handler) GetUnmatched(c *gin.Context) {
	filter := bookingusecases.UnmatchedFilter{
		MarketID: c.Query("market_id"), OperatingZoneID: c.Query("operating_zone_id"),
	}
	if raw := c.Query("older_than_minutes"); raw != "" {
		minutes, err := strconv.Atoi(raw)
		if err != nil || minutes < 0 {
			httpx.Abort(c, apperrors.BadRequest("'older_than_minutes' must be a non-negative integer.", nil))
			return
		}
		filter.OlderThan = time.Duration(minutes) * time.Minute
	}
	bookings, err := h.bookings.ListUnmatchedBookings(c.Request.Context(), filter)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, gin.H{"bookings": bookings})
}

func (h *Handler) PostCancel(c *gin.Context) {
	var req adminActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	booking, err := h.bookings.AdminCancelBooking(c.Request.Context(), operatorID(c, req.OperatorID), c.Param("id"), req.Reason)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, booking.Public())
}

func (h *Handler) PostForceRecover(c *gin.Context) {
	var req adminActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	booking, err := h.bookings.AdminForceRecover(c.Request.Context(), operatorID(c, req.OperatorID), c.Param("id"), req.Reason)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, booking.Public())
}

func (h *Handler) PostPauseProvider(c *gin.Context) {
	var req adminActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	state, err := h.bookings.AdminPauseProvider(c.Request.Context(), operatorID(c, req.OperatorID), c.Param("id"), req.Reason)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, gin.H{"provider_id": state.ProviderID, "state": state.Current})
}

func (h *Handler) PostReleaseHold(c *gin.Context) {
	var req adminActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	state, err := h.bookings.AdminReleaseHold(c.Request.Context(), operatorID(c, req.OperatorID), c.Param("id"), req.Reason)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, gin.H{"provider_id": state.ProviderID, "state": state.Current})
}

func (h *Handler) GetReplay(c *gin.Context) {
	replay, err := h.bookings.GetBookingReplay(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, replay)
}

// GetMetrics is deliberately public (mounted OUTSIDE the /internal HMAC
// group in routes.go) -- a Prometheus scraper is not a signed internal
// service caller, matching the standard "metrics endpoints are their own,
// separately-secured surface" convention (network-level restriction, not
// HMAC, is the real control here in a production deployment).
func (h *Handler) GetMetrics(c *gin.Context) {
	snapshot, err := h.bookings.GetMetricsSnapshot(c.Request.Context())
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.String(nethttp.StatusOK, formatPrometheusText(snapshot))
}
