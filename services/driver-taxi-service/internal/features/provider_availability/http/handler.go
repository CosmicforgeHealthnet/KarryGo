package availabilityhttp

import (
	nethttp "net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	availabilityusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/httpx"
)

type AvailabilityHandler struct {
	availability    *availabilityusecases.AvailabilityService
	defaultRadiusKm float64
}

func NewAvailabilityHandler(availability *availabilityusecases.AvailabilityService, defaultRadiusKm float64) *AvailabilityHandler {
	return &AvailabilityHandler{availability: availability, defaultRadiusKm: defaultRadiusKm}
}

type successResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
}

func respond(c *gin.Context, status int, data interface{}) {
	c.JSON(status, successResponse{Success: true, Data: data})
}

type setAvailabilityRequest struct {
	Status    string  `json:"status"`
	VehicleID string  `json:"vehicle_id"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	Heading   float64 `json:"heading"`
	Speed     float64 `json:"speed"`
	Accuracy  float64 `json:"accuracy"`
}

type heartbeatRequest struct {
	VehicleID string  `json:"vehicle_id"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	Heading   float64 `json:"heading"`
	Speed     float64 `json:"speed"`
	Accuracy  float64 `json:"accuracy"`
	// OnTrip lets the server hand back a tighter heartbeat cadence while the
	// driver is carrying a passenger.
	OnTrip bool `json:"on_trip"`
}

func providerID(c *gin.Context) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return "", false
	}
	return claims.Subject, true
}

func (h *AvailabilityHandler) SetAvailability(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	var req setAvailabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	state, err := h.availability.SetAvailability(c.Request.Context(), availabilityusecases.SetAvailabilityInput{
		ProviderID: id,
		Status:     req.Status,
		VehicleID:  req.VehicleID,
		Lat:        req.Lat,
		Lng:        req.Lng,
		Heading:    req.Heading,
		Speed:      req.Speed,
		Accuracy:   req.Accuracy,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, state)
}

func (h *AvailabilityHandler) Heartbeat(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	var req heartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	state, err := h.availability.Heartbeat(c.Request.Context(), availabilityusecases.SetAvailabilityInput{
		ProviderID: id,
		VehicleID:  req.VehicleID,
		Lat:        req.Lat,
		Lng:        req.Lng,
		Heading:    req.Heading,
		Speed:      req.Speed,
		Accuracy:   req.Accuracy,
	}, req.OnTrip)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, state)
}

func (h *AvailabilityHandler) GetAvailability(c *gin.Context) {
	id, ok := providerID(c)
	if !ok {
		return
	}
	state, err := h.availability.GetAvailability(c.Request.Context(), id)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, state)
}

// CheckAvailability gates the passenger's booking entry point.
func (h *AvailabilityHandler) CheckAvailability(c *gin.Context) {
	lat, _ := strconv.ParseFloat(c.Query("lat"), 64)
	lng, _ := strconv.ParseFloat(c.Query("lng"), 64)

	result, err := h.availability.CheckAvailability(c.Request.Context(),
		geo.Point{Lat: lat, Lng: lng}, h.defaultRadiusKm)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusOK, result)
}
