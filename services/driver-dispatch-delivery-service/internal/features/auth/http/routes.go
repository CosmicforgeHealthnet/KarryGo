package authhttp

import (
	"github.com/gin-gonic/gin"

	authusecases "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/usecases"
)

func RegisterRoutes(group *gin.RouterGroup, auth *authusecases.AuthUsecase) {
	handler := NewHandler(auth)

	// Legacy single-flow start (phone-only, backward compat).
	group.POST("/start", handler.Start)

	// New purpose-aware start endpoints.
	group.POST("/signup/start", handler.SignupStart)
	group.POST("/login/start", handler.LoginStart)

	// Verify — backward compatible (supports legacy phone_number field
	// as well as the new identifier + purpose fields).
	group.POST("/verify", handler.Verify)

	group.POST("/refresh", handler.Refresh)

	protected := group.Group("")
	protected.Use(DispatchRiderAuthRequired(auth.TokenUsecase()))
	protected.POST("/logout", handler.Logout)
}
