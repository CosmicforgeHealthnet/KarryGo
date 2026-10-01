package mediauploadhttp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/services/customer-service/internal/config"
	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const maxEvidenceBytes int64 = 10 << 20

type limiter interface {
	Allow(context.Context, string, string, int, time.Duration) error
}

type Handler struct {
	Config  config.Config
	HTTP    *http.Client
	Limiter limiter
}

type sessionRequest struct {
	Purpose         string `json:"purpose"`
	SourceAuthority string `json:"source_authority"`
	SourceRef       string `json:"source_ref"`
	MIMEType        string `json:"mime_type"`
	ByteSize        int64  `json:"byte_size"`
}

func Register(group *gin.RouterGroup, signer *sharedauth.TokenSigner, handler Handler) {
	protected := group.Group("")
	protected.Use(sharedauth.BearerMiddleware(signer, authusecases.CustomerRole, authusecases.CustomerService))
	protected.POST("/media-upload-sessions", handler.create)
}

func (h Handler) create(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	// Server-side V4 §24 gate: authoritative even for an outdated client.
	if !h.Config.MediaUploadSessionEnabled {
		httpx.Abort(c, apperrors.Unavailable("Evidence uploads are unavailable.", nil))
		return
	}
	if h.Limiter == nil || h.Config.MediaBaseURL == "" || h.Config.MediaPublicBaseURL == "" || h.Config.SupportBaseURL == "" || len(h.Config.MediaPrivateSecret) == 0 {
		httpx.Abort(c, apperrors.Unavailable("Evidence uploads are unavailable.", nil))
		return
	}
	if err := h.Limiter.Allow(c.Request.Context(), "evidence_upload_session", claims.Subject, 10, time.Hour); err != nil {
		httpx.Abort(c, err)
		return
	}
	var in sessionRequest
	if err := c.ShouldBindJSON(&in); err != nil ||
		in.Purpose != "DISPUTE_ATTACHMENT" || in.SourceAuthority != "SUPPORT_DISPUTE" ||
		!opaqueRef(in.SourceRef) ||
		(in.MIMEType != "image/jpeg" && in.MIMEType != "image/png" && in.MIMEType != "image/webp" && in.MIMEType != "application/pdf") ||
		in.ByteSize <= 0 || in.ByteSize > maxEvidenceBytes {
		httpx.Abort(c, apperrors.BadRequest("Unsupported evidence upload scope.", nil))
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if _, err := uuid.Parse(key); err != nil {
		httpx.Abort(c, apperrors.BadRequest("A UUID Idempotency-Key is required.", nil))
		return
	}
	client := h.HTTP
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	// Support independently verifies the same customer bearer and returns its
	// server-owned complainant id. A caller cannot authorize another case by
	// merely naming its opaque id.
	ownerURL := strings.TrimRight(h.Config.SupportBaseURL, "/") + "/api/v1/support-disputes/complaints/" + url.PathEscape(in.SourceRef)
	ownerReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, ownerURL, nil)
	if err != nil {
		httpx.Abort(c, apperrors.Unavailable("Support ownership is unavailable.", err))
		return
	}
	ownerReq.Header.Set("Authorization", c.GetHeader("Authorization"))
	ownerResp, err := client.Do(ownerReq)
	if err != nil {
		httpx.Abort(c, apperrors.Unavailable("Support ownership is unavailable.", err))
		return
	}
	defer ownerResp.Body.Close()
	if ownerResp.StatusCode != http.StatusOK {
		httpx.Abort(c, apperrors.Forbidden("This support case is unavailable.", nil))
		return
	}
	var owner struct {
		Success bool `json:"success"`
		Data    struct {
			ID              string `json:"id"`
			ComplainantID   string `json:"complainant_id"`
			ComplainantType string `json:"complainant_type"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(ownerResp.Body, 64<<10)).Decode(&owner) != nil ||
		!owner.Success || owner.Data.ID != in.SourceRef || owner.Data.ComplainantID != claims.Subject || owner.Data.ComplainantType != "customer" {
		httpx.Abort(c, apperrors.Forbidden("This support case is unavailable.", nil))
		return
	}
	mediaBody, _ := json.Marshal(map[string]any{
		"ownerType": "CUSTOMER", "ownerReference": claims.Subject,
		"documentCategory": "SUPPORT_EVIDENCE", "uploadPurpose": in.Purpose,
		"allowedMimeTypes": []string{in.MIMEType}, "maxSizeBytes": in.ByteSize,
		"retentionClass": "SUPPORT", "visibility": "PRIVATE",
		"bindingType": "COMPLAINT", "bindingId": in.SourceRef,
	})
	mediaURL := strings.TrimRight(h.Config.MediaBaseURL, "/") + "/api/v1/media-files/internal/media/upload-authorizations"
	mediaReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, mediaURL, bytes.NewReader(mediaBody))
	if err != nil {
		httpx.Abort(c, apperrors.Unavailable("Media authorization is unavailable.", err))
		return
	}
	mediaReq.Header.Set("Content-Type", "application/json")
	mediaReq.Header.Set("X-Organization-ID", "customer_"+claims.Subject)
	mediaReq.Header.Set("Idempotency-Key", claims.Subject+":"+key)
	fingerprint := sha256.Sum256(mediaBody)
	mediaReq.Header.Set("X-Request-Fingerprint", hex.EncodeToString(fingerprint[:]))
	if err := serviceauth.SignRequest(mediaReq, "customer-service", h.Config.MediaPrivateSecret, mediaBody, time.Now().UTC()); err != nil {
		httpx.Abort(c, apperrors.Unavailable("Media authorization is unavailable.", err))
		return
	}
	mediaResp, err := client.Do(mediaReq)
	if err != nil {
		httpx.Abort(c, apperrors.Unavailable("Media authorization is unavailable.", err))
		return
	}
	defer mediaResp.Body.Close()
	if mediaResp.StatusCode == http.StatusConflict {
		httpx.Abort(c, apperrors.Conflict("Idempotency-Key was already used for a different upload scope.", nil))
		return
	}
	if mediaResp.StatusCode != http.StatusOK {
		httpx.Abort(c, apperrors.Unavailable("Media authorization was refused.", nil))
		return
	}
	var grant struct {
		Success bool `json:"success"`
		Data    struct {
			AuthorizationID string   `json:"uploadAuthorizationId"`
			ObjectID        string   `json:"mediaObjectId"`
			Token           string   `json:"uploadToken"`
			ExpiresAt       string   `json:"expiresAt"`
			MaxBytes        int64    `json:"maxSizeBytes"`
			MIMEs           []string `json:"allowedMimeTypes"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(mediaResp.Body, 64<<10)).Decode(&grant) != nil ||
		!grant.Success || grant.Data.AuthorizationID == "" || grant.Data.ObjectID == "" || grant.Data.Token == "" ||
		grant.Data.MaxBytes <= 0 || grant.Data.MaxBytes > in.ByteSize ||
		len(grant.Data.MIMEs) != 1 || grant.Data.MIMEs[0] != in.MIMEType {
		httpx.Abort(c, apperrors.Unavailable("Media authorization is incomplete.", nil))
		return
	}
	expiresAt, err := time.Parse(time.RFC3339, grant.Data.ExpiresAt)
	if err != nil || !expiresAt.After(time.Now().UTC()) {
		httpx.Abort(c, apperrors.Unavailable("Media authorization has expired.", nil))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{
		"upload_session_id":  grant.Data.AuthorizationID,
		"media_ref":          grant.Data.ObjectID,
		"upload_url":         strings.TrimRight(h.Config.MediaPublicBaseURL, "/") + "/api/v1/media-files/customer-uploads/" + url.PathEscape(grant.Data.ObjectID) + "/content",
		"upload_token":       grant.Data.Token,
		"expires_at":         grant.Data.ExpiresAt,
		"max_bytes":          grant.Data.MaxBytes,
		"allowed_mime_types": grant.Data.MIMEs,
	}})
}

func opaqueRef(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, ch := range value {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}
