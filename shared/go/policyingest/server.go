package policyingest

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/events"
)

// RegisterRoutes mounts the publication sink on a group the caller has ALREADY
// protected with its own HMAC verifier, caller allowlist and replay defence.
//
// Like the capability and preview contracts, this installs no auth of its own.
// Each service's internal boundary differs -- different nonce stores, different
// allowlists -- and a shared package quietly installing a weaker one is how a
// surface ends up less protected than its neighbours. Here that would be
// especially bad: this endpoint decides which vehicles may take which work.
func RegisterRoutes(group *gin.RouterGroup, ingestor *Ingestor) {
	if group == nil || ingestor == nil {
		return
	}
	group.POST(IngressPath, func(c *gin.Context) {
		raw, err := io.ReadAll(io.LimitReader(c.Request.Body, defaultMaxDocument+1))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false,
				"error": gin.H{"code": "validation_failed", "message": "Publication body could not be read."}})
			return
		}

		var envelope events.Envelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false,
				"error": gin.H{"code": "validation_failed", "message": "Publication envelope is invalid."}})
			return
		}

		result, err := ingestor.Ingest(c.Request.Context(), envelope)
		if err != nil {
			// A store failure is the only 5xx here. Everything the publisher
			// could have done wrong is reported as a 200 with an outcome, so
			// a genuine infrastructure problem stays distinguishable from a
			// bad message -- and only the first is worth retrying blindly.
			c.JSON(http.StatusInternalServerError, gin.H{"success": false,
				"error": gin.H{"code": "internal_error", "message": "Publication could not be processed."}})
			return
		}

		// Every decided outcome is 200, including REJECTED. The publisher is
		// an at-least-once pipeline: a 4xx on a malformed message would make
		// it retry that message forever, and a 4xx on a duplicate would make
		// it retry a message that already succeeded. The outcome field is the
		// answer; the status code only says whether we got that far.
		status := http.StatusOK
		if result.Outcome == OutcomeReconciliationRequired {
			// The one exception: ACCEPTED tells the publisher we have taken
			// custody of this publication but have not finished applying it.
			status = http.StatusAccepted
		}
		c.JSON(status, gin.H{"success": true, "data": result})
	})
}

// ErrorOutcome maps an ingestion error to a stable outcome for callers that
// drive the Ingestor themselves (a Redis consumer, a test harness) rather than
// through the HTTP sink.
func ErrorOutcome(err error) (Outcome, string) {
	switch {
	case err == nil:
		return OutcomeApplied, ReasonApplied
	case errors.Is(err, ErrChecksumMismatch):
		return OutcomeRejected, ReasonChecksumMismatch
	case errors.Is(err, ErrUnsupportedSchema):
		return OutcomeRejected, ReasonUnsupportedSchema
	case errors.Is(err, ErrInvalidPublication):
		return OutcomeRejected, ReasonMalformedDocument
	default:
		return OutcomeReconciliationRequired, ReasonReadBackUnavailable
	}
}
