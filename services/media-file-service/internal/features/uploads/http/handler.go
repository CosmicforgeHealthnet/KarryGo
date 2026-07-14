package uploadhttp

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	nethttp "net/http"

	"github.com/gin-gonic/gin"

	filemetadatamodels "cosmicforge/logistics/services/media-file-service/internal/features/file_metadata/models"
	uploadusecases "cosmicforge/logistics/services/media-file-service/internal/features/uploads/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

const (
	callerServiceContextKey = "media_file_caller_service"
	partnerIDContextKey     = "media_file_partner_id"
	partnerOwnerService     = "partner-service"
)

type UploadHandler struct {
	uploads          *uploadusecases.UploadService
	serviceTokens    map[string]string
	maxUploadBytes   int64
	partnerJWTSecret string
}

func NewUploadHandler(uploads *uploadusecases.UploadService, serviceTokens map[string]string, maxUploadBytes int64, partnerJWTSecret string) *UploadHandler {
	return &UploadHandler{
		uploads:          uploads,
		serviceTokens:    serviceTokens,
		maxUploadBytes:   maxUploadBytes,
		partnerJWTSecret: partnerJWTSecret,
	}
}

func (h *UploadHandler) AuthenticateService() gin.HandlerFunc {
	return func(c *gin.Context) {
		serviceName := strings.TrimSpace(c.GetHeader("X-Karrygo-Service"))
		token := bearerToken(c.GetHeader("Authorization"))
		expectedToken, ok := h.serviceTokens[serviceName]
		if serviceName != "" && token != "" && ok && subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) == 1 {
			c.Set(callerServiceContextKey, serviceName)
			c.Next()
			return
		}

		if partnerID, ok := verifyPartnerSessionJWT(h.partnerJWTSecret, token, time.Now()); ok {
			c.Set(callerServiceContextKey, partnerOwnerService)
			c.Set(partnerIDContextKey, partnerID)
			c.Next()
			return
		}

		httpx.Abort(c, apperrors.Unauthorized("Service authentication is required.", nil))
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

	ownerService := c.PostForm("owner_service")
	ownerID := c.PostForm("owner_id")
	if callerService(c) == partnerOwnerService {
		if ownerService == "" {
			ownerService = partnerOwnerService
		}
		if ownerID == "" {
			ownerID = partnerID(c)
		}
	}

	asset, err := h.uploads.Upload(c.Request.Context(), uploadusecases.UploadInput{
		CallerService:    callerService(c),
		OwnerService:     ownerService,
		OwnerID:          ownerID,
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

func (h *UploadHandler) Download(c *gin.Context) {
	result, err := h.uploads.Download(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	defer result.Reader.Close()

	c.Header("Content-Type", result.ContentType)
	c.Header("Content-Length", fmt.Sprintf("%d", result.Size))
	c.Header("Content-Disposition", "inline")

	if _, err := io.Copy(c.Writer, result.Reader); err != nil {
		c.Error(err)
	}
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

func partnerID(c *gin.Context) string {
	value, _ := c.Get(partnerIDContextKey)
	id, _ := value.(string)
	return id
}

type partnerSessionClaims struct {
	PartnerID string `json:"partner_id"`
	Sub       string `json:"sub"`
	Exp       int64  `json:"exp"`
}

func verifyPartnerSessionJWT(secret string, token string, now time.Time) (string, bool) {
	if secret == "" || token == "" {
		return "", false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", false
	}
	unsigned := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	var claims partnerSessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", false
	}
	partnerID := claims.PartnerID
	if partnerID == "" {
		partnerID = claims.Sub
	}
	if partnerID == "" || claims.Exp == 0 || now.Unix() >= claims.Exp {
		return "", false
	}
	return partnerID, true
}
