package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type successResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

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
