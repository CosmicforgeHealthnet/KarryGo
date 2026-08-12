package restrictions

import (
	"context"
	"errors"
	"net/http"

	"cosmicforge/logistics/services/hauling-service/internal/features/fleetauthority"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

func Register(root *gin.RouterGroup, s *Service, authority *fleetauthority.Handler, verifier *serviceauth.Verifier, allowed []string) {
	g := root.Group("/internal/restrictions")
	g.Use(fleetauthority.Boundary(authority, verifier, allowed)...)
	g.POST("/apply", command(s.Apply))
	g.POST("/clear", command(s.Clear))
	g.GET("/operations/:operationReference", func(c *gin.Context) { v, e := s.ReadOperation(c, c.Param("operationReference")); respond(c, v, e) })
	g.GET("/:organizationId/:subjectType/:subjectReference", func(c *gin.Context) {
		v, e := s.Read(c, c.Param("organizationId"), c.Param("subjectType"), c.Param("subjectReference"))
		respond(c, v, e)
	})
}
func command(fn func(context.Context, Command) (OperationView, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in Command
		if c.ShouldBindJSON(&in) != nil {
			c.JSON(400, gin.H{"error": gin.H{"code": "INVALID_REQUEST"}})
			return
		}
		v, e := fn(c, in)
		respond(c, v, e)
	}
}
func respond(c *gin.Context, v any, e error) {
	if e == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": v})
		return
	}
	switch {
	case errors.Is(e, ErrInvalid):
		c.JSON(400, gin.H{"error": gin.H{"code": "INVALID_REQUEST"}})
	case errors.Is(e, ErrNotFound):
		c.JSON(404, gin.H{"error": gin.H{"code": "NOT_FOUND"}})
	case errors.Is(e, ErrConflict):
		c.JSON(409, gin.H{"error": gin.H{"code": "CONFLICT"}})
	default:
		c.JSON(500, gin.H{"error": gin.H{"code": "RESTRICTION_AUTHORITY_UNAVAILABLE"}})
	}
}
