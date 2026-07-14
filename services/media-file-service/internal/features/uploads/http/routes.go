package uploadhttp

import (
	"github.com/gin-gonic/gin"

	uploadusecases "cosmicforge/logistics/services/media-file-service/internal/features/uploads/usecases"
)

func RegisterUploadRoutes(group *gin.RouterGroup, uploadService *uploadusecases.UploadService, serviceTokens map[string]string, maxUploadBytes int64, partnerJWTSecret string) {
	handler := NewUploadHandler(uploadService, serviceTokens, maxUploadBytes, partnerJWTSecret)

	protected := group.Group("")
	protected.Use(handler.AuthenticateService())
	protected.POST("/uploads", handler.Upload)
	// Partner web onboarding expects this shorter upload path. Keep it as a
	// compatibility alias while preserving the established media-files API.
	protected.POST("../media/upload", handler.Upload)
	protected.GET("/files/:id", handler.GetFile)
	protected.GET("/files", handler.ListFiles)
	protected.DELETE("/files/:id", handler.DeleteFile)

	group.GET("/:id/download", handler.Download)
}
