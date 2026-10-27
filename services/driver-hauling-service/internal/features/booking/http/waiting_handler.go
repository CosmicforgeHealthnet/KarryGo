package bookinghttp

import (
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	bookingusecases "cosmicforge/logistics/services/hauling-service/internal/features/booking/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// WaitingAdminService may change waiting-time rates over the signed admin route.
const WaitingAdminService = "admin-backoffice-service"

// MarkArrivedAtDropoff starts the drop-off waiting clock.
func (h *BookingHandler) MarkArrivedAtDropoff(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	view, err := h.svc.MarkArrivedAtDropoff(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": view})
}

// ProviderWaiting returns a booking's waiting for its driver.
func (h *BookingHandler) ProviderWaiting(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	view, err := h.svc.ProviderWaiting(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": view})
}

// CustomerWaiting returns a booking's waiting for the customer.
func (h *BookingHandler) CustomerWaiting(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	view, err := h.svc.CustomerWaiting(c.Request.Context(), c.Param("id"), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": view})
}

type waitingPolicyRequest struct {
	FreeMinutes  int    `json:"free_minutes"`
	BlockMinutes int    `json:"block_minutes"`
	BlockFeeKobo int64  `json:"block_fee_kobo"`
	PerformedBy  string `json:"performed_by"`
	Reason       string `json:"reason"`
}

// RegisterWaitingAdminRoutes mounts GET/PUT /internal/admin/waiting-policy
// behind service signatures from WaitingAdminService. Skipped when no verifier
// is configured.
func RegisterWaitingAdminRoutes(group *gin.RouterGroup, svc *bookingusecases.BookingService, verifier *serviceauth.Verifier) {
	if verifier == nil || svc == nil {
		return
	}
	admin := group.Group("/internal/admin/waiting-policy", serviceauth.Middleware(verifier), func(c *gin.Context) {
		if c.GetString("service_name") != WaitingAdminService {
			httpx.Abort(c, apperrors.Forbidden("This caller cannot manage waiting-time rates.", nil))
			return
		}
		c.Next()
	})
	admin.GET("", func(c *gin.Context) {
		policy, err := svc.CurrentWaitingPolicy(c.Request.Context())
		if err != nil {
			httpx.Abort(c, err)
			return
		}
		c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": policy})
	})
	admin.PUT("", func(c *gin.Context) {
		var req waitingPolicyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
			return
		}
		policy, err := svc.SetWaitingPolicy(c.Request.Context(), bookingusecases.SetWaitingPolicyInput{
			FreeMinutes: req.FreeMinutes, BlockMinutes: req.BlockMinutes, BlockFeeKobo: req.BlockFeeKobo,
			PerformedBy: req.PerformedBy, Reason: req.Reason,
		})
		if err != nil {
			httpx.Abort(c, err)
			return
		}
		c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": policy})
	})
}
