package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cosmicforge/logistics/shared/go/providercompat"
)

// CompletedWorkSubmitter is the slice of providercompat.Client this package
// calls. Declared locally so the outbox depends on a narrow capability rather
// than on the whole Fleet client.
type CompletedWorkSubmitter interface {
	SubmitCompletedWork(ctx context.Context, request providercompat.CompletedWorkRequest) (providercompat.SubmitResult, error)
}

// TerminalDeliveryError marks a failure that no retry can fix (Section 8
// TAX-016B). The publisher dead-letters it immediately into
// FAILED_TERMINAL rather than burning the whole attempt budget first.
type TerminalDeliveryError struct {
	Code string
	Err  error
}

func (e *TerminalDeliveryError) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Err.Error()
	}
	return e.Code
}

func (e *TerminalDeliveryError) Unwrap() error { return e.Err }

// AmbiguousDeliveryError marks an outcome we never learned (Section 8
// TAX-017A). It is retried, but only ever as a REDELIVERY under the same
// idempotency key, and once the budget is exhausted it becomes
// RECONCILIATION_REQUIRED rather than a plain terminal failure -- because
// Fleet may in fact hold the record.
type AmbiguousDeliveryError struct {
	Code string
	Err  error
}

func (e *AmbiguousDeliveryError) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Err.Error()
	}
	return e.Code
}

func (e *AmbiguousDeliveryError) Unwrap() error { return e.Err }

// FleetCompletedWorkDestination delivers completed-work rows to Fleet's
// canonical ingestion endpoint.
type FleetCompletedWorkDestination struct {
	Client  CompletedWorkSubmitter
	Enabled bool
}

// Deliver submits one completed-work row.
//
// The request is rebuilt ENTIRELY from the stored payload. Nothing is read
// from the live booking here, which is what makes a delivery replayed days
// after the fact reproduce exactly what was completed, under exactly the same
// idempotency key.
func (d FleetCompletedWorkDestination) Deliver(ctx context.Context, record Record) error {
	if !d.Enabled || d.Client == nil {
		// Not the row's fault: leave it queued rather than dead-lettering
		// authoritative completion against a target that is switched off.
		return errors.New("fleet completed-work delivery is not enabled")
	}
	var payload CompletedWorkPayload
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return &TerminalDeliveryError{Code: "payload_decode_failed", Err: err}
	}
	if payload.IdempotencyKey == "" {
		return &TerminalDeliveryError{Code: "missing_idempotency_key"}
	}
	result, err := d.Client.SubmitCompletedWork(ctx, payload.Request())
	switch result.Outcome {
	case providercompat.SubmitAccepted, providercompat.SubmitDuplicate:
		// DUPLICATE is success: it proves Fleet holds exactly one record for
		// this idempotency identity, which is the contract's guarantee.
		return nil
	case providercompat.SubmitTerminal:
		return &TerminalDeliveryError{Code: codeOr(result.Code, "rejected"), Err: err}
	case providercompat.SubmitUnknown:
		return &AmbiguousDeliveryError{Code: codeOr(result.Code, "unknown"), Err: err}
	default:
		if err != nil {
			return fmt.Errorf("fleet completed-work delivery failed (%s): %w", codeOr(result.Code, "retryable"), err)
		}
		return errors.New("fleet completed-work delivery failed: " + codeOr(result.Code, "retryable"))
	}
}

func codeOr(code, fallback string) string {
	if code != "" {
		return code
	}
	return fallback
}

// Router dispatches an outbox record to the destination that owns its target.
//
// This is what lets Vendor lifecycle events and Fleet completed-work rows
// share ONE durable outbox, one worker, one leasing and backoff policy --
// instead of standing up a second outbox framework, which Section 8 TAX-016
// explicitly rules out.
type Router struct {
	byTarget map[string]Destination
}

// NewRouter builds a target-keyed destination router.
func NewRouter(byTarget map[string]Destination) *Router {
	return &Router{byTarget: byTarget}
}

// Deliver routes one record.
func (r *Router) Deliver(ctx context.Context, record Record) error {
	destination, ok := r.byTarget[record.Target]
	if !ok || destination == nil {
		// An unroutable row can never become deliverable by waiting.
		return &TerminalDeliveryError{Code: "no_destination_for_target_" + record.Target}
	}
	return destination.Deliver(ctx, record)
}

// ReconciliationRow is the operator-facing view of a completed-work delivery
// (Section 8 TAX-017).
type ReconciliationRow struct {
	ID                  string     `json:"id"`
	SourceJobID         string     `json:"sourceJobId"`
	SourceAssignmentID  string     `json:"sourceAssignmentId"`
	OrganizationID      string     `json:"organizationId"`
	ProviderID          string     `json:"providerId"`
	IdempotencyKey      string     `json:"idempotencyKey"`
	CorrelationID       string     `json:"correlationId"`
	ReconciliationState string     `json:"reconciliationState"`
	Attempts            int        `json:"attempts"`
	LastErrorCode       string     `json:"lastErrorCode"`
	LastError           string     `json:"lastError"`
	NextRetryAt         *time.Time `json:"nextRetryAt,omitempty"`
	OccurredAt          time.Time  `json:"occurredAt"`
	DeliveredAt         *time.Time `json:"deliveredAt,omitempty"`
}
