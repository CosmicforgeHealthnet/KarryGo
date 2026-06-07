package authhttp

import (
	"github.com/gin-gonic/gin"

	authusecases "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/usecases"
)

func RegisterRoutes(group *gin.RouterGroup, auth *authusecases.AuthUsecase) {
	handler := NewHandler(auth)

	group.POST("/start", handler.Start)
	group.POST("/verify", handler.Verify)
	group.POST("/refresh", handler.Refresh)

	protected := group.Group("")
	protected.Use(DispatchRiderAuthRequired(auth.TokenUsecase()))
	protected.POST("/logout", handler.Logout)
}
