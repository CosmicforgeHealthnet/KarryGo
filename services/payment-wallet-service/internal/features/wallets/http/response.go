package wallethttp

import (
	nethttp "net/http"

	"github.com/gin-gonic/gin"
)

type successResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
}

func respond(c *gin.Context, status int, data interface{}) {
	c.JSON(status, successResponse{
		Success: true,
		Data:    data,
	})
}

func respondOK(c *gin.Context, data interface{}) {
	respond(c, nethttp.StatusOK, data)
}
