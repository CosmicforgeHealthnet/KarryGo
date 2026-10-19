package provideruploadshttp

import (
	nethttp "net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	provideruploadsusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_uploads/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

// RegisterUploadRoutes mounts POST /provider/uploads: multipart form with a
// "file" part and a "purpose" field.
func RegisterUploadRoutes(group *gin.RouterGroup, svc *provideruploadsusecases.UploadService, authSvc *providerauthusecases.AuthService) {
	protected := group.Group("/provider")
	protected.Use(sharedauth.BearerMiddleware(authSvc.AccessSigner(), providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
	protected.POST("/uploads", func(c *gin.Context) { upload(c, svc) })
}

func upload(c *gin.Context, svc *provideruploadsusecases.UploadService) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	// Bound the whole request body, form fields included.
	c.Request.Body = nethttp.MaxBytesReader(c.Writer, c.Request.Body, provideruploadsusecases.MaxUploadBytes+(1<<20))

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		httpx.Abort(c, apperrors.Validation("Check your upload.", []apperrors.FieldViolation{
			{Field: "file", Message: "Attach a file (max 10 MB)."},
		}))
		return
	}
	defer file.Close()

	result, err := svc.Upload(c.Request.Context(), provideruploadsusecases.UploadInput{
		ProviderID: claims.Subject,
		Purpose:    c.PostForm("purpose"),
		Filename:   filepath.Base(header.Filename),
		Body:       file,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(nethttp.StatusCreated, gin.H{"success": true, "data": result})
}
