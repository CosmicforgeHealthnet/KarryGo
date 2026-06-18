package uploadhttp

import (
	"github.com/gin-gonic/gin"

	uploadusecases "cosmicforge/logistics/services/media-file-service/internal/features/uploads/usecases"
)

func RegisterUploadRoutes(group *gin.RouterGroup, uploadService *uploadusecases.UploadService, serviceTokens map[string]string, maxUploadBytes int64) {
	handler := NewUploadHandler(uploadService, serviceTokens, maxUploadBytes)

	protected := group.Group("")
	protected.Use(handler.AuthenticateService())
	protected.POST("/uploads", handler.Upload)
	protected.GET("/files/:id", handler.GetFile)
	protected.GET("/files", handler.ListFiles)
	protected.DELETE("/files/:id", handler.DeleteFile)
}
