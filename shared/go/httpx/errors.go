package httpx

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/logging"
)

const RequestIDKey = "request_id"

type errorResponse struct {
	Success bool      `json:"success"`
	Error   errorBody `json:"error"`
}

type errorBody struct {
	Code      apperrors.Code             `json:"code"`
	Message   string                     `json:"message"`
	RequestID string                     `json:"request_id,omitempty"`
	Fields    []apperrors.FieldViolation `json:"fields,omitempty"`
	Details   map[string]interface{}     `json:"details,omitempty"`
}

func Abort(c *gin.Context, err error) {
	_ = c.Error(err)
	c.Abort()
}

func RespondError(c *gin.Context, err error) {
	if c.Writer.Written() {
		return
	}

	appErr := apperrors.From(err)
	requestID := GetRequestID(c)

	if appErr.Status >= http.StatusInternalServerError {
		// A dependency error may embed a credential, request body or DSN.
		logging.Structured("error", "http_error", map[string]any{"request_id": LogRequestID(c), "status": appErr.Status, "code": string(appErr.Code)})
	}

	c.JSON(appErr.Status, errorResponse{
		Success: false,
		Error: errorBody{
			Code:      appErr.Code,
			Message:   appErr.Message,
			RequestID: requestID,
			Fields:    appErr.Fields,
			Details:   appErr.Details,
		},
	})
}

func GetRequestID(c *gin.Context) string {
	value, ok := c.Get(RequestIDKey)
	if !ok {
		return ""
	}

	requestID, _ := value.(string)
	return requestID
}

// Preserve the existing caller correlation contract in HTTP responses without
// printing arbitrary header contents into logs. Opaque non-UUID request IDs
// remain correlatable by a deterministic digest, not their potentially secret value.
func LogRequestID(c *gin.Context) string {
	value := GetRequestID(c)
	if id, err := uuid.Parse(value); err == nil {
		return id.String()
	}
	if value == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(value))
	return "request-" + hex.EncodeToString(digest[:12])
}
