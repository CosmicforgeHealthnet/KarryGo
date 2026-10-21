package partnerauth

import "github.com/gin-gonic/gin"

// ErrorResponse is the standard Partner API error envelope.
//
//	{ "success": false, "error": { "code": "...", "message": "...", "fields": [] } }
type ErrorResponse struct {
	Success bool        `json:"success"`
	Error   ErrorDetail `json:"error"`
}

// ErrorDetail is the error body of a Partner API error response.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Fields  []any  `json:"fields"`
}

// WriteError writes a Partner API error envelope and aborts the request.
// It is shared by the auth middleware and the partner route handlers so the
// external error shape stays consistent.
func WriteError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, ErrorResponse{
		Success: false,
		Error: ErrorDetail{
			Code:    code,
			Message: message,
			Fields:  []any{},
		},
	})
}
