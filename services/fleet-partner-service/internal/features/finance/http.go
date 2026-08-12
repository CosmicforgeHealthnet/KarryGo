package finance

import (
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

func RegisterInternalRoutes(group *gin.RouterGroup, service *Service, allowed []string) {
	allow := map[string]bool{}
	for _, v := range allowed {
		allow[v] = true
	}
	group.POST("/finance/events", func(c *gin.Context) {
		if !allow[c.GetString(middleware.ServiceCallerKey)] {
			c.JSON(http.StatusForbidden, gin.H{"status": "REJECTED", "code": "caller_not_allowed"})
			return
		}
		raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "REJECTED", "code": "invalid_envelope"})
			return
		}
		status, err := service.IngestWalletEvent(c, raw)
		if err != nil {
			code := "invalid_envelope"
			httpStatus := http.StatusUnprocessableEntity
			if err == ErrEventConflict {
				code = "payload_hash_mismatch"
				httpStatus = http.StatusConflict
			}
			c.JSON(httpStatus, gin.H{"status": "REJECTED", "code": code})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": status})
	})
	replay := func(projection bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			caller := c.GetString(middleware.ServiceCallerKey)
			if !allow[caller] {
				c.JSON(http.StatusForbidden, gin.H{"status": "REJECTED", "code": "caller_not_allowed"})
				return
			}
			organizationID := c.Query("organizationId")
			if organizationID == "" {
				c.JSON(http.StatusBadRequest, gin.H{"status": "REJECTED", "code": "organization_required"})
				return
			}
			var err error
			if projection {
				err = service.ReplayProjection(c, organizationID, c.Param("id"), caller, c.GetHeader("X-Correlation-ID"))
			} else {
				err = service.ReplayOperation(c, organizationID, c.Param("id"), caller, c.GetHeader("X-Correlation-ID"))
			}
			if err != nil {
				status := http.StatusConflict
				if err == ErrNotFound {
					status = http.StatusNotFound
				}
				c.JSON(status, gin.H{"status": "REJECTED", "code": "replay_not_available"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "PENDING"})
		}
	}
	group.POST("/finance/operations/:id/replay", replay(false))
	group.POST("/finance/projections/:id/replay", replay(true))
}
