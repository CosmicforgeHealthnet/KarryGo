package finance

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
)

// rejection builds a standard error whose code is the stable machine code the
// route has always reported.
func rejection(status int, code, message string) *apperrors.Error {
	return apperrors.New(status, apperrors.Code(code), message, nil)
}

// RegisterInternalRoutes mounts wallet event ingestion and the operator replay
// routes. Responses use the standard envelopes. No top-level alias is kept:
// payment-wallet-service's HTTPPublisher classifies a delivery by HTTP status
// alone and discards the body, and the replay routes have no service caller.
func RegisterInternalRoutes(group *gin.RouterGroup, service *Service, allowed []string) {
	allow := map[string]bool{}
	for _, v := range allowed {
		allow[v] = true
	}
	group.POST("/finance/events", func(c *gin.Context) {
		if !allow[c.GetString(middleware.ServiceCallerKey)] {
			respond.Error(c, rejection(http.StatusForbidden, "caller_not_allowed",
				"Service is not allowed to call this endpoint."))
			return
		}
		raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
		if err != nil {
			respond.Error(c, rejection(http.StatusBadRequest, "invalid_envelope",
				"The event body could not be read."))
			return
		}
		status, err := service.IngestWalletEvent(c, raw)
		if err != nil {
			if err == ErrEventConflict {
				respond.Error(c, rejection(http.StatusConflict, "payload_hash_mismatch",
					"This event ID was already received with a different payload."))
				return
			}
			respond.Error(c, rejection(http.StatusUnprocessableEntity, "invalid_envelope",
				"The event envelope is invalid."))
			return
		}
		respond.OK(c, gin.H{"status": status})
	})
	replay := func(projection bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			caller := c.GetString(middleware.ServiceCallerKey)
			if !allow[caller] {
				respond.Error(c, rejection(http.StatusForbidden, "caller_not_allowed",
					"Service is not allowed to call this endpoint."))
				return
			}
			organizationID := c.Query("organizationId")
			if organizationID == "" {
				respond.Error(c, rejection(http.StatusBadRequest, "organization_required",
					"organizationId is required."))
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
				respond.Error(c, rejection(status, "replay_not_available",
					"This record cannot be replayed."))
				return
			}
			respond.OK(c, gin.H{"status": "PENDING"})
		}
	}
	group.POST("/finance/operations/:id/replay", replay(false))
	group.POST("/finance/projections/:id/replay", replay(true))
}
