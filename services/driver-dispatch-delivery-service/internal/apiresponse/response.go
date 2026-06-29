// Package apiresponse provides the success-envelope helper for this service.
//
// The shared platform convention emits success responses inline as
// { "success": true, "data": ... }. This service additionally carries a
// human-readable "message" on several endpoints, so it keeps a small local
// helper rather than reshaping shared/go/httpx.
package apiresponse

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type successResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// RespondSuccess writes a standard success envelope with an optional message.
func RespondSuccess(c *gin.Context, status int, message string, data interface{}) {
	if status == 0 {
		status = http.StatusOK
	}

	c.JSON(status, successResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}
