package vendorauthhttp

import (
	"github.com/gin-gonic/gin"

	sharedauth "cosmicforge/logistics/shared/go/auth"

	vendorauthrepositories "cosmicforge/logistics/services/vendor-service/internal/features/auth/repositories"
	vendorauthusecases "cosmicforge/logistics/services/vendor-service/internal/features/auth/usecases"
)

// RegisterRoutes mounts /auth/start|verify|resend|refresh|logout (public) and
// /auth/step-up/start|verify (authenticated) under the given group, per
// docs/API_ROUTE_REGISTER.md.
func RegisterRoutes(group *gin.RouterGroup, handler *Handler, signer *sharedauth.TokenSigner,
	sessions *vendorauthrepositories.SessionRepository, staff vendorauthusecases.StaffResolver) {
	auth := group.Group("/auth")
	auth.POST("/start", handler.Start)
	auth.POST("/verify", handler.Verify)
	auth.POST("/resend", handler.Resend)
	auth.POST("/refresh", handler.Refresh)
	auth.POST("/logout", handler.Logout)

	stepUp := auth.Group("/step-up")
	stepUp.Use(RequireAuth(signer, sessions, staff))
	stepUp.POST("/start", handler.StepUpStart)
	stepUp.POST("/verify", handler.StepUpVerify)
}
