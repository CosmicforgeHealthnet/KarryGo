package journeyshare

import (
	"context"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
)

type ReadFunc func(context.Context, string, string) (Projection, error)

func RegisterRead(root *gin.Engine, path string, read ReadFunc, boundary ...gin.HandlerFunc) {
	handlers := append([]gin.HandlerFunc{}, boundary...)
	handlers = append(handlers, func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		customer := c.Query("customer_id")
		if customer == "" {
			httpx.RespondError(c, apperrors.BadRequest("Customer assertion is required.", nil))
			return
		}
		p, err := read(c, c.Param("sourceRef"), customer)
		if err != nil {
			httpx.RespondError(c, err)
			return
		}
		c.JSON(200, gin.H{"success": true, "data": p})
	})
	root.GET(path, handlers...)
}
