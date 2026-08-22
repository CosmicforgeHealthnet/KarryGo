package savedlocationhttp

import (
	nethttp "net/http"
	"strings"

	"github.com/gin-gonic/gin"

	savedlocationusecases "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// InternalSavedLocationHandler exposes only the minimal saved-address snapshot
// required by trusted platform services.
//
// It does not expose Customer Service profile/auth/session data.
type InternalSavedLocationHandler struct {
	locations *savedlocationusecases.SavedLocationService
}

func NewInternalSavedLocationHandler(
	locations *savedlocationusecases.SavedLocationService,
) *InternalSavedLocationHandler {
	return &InternalSavedLocationHandler{
		locations: locations,
	}
}

// GetLocation resolves one address for one specific customer.
//
// GET /internal/customers/:customerId/saved-locations/:locationId
//
// Both identifiers are required. The use case verifies ownership, preventing a
// caller from pairing Customer A with Customer B's saved-location identifier.
func (h *InternalSavedLocationHandler) GetLocation(c *gin.Context) {
	customerID := strings.TrimSpace(c.Param("customerId"))
	locationID := strings.TrimSpace(c.Param("locationId"))

	if customerID == "" || locationID == "" {
		httpx.RespondError(
			c,
			apperrors.NotFound("Saved location not found.", nil),
		)
		return
	}

	location, err := h.locations.GetLocation(
		c.Request.Context(),
		locationID,
		customerID,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	c.JSON(nethttp.StatusOK, gin.H{
		"success": true,
		"data":    location.Public(),
	})
}

// RegisterInternalRoutes mounts the Customer Service saved-address authority
// behind the platform's shared HMAC-v2 service-auth contract.
//
// When verifier is nil, no internal route is registered.
func RegisterInternalRoutes(
	group *gin.RouterGroup,
	locations *savedlocationusecases.SavedLocationService,
	verifier *serviceauth.Verifier,
) {
	if verifier == nil || locations == nil {
		return
	}

	handler := NewInternalSavedLocationHandler(locations)

	internal := group.Group("/internal")
	internal.Use(serviceauth.Middleware(verifier))

	internal.GET(
		"/customers/:customerId/saved-locations/:locationId",
		handler.GetLocation,
	)
}
