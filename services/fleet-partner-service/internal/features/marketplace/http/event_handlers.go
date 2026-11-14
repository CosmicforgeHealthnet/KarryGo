package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
)

// maxEventBodyBytes bounds a single delivered event.
//
// An unbounded read on an authenticated endpoint is still a way to exhaust
// memory; a legitimate marketplace event is a few kilobytes.
const maxEventBodyBytes = 256 << 10

// RegisterEventIngestionRoutes mounts the marketplace event ingestion route.
//
// The caller applies service authentication to the group; the per-route
// allowlist below decides which authenticated services may publish events.
// Proving who you are is not the same as being permitted to write here.
func RegisterEventIngestionRoutes(group *gin.RouterGroup, handler *InternalHandler) {
	group.POST("/events", handler.requireAllowedCaller, handler.IngestEvent)
}

// IngestEvent consumes one marketplace event from a source service.
//
// The response contract is deliberately narrow and stable:
//
//	200 data {"status":"ACCEPTED"|"DUPLICATE"|"IGNORED","eventId":...}
//	400 error.code=...                     permanent — do not retry
//	403 error.code="caller_not_allowed"
//	500 error.code=...                     transient — retry
//
// Bodies use the standard envelopes; see the alias notes below for the
// deprecated top-level status/eventId/code fields still emitted.
//
// The producer classifies retry-versus-dead-letter from this alone, so a
// transient failure must never surface as 400 and a permanent rejection must
// never surface as 500.
func (h *InternalHandler) IngestEvent(c *gin.Context) {
	// The sourceService is taken from the VERIFIED signature, not from the
	// body. A caller must not be able to attribute its events to another
	// service by editing a JSON field.
	raw, _ := c.Get("service_name")
	callerService, _ := raw.(string)

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxEventBodyBytes+1))
	if err != nil {
		respondIngestError(c, http.StatusInternalServerError, "body_read_failed")
		return
	}
	if len(body) > maxEventBodyBytes {
		respondIngestError(c, http.StatusBadRequest, "payload_too_large")
		return
	}

	var event usecases.InboundEvent
	if err := json.Unmarshal(body, &event); err != nil {
		respondIngestError(c, http.StatusBadRequest, "invalid_envelope")
		return
	}
	if event.SourceService != "" && event.SourceService != callerService {
		respondIngestError(c, http.StatusForbidden, "source_service_mismatch")
		return
	}
	event.SourceService = callerService

	result, err := h.service.IngestEvent(c.Request.Context(), body, event)
	if err != nil {
		var ingestErr *usecases.IngestError
		if errors.As(err, &ingestErr) {
			status := ingestErr.Status
			if status == 0 {
				status = http.StatusBadRequest
			}
			respondIngestError(c, status, ingestErr.Code)
			return
		}
		// Unclassified: the transaction rolled back, so nothing was committed
		// and a retry of the same event is safe.
		respondIngestError(c, http.StatusInternalServerError, "ingest_failed")
		return
	}

	receipt := gin.H{
		"status":  result.Status,
		"eventId": result.EventID,
		"code":    result.Code,
	}
	// Deprecated aliases (FLEET-6h): status/eventId/code are repeated at the
	// top level because driver-dispatch-delivery-service's
	// HTTPEventDeliverer read "status" (DUPLICATE) and "code" there before it
	// learned the envelope. Remove once every dispatch binary in service reads
	// data.status / data.code.
	respond.JSONWithAliases(c, http.StatusOK, receipt, receipt)
}

// respondIngestError writes the narrow rejection contract.
//
// Only the code is returned. No message is echoed, because the producer stores
// what comes back and an echoed payload fragment would put source data into
// another service's error columns: the standard error envelope's message is a
// fixed, payload-free sentence.
func respondIngestError(c *gin.Context, status int, code string) {
	label := "REJECTED"
	message := "The event was rejected."
	if status >= http.StatusInternalServerError {
		label = "ERROR"
		message = "The event could not be processed. Retry the same event."
	}
	appErr := apperrors.New(status, apperrors.Code(code), message, nil)
	appErr.Retryable = status >= http.StatusInternalServerError
	// Deprecated aliases (FLEET-6h): top-level status/code are kept because
	// driver-dispatch-delivery-service's HTTPEventDeliverer and
	// driver-taxi-service's FleetMarketplaceDestination read the rejection
	// "code" there (dispatch also decides retry-versus-dead-letter from it)
	// before they learned error.code. Remove once both services run the
	// envelope-tolerant decoder everywhere.
	respond.ErrorWithAliases(c, appErr, map[string]any{"status": label, "code": code})
}
