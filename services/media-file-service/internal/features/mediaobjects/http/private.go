// Package http exposes the private, service-to-service media contract.
//
// Every route here sits behind four gates, in this order: shared HMAC service
// authentication (which also bounds request age), a caller allowlist, a
// single-use nonce, and then the use case's own tenant and ownership checks.
// Passing the first three proves the request came from a service that holds
// the secret and has not been replayed; it proves nothing about what that
// service may touch, which is why the fourth gate exists.
package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/models"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/repositories"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
)

const (
	nonceHeader          = "X-Service-Nonce"
	organizationHeader   = "X-Organization-ID"
	correlationHeader    = "X-Correlation-ID"
	causationHeader      = "X-Causation-ID"
	idempotencyHeader    = "Idempotency-Key"
	fingerprintHeader    = "X-Request-Fingerprint"
	payloadVersionHeader = "X-Payload-Version"
	nonceTTL             = 10 * time.Minute
	// maxUploadBody bounds what the completion route will read at all. It is
	// deliberately larger than the policy maximum so an oversized upload is
	// refused by the size check with a controlled code, rather than by a
	// truncated read that would look like an incomplete transfer.
	maxUploadBodySlack = 1 << 20
)

// Handler serves the private media contract.
type Handler struct {
	service *usecases.Service
	pool    *pgxpool.Pool
}

// NewHandler builds the handler.
func NewHandler(service *usecases.Service, pool *pgxpool.Pool) *Handler {
	return &Handler{service: service, pool: pool}
}

// RegisterPrivateRoutes mounts the versioned private surface.
func RegisterPrivateRoutes(group *gin.RouterGroup, handler *Handler,
	verifier *serviceauth.Verifier, allowedCallers []string, maxUploadBytes int64) {
	allowed := map[string]bool{}
	for _, caller := range allowedCallers {
		if name := strings.TrimSpace(caller); name != "" {
			allowed[name] = true
		}
	}

	group.Use(func(c *gin.Context) {
		// Verification is performed here rather than through
		// serviceauth.Middleware because that helper reports failures by
		// recording a gin error and relies on a global error-rendering
		// middleware to turn it into a status. Mounting these routes on an
		// engine without that middleware would make a rejected signature
		// answer 200 with an empty body — an authentication gate must not
		// depend on someone else's wiring to fail closed.
		name, err := verifier.VerifyRequest(c.Request)
		if err != nil {
			status := http.StatusUnauthorized
			code := "service_authentication_required"
			if appErr := apperrors.From(err); appErr != nil && appErr.Status == http.StatusForbidden {
				status = http.StatusForbidden
				code = "caller_not_allowed"
			}
			c.AbortWithStatusJSON(status, errorBody(code))
			return
		}
		c.Set("service_name", name)
		if !allowed[name] {
			c.AbortWithStatusJSON(http.StatusForbidden, errorBody("caller_not_allowed"))
			return
		}
		nonce := strings.TrimSpace(c.GetHeader(nonceHeader))
		if nonce == "" || len(nonce) > 128 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errorBody("replay_nonce_required"))
			return
		}
		limit := maxUploadBytes + maxUploadBodySlack
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, limit))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, errorBody("invalid_body"))
			return
		}
		c.Request.Body = io.NopCloser(strings.NewReader(string(body)))
		// The nonce is bound to the exact request it authenticates, so an
		// attacker cannot reuse a captured nonce on a different call.
		sum := sha256.Sum256(append([]byte(c.Request.Method+"\x00"+c.Request.URL.Path+"\x00"), body...))
		if err := repositories.ClaimServiceNonce(c.Request.Context(), handler.pool, name, nonce,
			hex.EncodeToString(sum[:]), time.Now().UTC().Add(nonceTTL)); err != nil {
			c.AbortWithStatusJSON(http.StatusConflict, errorBody("replayed_request"))
			return
		}
		c.Next()
	})

	group.POST("/upload-authorizations", handler.authorizeUpload)
	group.POST("/objects/:id/content", handler.completeUpload)
	group.GET("/objects/:id", handler.readObject)
	group.GET("/objects/:id/history", handler.readHistory)
	group.POST("/objects/:id/download-authorizations", handler.authorizeDownload)
	group.GET("/objects/:id/content", handler.readContent)
	group.POST("/objects/:id/deletions", handler.requestDeletion)
	group.GET("/objects/:id/deletion", handler.readDeletion)
	group.POST("/objects/:id/retention", handler.setRetention)
}

func errorBody(code string) gin.H {
	return gin.H{"success": false, "error": gin.H{"code": code}}
}

func respond(c *gin.Context, value any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": value})
}

// fail maps a use-case error onto a status and a controlled code. Downstream
// detail is never echoed: a caller learns which rule refused it, not what the
// storage backend said.
func fail(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "internal_error"

	var validationErr *usecases.ValidationError
	switch {
	case errors.As(err, &validationErr):
		status = http.StatusUnprocessableEntity
		code = strings.ToLower(validationErr.Code)
	case errors.Is(err, usecases.ErrNotFound):
		status = http.StatusNotFound
		code = "not_found"
	case errors.Is(err, usecases.ErrAuthorizationUsed):
		status = http.StatusConflict
		code = "authorization_already_used"
	case errors.Is(err, usecases.ErrAuthorizationGone):
		status = http.StatusGone
		code = "authorization_expired"
	case errors.Is(err, usecases.ErrConflict):
		status = http.StatusConflict
		code = "conflict"
	case errors.Is(err, usecases.ErrNotDownloadable):
		status = http.StatusForbidden
		code = "not_downloadable"
	case errors.Is(err, usecases.ErrDeletionBlocked):
		status = http.StatusConflict
		code = "deletion_blocked"
	case errors.Is(err, usecases.ErrUnavailable):
		status = http.StatusServiceUnavailable
		code = "storage_unavailable"
	case errors.Is(err, usecases.ErrInvalid):
		status = http.StatusUnprocessableEntity
		code = "validation_failed"
	case errors.Is(err, repositories.ErrVersionMismatch):
		status = http.StatusConflict
		code = "version_conflict"
	}
	c.JSON(status, errorBody(code))
}

func callerFrom(c *gin.Context) usecases.Caller {
	return usecases.Caller{
		Service:        c.GetString("service_name"),
		OrganizationID: strings.TrimSpace(c.GetHeader(organizationHeader)),
		CorrelationID:  c.GetHeader(correlationHeader),
		CausationID:    c.GetHeader(causationHeader),
		RequestID:      c.GetHeader("X-Request-ID"),
	}
}

type authorizeUploadRequest struct {
	OwnerType        string   `json:"ownerType"`
	OwnerReference   string   `json:"ownerReference"`
	DocumentCategory string   `json:"documentCategory"`
	UploadPurpose    string   `json:"uploadPurpose"`
	AllowedMIMETypes []string `json:"allowedMimeTypes"`
	MaxSizeBytes     int64    `json:"maxSizeBytes"`
	RetentionClass   string   `json:"retentionClass"`
	// Visibility defaults to PRIVATE when omitted. DATA_EXPORT may never be
	// anything else, and the use case refuses the combination rather than
	// silently correcting it.
	Visibility string `json:"visibility"`
	// FileName is required for DATA_EXPORT, where the extension must pair
	// exactly with the declared content type.
	FileName string `json:"fileName"`
}

func (h *Handler) authorizeUpload(c *gin.Context) {
	var body authorizeUploadRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, usecases.ErrInvalid)
		return
	}
	payloadVersion, _ := strconv.Atoi(c.GetHeader(payloadVersionHeader))

	grant, err := h.service.AuthorizeUpload(c.Request.Context(), usecases.AuthorizeUploadInput{
		Caller:             callerFrom(c),
		OwnerType:          body.OwnerType,
		OwnerReference:     body.OwnerReference,
		DocumentCategory:   body.DocumentCategory,
		UploadPurpose:      body.UploadPurpose,
		AllowedMIMETypes:   body.AllowedMIMETypes,
		MaxSizeBytes:       body.MaxSizeBytes,
		RetentionClass:     body.RetentionClass,
		Visibility:         body.Visibility,
		FileName:           body.FileName,
		IdempotencyKey:     strings.TrimSpace(c.GetHeader(idempotencyHeader)),
		RequestFingerprint: strings.TrimSpace(c.GetHeader(fingerprintHeader)),
		PayloadVersion:     payloadVersion,
	})
	if err != nil {
		fail(c, err)
		return
	}

	// The upload token is returned once, here. It is not stored in that form
	// and never appears in a log line.
	respond(c, gin.H{
		"uploadAuthorizationId": grant.AuthorizationID,
		"mediaObjectId":         grant.MediaObjectID,
		"uploadToken":           grant.UploadToken,
		"uploadPath":            grant.UploadPath,
		"expiresAt":             grant.ExpiresAt.Format(time.RFC3339),
		"maxSizeBytes":          grant.MaxSizeBytes,
		"allowedMimeTypes":      grant.AllowedMIMETypes,
		"replayed":              grant.Replayed,
	})
}

func (h *Handler) completeUpload(c *gin.Context) {
	content, err := io.ReadAll(c.Request.Body)
	if err != nil {
		fail(c, usecases.ErrInvalid)
		return
	}
	declaredSize, _ := strconv.ParseInt(c.GetHeader("X-Declared-Size"), 10, 64)

	object, err := h.service.CompleteUpload(c.Request.Context(), usecases.CompleteUploadInput{
		Caller:           callerFrom(c),
		MediaObjectID:    c.Param("id"),
		AuthorizationID:  strings.TrimSpace(c.GetHeader("X-Upload-Authorization-ID")),
		UploadToken:      strings.TrimSpace(c.GetHeader("X-Upload-Token")),
		DeclaredMIME:     c.GetHeader("X-Declared-Content-Type"),
		DeclaredSize:     declaredSize,
		DeclaredChecksum: strings.TrimSpace(c.GetHeader("X-Declared-Checksum-SHA256")),
		Content:          content,
	})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, objectView(object))
}

func (h *Handler) readObject(c *gin.Context) {
	object, err := h.service.GetObject(c.Request.Context(), callerFrom(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, objectView(object))
}

func (h *Handler) readHistory(c *gin.Context) {
	entries, err := h.service.History(c.Request.Context(), callerFrom(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	views := make([]gin.H, 0, len(entries))
	for _, entry := range entries {
		views = append(views, gin.H{
			"action":     entry.Action,
			"fromState":  entry.FromState,
			"toState":    entry.ToState,
			"actorType":  entry.ActorType,
			"actorId":    entry.ActorID,
			"occurredAt": entry.OccurredAt.UTC().Format(time.RFC3339),
		})
	}
	respond(c, gin.H{"history": views})
}

func (h *Handler) authorizeDownload(c *gin.Context) {
	grant, err := h.service.AuthorizeDownload(c.Request.Context(), callerFrom(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, gin.H{
		"mediaObjectId": grant.MediaObjectID,
		"downloadToken": grant.Token,
		"downloadPath":  grant.Path,
		"expiresAt":     grant.ExpiresAt.Format(time.RFC3339),
		"contentType":   grant.ContentType,
		"sizeBytes":     grant.SizeBytes,
	})
}

// readContent streams object bytes against a grant.
//
// The token arrives in a header rather than the query string so it does not
// end up in an access log, a proxy log or a browser history.
func (h *Handler) readContent(c *gin.Context) {
	token := strings.TrimSpace(c.GetHeader("X-Download-Token"))
	content, object, err := h.service.ReadContent(c.Request.Context(), c.Param("id"), token)
	if err != nil {
		fail(c, err)
		return
	}
	contentType := object.DetectedMIME
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	// Everything served from the private contract is a download, never
	// something a browser renders. The disposition names the file so it lands
	// with the extension it was validated against, and nosniff stops a browser
	// second-guessing the type it was given — which is what would otherwise
	// let a crafted CSV be treated as HTML and executed in the operator's
	// session.
	disposition := "attachment"
	if name := sanitizeDownloadName(object.FileName); name != "" {
		disposition += `; filename="` + name + `"`
	}
	c.Header("Content-Disposition", disposition)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Data(http.StatusOK, contentType, content)
}

type deletionRequestBody struct {
	Reason string `json:"reason"`
}

func (h *Handler) requestDeletion(c *gin.Context) {
	var body deletionRequestBody
	_ = c.ShouldBindJSON(&body)

	request, err := h.service.RequestDeletion(c.Request.Context(), usecases.RequestDeletionInput{
		Caller:             callerFrom(c),
		MediaObjectID:      c.Param("id"),
		Reason:             body.Reason,
		IdempotencyKey:     strings.TrimSpace(c.GetHeader(idempotencyHeader)),
		RequestFingerprint: strings.TrimSpace(c.GetHeader(fingerprintHeader)),
	})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, deletionView(request))
}

func (h *Handler) readDeletion(c *gin.Context) {
	object, request, err := h.service.DeletionStatus(c.Request.Context(), callerFrom(c), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	payload := gin.H{
		"mediaObjectId": object.ID,
		"deletionState": object.DeletionState,
		"sourceVersion": object.Version,
	}
	if request != nil {
		payload["request"] = deletionView(request)
	}
	respond(c, payload)
}

type retentionRequestBody struct {
	RetentionClass string  `json:"retentionClass"`
	RetentionUntil string  `json:"retentionUntil"`
	LegalHold      *bool   `json:"legalHold"`
	Reason         *string `json:"reason"`
}

func (h *Handler) setRetention(c *gin.Context) {
	var body retentionRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, usecases.ErrInvalid)
		return
	}
	var until *time.Time
	if strings.TrimSpace(body.RetentionUntil) != "" {
		parsed, err := time.Parse(time.RFC3339, body.RetentionUntil)
		if err != nil {
			fail(c, usecases.ErrInvalid)
			return
		}
		until = &parsed
	}
	object, err := h.service.SetRetention(c.Request.Context(), callerFrom(c), c.Param("id"),
		body.RetentionClass, until, body.LegalHold)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, objectView(object))
}

// objectView is the allowlisted object response.
//
// Assembled field by field so a column added later cannot become a response
// field by accident. The storage bucket and object key are deliberately
// absent: a consumer that knew the key would be one configuration mistake away
// from reaching the bytes without a grant.
func objectView(object *models.Object) gin.H {
	view := gin.H{
		"mediaObjectId":         object.ID,
		"organizationId":        object.OrganizationID,
		"ownerService":          object.OwnerService,
		"ownerType":             object.OwnerType,
		"ownerReference":        object.OwnerReference,
		"uploadPurpose":         object.UploadPurpose,
		"visibility":            object.Visibility,
		"fileName":              object.FileName,
		"declaredMime":          object.DeclaredMIME,
		"detectedMime":          object.DetectedMIME,
		"sizeBytes":             object.SizeBytes,
		"checksumSha256":        object.ChecksumSHA256,
		"capturedAt":            optionalTime(object.CapturedAt),
		"sourceTimestamp":       optionalTime(object.SourceTimestamp),
		"captureMethod":         object.CaptureMethod,
		"deviceReference":       object.DeviceReference,
		"actorReference":        object.ActorReference,
		"originalMimeType":      object.OriginalMIMEType,
		"originalSizeBytes":     object.OriginalSizeBytes,
		"integrityHash":         object.IntegrityHash,
		"signatureValidation":   object.SignatureValidation,
		"uploadStatus":          object.UploadStatus,
		"safetyStatus":          object.SafetyStatus,
		"safetyReason":          object.SafetyReason,
		"deletionState":         object.DeletionState,
		"retentionClass":        object.RetentionClass,
		"legalHold":             object.LegalHold,
		"validationFailureCode": object.ValidationFailureCode,
		"sourceVersion":         object.Version,
		"createdAt":             object.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt":             object.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if object.RetentionUntil != nil {
		view["retentionUntil"] = object.RetentionUntil.UTC().Format(time.RFC3339)
	}
	if object.ScannedAt != nil {
		view["scannedAt"] = object.ScannedAt.UTC().Format(time.RFC3339)
	}
	if object.DeletedAt != nil {
		view["deletedAt"] = object.DeletedAt.UTC().Format(time.RFC3339)
	}
	return view
}

func optionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func deletionView(request *models.DeletionRequest) gin.H {
	view := gin.H{
		"deletionRequestId": request.ID,
		"mediaObjectId":     request.MediaObjectID,
		"status":            request.Status,
		"blockReason":       request.BlockReason,
		"attempts":          request.Attempts,
		"createdAt":         request.CreatedAt.UTC().Format(time.RFC3339),
	}
	if request.CompletedAt != nil {
		view["completedAt"] = request.CompletedAt.UTC().Format(time.RFC3339)
	}
	return view
}

// ContentFingerprint is exported for callers that need to compute the same
// request fingerprint the private contract expects.
func ContentFingerprint(parts ...string) string {
	hasher := sha256.New()
	for _, part := range parts {
		_, _ = hasher.Write([]byte(part))
		_, _ = hasher.Write([]byte{0})
	}
	return base64.RawURLEncoding.EncodeToString(hasher.Sum(nil))
}

// sanitizeDownloadName bounds what may appear in a Content-Disposition header.
//
// The stored name was validated on the way in, but a header is a different
// context: a quote or a newline here would let a name break out of the header
// value. Anything outside a conservative set is dropped rather than escaped,
// because a slightly shorter filename is a far better outcome than a header
// injection.
func sanitizeDownloadName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 {
		return ""
	}
	var builder strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
