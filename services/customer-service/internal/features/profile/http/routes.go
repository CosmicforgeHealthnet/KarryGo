package profilehttp

import (
	"github.com/gin-gonic/gin"

	profileusecases "cosmicforge/logistics/services/customer-service/internal/features/profile/usecases"
	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func RegisterProfileRoutes(group *gin.RouterGroup, profileService *profileusecases.ProfileService, accessSigner *sharedauth.TokenSigner) {
	handler := NewProfileHandler(profileService)

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
