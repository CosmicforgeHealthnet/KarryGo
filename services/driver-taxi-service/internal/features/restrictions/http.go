package restrictions

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Register mounts Fleet's restriction surface on group (Taxi mounts it at
// <api-base>/internal/restrictions). boundary is Taxi's existing Fleet
// service-auth boundary -- HMAC v2 signature, caller allowlist and durable
// nonce. With no boundary nothing is mounted: an unauthenticated restriction
// surface would let anyone lift a safety block.
func Register(group *gin.RouterGroup, s *Service, boundary ...gin.HandlerFunc) {
	if group == nil || s == nil || len(boundary) == 0 {
		return
	}
	group.Use(boundary...)
	group.POST("/apply", command(s.Apply))
	group.POST("/clear", command(s.Clear))
	group.GET("/operations/:operationReference", func(c *gin.Context) {
		v, err := s.ReadOperation(c.Request.Context(), c.Param("operationReference"))
		respond(c, v, err)
	})
	group.GET("/:organizationId/:subjectType/:subjectReference", func(c *gin.Context) {
		v, err := s.Read(c.Request.Context(), c.Param("organizationId"), c.Param("subjectType"), c.Param("subjectReference"))
		respond(c, v, err)
	})
}

func command(fn func(context.Context, Command) (OperationView, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in Command
		if c.ShouldBindJSON(&in) != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "INVALID_REQUEST"}})
			return
		}
		v, err := fn(c.Request.Context(), in)
		respond(c, v, err)
	}
}

func respond(c *gin.Context, v any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": v})
		return
	}
	status, code := http.StatusInternalServerError, "RESTRICTION_AUTHORITY_UNAVAILABLE"
	switch {
	case errors.Is(err, ErrInvalid):
		status, code = http.StatusBadRequest, "INVALID_REQUEST"
	case errors.Is(err, ErrNotFound):
		status, code = http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, ErrConflict):
		status, code = http.StatusConflict, "CONFLICT"
	}
	c.JSON(status, gin.H{"success": false, "error": gin.H{"code": code}})
}
