package identityhttp

import (
	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/usecases"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// RegisterIdentityRoutes wires the internal provider-lookup endpoint behind
// HMAC service-auth. Registration is skipped when no service secrets are
// configured -- the same "disabled integration, no half-open surface"
// convention as operationshttp's own /internal group.
//
// Mounted separately from operationshttp's /internal group (rather than
// folded into it) so this package can be added to any future service
// wanting the exact "GET /internal/providers/:id" identity-lookup shape
// without also pulling in booking-ops concerns -- matches
// driver-hauling-service's own module boundary for the identical endpoint.
func RegisterIdentityRoutes(group *gin.RouterGroup, auth *providerauthusecases.AuthService, verifier *serviceauth.Verifier) {
	if verifier == nil {
		return
	}
	handler := NewHandler(auth)

	internal := group.Group("/internal")
	internal.Use(serviceauth.Middleware(verifier))
	internal.GET("/providers/:id", handler.GetProvider)
}
