package restrictions

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func Register(group *gin.RouterGroup, service *Service, verifier *serviceauth.Verifier,
	nonces authority.NonceClaimer, allowed []string) {
	group.Use(authority.ServiceBoundary(verifier, nonces, allowed)...)
	group.POST("/apply", command(service.Apply))
	group.POST("/clear", command(service.Clear))
	group.GET("/operations/:operationReference", func(c *gin.Context) {
		v, err := service.ReadOperation(c, c.Param("operationReference"))
		respond(c, v, err)
	})
	group.GET("/:organizationId/:subjectType/:subjectReference", func(c *gin.Context) {
		v, err := service.Read(c, c.Param("organizationId"), c.Param("subjectType"), c.Param("subjectReference"))
		respond(c, v, err)
	})
}

func command(fn func(context.Context, Command) (OperationView, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in Command
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_REQUEST"}})
			return
		}
		v, err := fn(c, in)
		respond(c, v, err)
	}
}

func respond(c *gin.Context, value any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": value})
		return
	}
	switch {
	case errors.Is(err, ErrInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_REQUEST"}})
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND"}})
	case errors.Is(err, ErrConflict):
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "CONFLICT"}})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "RESTRICTION_AUTHORITY_UNAVAILABLE"}})
	}
}
