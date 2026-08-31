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

// heartbeatRequest carries one GPS fix. Heading, speed and accuracy are
// optional so an older app build that only sends lat/lng keeps working.
type heartbeatRequest struct {
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	Heading  float64 `json:"heading"`
	Speed    float64 `json:"speed"`
	Accuracy float64 `json:"accuracy"`
	// OnTrip lets the device ask for the tighter reporting cadence while it
	// is driving a live customer map.
	OnTrip bool `json:"on_trip"`
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
		Heading:    req.Heading,
		Speed:      req.Speed,
		Accuracy:   req.Accuracy,
	}); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": gin.H{
		"heartbeat": "ok",
		// The app reads this back each beat, so the cadence is tunable
		// server side without a new app release.
		"next_heartbeat_seconds": h.svc.HeartbeatIntervalSeconds(req.OnTrip),
	}})
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
