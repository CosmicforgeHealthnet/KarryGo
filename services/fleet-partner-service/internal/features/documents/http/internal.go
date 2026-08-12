package http

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
)

// The media event ingestion route.
//
// It sits behind the internal route class, which has already applied HMAC
// service authentication (including timestamp freshness) and the nonce replay
// gate. What this adds is the per-route caller allowlist: holding a valid
// service secret proves who you are, not that you may publish document events.

// InternalHandler serves the service-to-service document surface.
type InternalHandler struct {
	service *usecases.Service
	allowed map[string]bool
}

// NewInternalHandler builds the handler with a caller allowlist.
func NewInternalHandler(service *usecases.Service, allowedCallers []string) *InternalHandler {
	allowed := map[string]bool{}
	for _, caller := range allowedCallers {
		if name := strings.TrimSpace(caller); name != "" {
			allowed[name] = true
		}
	}
	return &InternalHandler{service: service, allowed: allowed}
}

// RegisterInternalRoutes mounts the media event ingestion route.
func RegisterInternalRoutes(group *gin.RouterGroup, handler *InternalHandler) {
	group.POST("/documents/media-events", handler.ingestMediaEvent)
}

// ingestMediaEvent applies one media event.
//
// The status codes are load-bearing for the publisher's retry logic:
//
//	200 - applied, duplicated, older or ignored; stop retrying
//	409 - payload conflict; retrying the same body will never succeed
//	422 - malformed envelope; the same body will never succeed
//	403 - caller not allowlisted
func (h *InternalHandler) ingestMediaEvent(c *gin.Context) {
	caller := c.GetString(middleware.ServiceCallerKey)
	if !h.allowed[caller] {
		c.JSON(http.StatusForbidden, gin.H{"status": "REJECTED", "code": "caller_not_allowed"})
		return
	}

	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "REJECTED", "code": "invalid_envelope"})
		return
	}

	outcome, err := h.service.IngestMediaEvent(c.Request.Context(), raw)
	if err != nil {
		switch {
		case errors.Is(err, usecases.ErrEventConflict):
			c.JSON(http.StatusConflict, gin.H{"status": "REJECTED", "code": "payload_conflict"})
		case errors.Is(err, usecases.ErrEventInvalid):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"status": "REJECTED", "code": "invalid_envelope"})
		default:
			// An internal failure is retryable: the publisher should redeliver
			// rather than treat the event as consumed.
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "RETRY", "code": "ingestion_unavailable"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": outcome})
}
