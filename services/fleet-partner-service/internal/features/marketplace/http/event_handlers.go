package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
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
//	200 {"status":"ACCEPTED"|"DUPLICATE"|"IGNORED","eventId":...}
//	400 {"status":"REJECTED","code":...}   permanent — do not retry
//	403 {"status":"REJECTED","code":"caller_not_allowed"}
//	500 {"status":"ERROR","code":...}      transient — retry
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

	c.JSON(http.StatusOK, gin.H{
		"status":  result.Status,
		"eventId": result.EventID,
		"code":    result.Code,
	})
}

// respondIngestError writes the narrow rejection contract.
//
// Only the code is returned. No message is echoed, because the producer stores
// what comes back and an echoed payload fragment would put source data into
// another service's error columns.
func respondIngestError(c *gin.Context, status int, code string) {
	label := "REJECTED"
	if status >= http.StatusInternalServerError {
		label = "ERROR"
	}
	c.AbortWithStatusJSON(status, gin.H{"status": label, "code": code})
}
