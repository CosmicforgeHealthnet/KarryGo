package availabilityhttp

import (
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	availabilityusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type AvailabilityHandler struct {
	svc *availabilityusecases.AvailabilityService
}

func NewAvailabilityHandler(svc *availabilityusecases.AvailabilityService) *AvailabilityHandler {
	return &AvailabilityHandler{svc: svc}
}

type setAvailabilityRequest struct {
	Status  string  `json:"status"`
	TruckID string  `json:"truck_id"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
}

type heartbeatRequest struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

func (h *AvailabilityHandler) SetAvailability(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var req setAvailabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	result, err := h.svc.SetAvailability(c.Request.Context(), availabilityusecases.SetAvailabilityInput{
		ProviderID: claims.Subject,
		Status:     req.Status,
		TruckID:    req.TruckID,
		Lat:        req.Lat,
		Lng:        req.Lng,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": result})
}

func (h *AvailabilityHandler) Heartbeat(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var req heartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if err := h.svc.Heartbeat(c.Request.Context(), availabilityusecases.HeartbeatInput{
		ProviderID: claims.Subject,
		Lat:        req.Lat,
		Lng:        req.Lng,
	}); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": gin.H{"heartbeat": "ok"}})
}

func (h *AvailabilityHandler) GetStatus(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	result, err := h.svc.GetStatus(c.Request.Context(), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": result})
}

func (h *AvailabilityHandler) CheckAvailability(c *gin.Context) {
	result, err := h.svc.CheckAvailability(c.Request.Context())
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": result})
}
