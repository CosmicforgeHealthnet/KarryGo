package identityhttp

import (
	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// RegisterIdentityRoutes wires the internal provider-lookup endpoint behind HMAC
// service-auth. Registration is skipped when no service secrets are configured.
func RegisterIdentityRoutes(group *gin.RouterGroup, authService *providerauthusecases.AuthService, verifier *serviceauth.Verifier) {
	if verifier == nil {
		return
	}
	handler := NewHandler(authService)

	internal := group.Group("/internal")
	internal.Use(serviceauth.Middleware(verifier))
	internal.GET("/providers/:id", handler.GetProvider)
}
