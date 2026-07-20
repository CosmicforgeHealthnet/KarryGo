package identityhttp

import (
	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// RegisterIdentityRoutes wires the internal rider-lookup endpoint behind HMAC
// service-auth. Registration is skipped when no service secrets are configured.
func RegisterIdentityRoutes(engine *gin.Engine, providers ProviderLookup, verifier *serviceauth.Verifier) {
	if verifier == nil {
		return
	}
	handler := NewHandler(providers)

	internal := engine.Group("/api/v1/dispatch/internal")
	internal.Use(serviceauth.Middleware(verifier))
	internal.GET("/providers/:id", handler.GetProvider)
}
