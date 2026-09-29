package profilehttp

import (
	"context"
	"github.com/gin-gonic/gin"
	"time"

	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	profileusecases "cosmicforge/logistics/services/customer-service/internal/features/profile/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func RegisterProfileRoutes(group *gin.RouterGroup, profileService *profileusecases.ProfileService, accessSigner *sharedauth.TokenSigner, limiters ...interface {
	Allow(context.Context, string, string, int, time.Duration) error
}) {
	handler := NewProfileHandler(profileService, limiters...)

	protected := group.Group("/profile")
	protected.Use(sharedauth.BearerMiddleware(accessSigner, authusecases.CustomerRole, authusecases.CustomerService))
	protected.GET("", handler.GetProfile)
	protected.PUT("", handler.UpdateProfile)
	protected.POST("/photo", handler.UploadPhoto)
	protected.PUT("/photo-url", handler.SavePhotoURL)
	protected.GET("/emergency-contacts", handler.GetEmergencyContacts)
	protected.POST("/emergency-contacts", handler.AddEmergencyContact)
	protected.DELETE("/emergency-contacts/:id", handler.DeleteEmergencyContact)
}
