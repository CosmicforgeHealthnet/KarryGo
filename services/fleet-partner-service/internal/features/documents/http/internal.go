package http

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
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
		respond.Error(c, apperrors.New(http.StatusForbidden, "caller_not_allowed",
			"Service is not allowed to call this endpoint.", nil))
		return
	}

	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		respond.Error(c, apperrors.New(http.StatusBadRequest, "invalid_envelope",
			"The event body could not be read.", nil))
		return
	}

	outcome, err := h.service.IngestMediaEvent(c.Request.Context(), raw)
	if err != nil {
		switch {
		case errors.Is(err, usecases.ErrEventConflict):
			respond.Error(c, apperrors.New(http.StatusConflict, "payload_conflict",
				"This event ID was already received with a different payload.", nil))
		case errors.Is(err, usecases.ErrEventInvalid):
			respond.Error(c, apperrors.New(http.StatusUnprocessableEntity, "invalid_envelope",
				"The event envelope is invalid.", nil))
		default:
			// An internal failure is retryable: the publisher should redeliver
			// rather than treat the event as consumed.
			unavailable := apperrors.New(http.StatusServiceUnavailable, "ingestion_unavailable",
				"Media event ingestion is temporarily unavailable.", err)
			unavailable.Retryable = true
			respond.Error(c, unavailable)
		}
		return
	}
	// No top-level alias: media-file-service's publisher classifies the
	// delivery by HTTP status alone and discards the body.
	respond.OK(c, gin.H{"status": outcome})
}
