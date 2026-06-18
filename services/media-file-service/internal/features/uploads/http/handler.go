package uploadhttp

import (
	"crypto/subtle"
	"encoding/json"
	"strings"

	nethttp "net/http"

	"github.com/gin-gonic/gin"

	filemetadatamodels "cosmicforge/logistics/services/media-file-service/internal/features/file_metadata/models"
	uploadusecases "cosmicforge/logistics/services/media-file-service/internal/features/uploads/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

const callerServiceContextKey = "media_file_caller_service"

type UploadHandler struct {
	uploads        *uploadusecases.UploadService
	serviceTokens  map[string]string
	maxUploadBytes int64
}

func NewUploadHandler(uploads *uploadusecases.UploadService, serviceTokens map[string]string, maxUploadBytes int64) *UploadHandler {
	return &UploadHandler{
		uploads:        uploads,
		serviceTokens:  serviceTokens,
		maxUploadBytes: maxUploadBytes,
	}
}

func (h *UploadHandler) AuthenticateService() gin.HandlerFunc {
	return func(c *gin.Context) {
		serviceName := strings.TrimSpace(c.GetHeader("X-Karrygo-Service"))
		token := bearerToken(c.GetHeader("Authorization"))
		expectedToken, ok := h.serviceTokens[serviceName]
		if serviceName == "" || token == "" || !ok || subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
			httpx.Abort(c, apperrors.Unauthorized("Service authentication is required.", nil))
			return
		}

		c.Set(callerServiceContextKey, serviceName)
		c.Next()
	}
}

func (h *UploadHandler) Upload(c *gin.Context) {
	if h.maxUploadBytes > 0 {
		c.Request.Body = nethttp.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadBytes+1024*1024)
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		httpx.Abort(c, apperrors.BadRequest("File is required.", err))
		return
	}
	defer file.Close()

	metadata, err := parseMetadata(c.PostForm("metadata"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	asset, err := h.uploads.Upload(c.Request.Context(), uploadusecases.UploadInput{
		CallerService:    callerService(c),
		OwnerService:     c.PostForm("owner_service"),
		OwnerID:          c.PostForm("owner_id"),
		Purpose:          c.PostForm("purpose"),
		OriginalFilename: header.Filename,
		ContentType:      header.Header.Get("Content-Type"),
		SizeBytes:        header.Size,
		Body:             file,
		Metadata:         metadata,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respond(c, nethttp.StatusCreated, asset)
}

func (h *UploadHandler) GetFile(c *gin.Context) {
	asset, err := h.uploads.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	if !ownsAsset(c, asset) {
		httpx.Abort(c, apperrors.Forbidden("You do not have access to this file.", nil))
		return
	}

	respondOK(c, asset)
}

func (h *UploadHandler) ListFiles(c *gin.Context) {
	ownerService := c.Query("owner_service")
	if ownerService == "" {
		ownerService = callerService(c)
	}
	if ownerService != callerService(c) {
		httpx.Abort(c, apperrors.Forbidden("You do not have access to these files.", nil))
		return
	}

	assets, err := h.uploads.List(c.Request.Context(), filemetadatamodels.ListMediaAssetsFilter{
		OwnerService: ownerService,
		OwnerID:      c.Query("owner_id"),
		Purpose:      c.Query("purpose"),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respondOK(c, gin.H{"files": assets})
}

func (h *UploadHandler) DeleteFile(c *gin.Context) {
	asset, err := h.uploads.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	if !ownsAsset(c, asset) {
		httpx.Abort(c, apperrors.Forbidden("You do not have access to this file.", nil))
		return
	}

	if err := h.uploads.Delete(c.Request.Context(), asset.ID); err != nil {
		httpx.Abort(c, err)
		return
	}

	respondOK(c, gin.H{"deleted": true})
}

func parseMetadata(raw string) (map[string]interface{}, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]interface{}{}, nil
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, apperrors.BadRequest("Metadata must be a JSON object.", err)
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}

	return metadata, nil
}

func callerService(c *gin.Context) string {
	value, _ := c.Get(callerServiceContextKey)
	service, _ := value.(string)
	return service
}

func ownsAsset(c *gin.Context, asset filemetadatamodels.MediaAsset) bool {
	return asset.OwnerService == callerService(c)
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}

	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}
