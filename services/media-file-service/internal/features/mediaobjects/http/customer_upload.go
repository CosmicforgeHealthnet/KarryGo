package http

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
)

// RegisterCustomerUploadRoute exposes only grant redemption. Authorization
// remains private HMAC; the client receives one short-lived token from its
// authenticated Customer Service session, never a service credential.
func RegisterCustomerUploadRoute(group *gin.RouterGroup, service *usecases.Service, maxBytes int64) {
	if service == nil {
		return
	}
	group.POST("/customer-uploads/:id/content", func(c *gin.Context) {
		if maxBytes <= 0 {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, errorBody("upload_unavailable"))
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		content, err := io.ReadAll(c.Request.Body)
		if err != nil {
			fail(c, usecases.ErrInvalid)
			return
		}
		declaredSize, err := strconv.ParseInt(c.GetHeader("X-Declared-Size"), 10, 64)
		if err != nil || declaredSize <= 0 {
			fail(c, usecases.ErrInvalid)
			return
		}
		object, err := service.CompleteCustomerUpload(c.Request.Context(), usecases.CompleteUploadInput{
			MediaObjectID:    c.Param("id"),
			AuthorizationID:  strings.TrimSpace(c.GetHeader("X-Upload-Authorization-ID")),
			UploadToken:      strings.TrimSpace(c.GetHeader("X-Upload-Token")),
			DeclaredMIME:     c.GetHeader("Content-Type"),
			DeclaredSize:     declaredSize,
			DeclaredChecksum: strings.TrimSpace(c.GetHeader("X-Declared-Checksum-SHA256")),
			Content:          content,
		})
		if err != nil {
			fail(c, err)
			return
		}
		respond(c, objectView(object))
	})
}
