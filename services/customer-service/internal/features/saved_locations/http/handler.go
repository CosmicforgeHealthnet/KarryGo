package savedlocationhttp

import (
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	savedlocationusecases "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type SavedLocationHandler struct {
	locations *savedlocationusecases.SavedLocationService
}

func NewSavedLocationHandler(locations *savedlocationusecases.SavedLocationService) *SavedLocationHandler {
	return &SavedLocationHandler{locations: locations}
}

type saveLocationRequest struct {
	Title       string  `json:"title"`
	Address     string  `json:"address"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Description string  `json:"description"`
}

func (h *SavedLocationHandler) ListLocations(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	locations, err := h.locations.ListLocations(c.Request.Context(), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	public := make([]interface{}, len(locations))
	for i, l := range locations {
		public[i] = l.Public()
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": gin.H{"locations": public}})
}

func (h *SavedLocationHandler) AddLocation(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	var req saveLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}

	location, err := h.locations.AddLocation(c.Request.Context(), savedlocationusecases.SaveLocationInput{
		CustomerID:  claims.Subject,
		Title:       req.Title,
		Address:     req.Address,
		Latitude:    req.Latitude,
		Longitude:   req.Longitude,
		Description: req.Description,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, gin.H{"success": true, "data": location.Public()})
}

func (h *SavedLocationHandler) UpdateLocation(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	var req saveLocationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}

	location, err := h.locations.UpdateLocation(c.Request.Context(), c.Param("id"), savedlocationusecases.SaveLocationInput{
		CustomerID:  claims.Subject,
		Title:       req.Title,
		Address:     req.Address,
		Latitude:    req.Latitude,
		Longitude:   req.Longitude,
		Description: req.Description,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": location.Public()})
}

func (h *SavedLocationHandler) DeleteLocation(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	if err := h.locations.DeleteLocation(c.Request.Context(), c.Param("id"), claims.Subject); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": gin.H{}})
}
