package httpx

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"karrygo/backend/internal/platform/apperrors"
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
		log.Printf("request_id=%s status=%d code=%s error=%v", requestID, appErr.Status, appErr.Code, err)
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
