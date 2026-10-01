package marketplace

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/providercompat"
)

// CompletedWorkEventType is the outbox event type carrying one canonical
// Fleet completed-work submission.
//
// It rides the EXISTING dispatch_marketplace_outbox rather than a new table
// (Section 9 DSP-009): the same leasing, bounded backoff, delivery-attempt
// records, dead-lettering and ambiguous-response handling that every other
// Fleet delivery already gets. Only the destination endpoint differs, and
// that is the deliverer's job, not a second framework's.
const CompletedWorkEventType = "fleet.completed_work.submit.v1"

// CompletedWorkAggregate is the aggregate type recorded on the outbox row.
const CompletedWorkAggregate = "completed_work"

// CompletedWorkEnqueuer adapts the marketplace outbox to the trip feature's
// narrow enqueue port, so the trip package keeps no dependency on this one.
type CompletedWorkEnqueuer struct {
	repo *Repository
}

// NewCompletedWorkEnqueuer builds the adapter.
func NewCompletedWorkEnqueuer(repo *Repository) *CompletedWorkEnqueuer {
	return &CompletedWorkEnqueuer{repo: repo}
}

// EnqueueCompletedWork appends the delivery in the caller's transaction.
//
// aggregateVersion is the completion's own source version, so the outbox row
// carries the same version the payload does and a replayed completion is
// recognisable as the same one.
func (e *CompletedWorkEnqueuer) EnqueueCompletedWork(ctx context.Context, tx pgx.Tx, request providercompat.CompletedWorkRequest) error {
	// Validated before it is ever queued: an unsubmittable payload would
	// otherwise consume the whole retry budget and then dead-letter, long
	// after the mission it describes has been forgotten.
	if err := request.Validate(); err != nil {
		return err
	}
	// causationID carries the idempotency key so the outbox row itself
	// records the delivery identity a redelivery must reuse (DSP-010).
	return e.repo.PublishEventTo(ctx, tx, "fleet-partner-service", CompletedWorkEventType,
		CompletedWorkAggregate, request.SourceJobID, request.SourceVersion,
		request.CorrelationID, request.IdempotencyKey, request)
}

// CompletedWorkDeliverer wraps the generic event deliverer and sends
// completed-work rows to Fleet's canonical ingestion endpoint instead of the
// generic event ingress.
//
// Everything except the destination is inherited: the worker still claims,
// leases, backs off, records attempts, dead-letters and treats a lost response
// as a redelivery under the same event id. This is the whole point of routing
// inside one deliverer rather than standing up a parallel outbox.
type CompletedWorkDeliverer struct {
	// Events is the existing generic deliverer, used for every other event.
	Events EventDeliverer
	// Fleet submits canonical completed work. Nil means the completed-work
	// integration is not configured.
	Fleet CompletedWorkSubmitter
}

// CompletedWorkSubmitter is the slice of providercompat.Client this deliverer
// calls.
type CompletedWorkSubmitter interface {
	SubmitCompletedWork(ctx context.Context, request providercompat.CompletedWorkRequest) (providercompat.SubmitResult, error)
}

// Configured reports whether a live target exists for ordinary events.
func (d CompletedWorkDeliverer) Configured() bool {
	return d.Events != nil && d.Events.Configured()
}

// Deliver routes one envelope.
func (d CompletedWorkDeliverer) Deliver(ctx context.Context, envelope Envelope) (DeliveryResult, error) {
	if envelope.EventType != CompletedWorkEventType {
		if d.Events == nil {
			return DeliveryResult{Outcome: DeliveryRetryable, Code: "delivery_target_not_configured"}, nil
		}
		return d.Events.Deliver(ctx, envelope)
	}
	if d.Fleet == nil {
		// Not the row's fault: keep it queued rather than dead-lettering an
		// authoritative completion against a target that is switched off.
		return DeliveryResult{Outcome: DeliveryRetryable, Code: "completed_work_target_not_configured"}, nil
	}
	// The request is rebuilt ENTIRELY from the stored payload, never from the
	// live trip: a delivery replayed days later must reproduce exactly what
	// was completed, under exactly the same idempotency key.
	encoded, err := json.Marshal(envelope.Payload)
	if err != nil {
		return DeliveryResult{Outcome: DeliveryRejected, Code: "payload_encode_failed"}, err
	}
	var request providercompat.CompletedWorkRequest
	if err := json.Unmarshal(encoded, &request); err != nil {
		return DeliveryResult{Outcome: DeliveryRejected, Code: "payload_decode_failed"}, err
	}
	request.CompletedAt = request.CompletedAt.UTC()
	result, submitErr := d.Fleet.SubmitCompletedWork(ctx, request)
	return classifyCompletedWorkSubmit(result, submitErr)
}

// classifyCompletedWorkSubmit maps the shared client's outcome onto the
// outbox worker's own vocabulary.
//
// DUPLICATE is a success: it is exactly what a redelivery after a lost
// response returns, and it proves Fleet holds the record once. UNKNOWN stays
// UNKNOWN so MarkDeliveryFailed leaves the event id and payload hash
// untouched, making the retry a REDELIVERY rather than a new completion
// (Section 9 DSP-010).
func classifyCompletedWorkSubmit(result providercompat.SubmitResult, err error) (DeliveryResult, error) {
	switch result.Outcome {
	case providercompat.SubmitAccepted:
		return DeliveryResult{Outcome: DeliveryAccepted, Code: result.Code}, nil
	case providercompat.SubmitDuplicate:
		return DeliveryResult{Outcome: DeliveryDuplicate, Code: result.Code}, nil
	case providercompat.SubmitTerminal:
		return DeliveryResult{Outcome: DeliveryRejected, Code: codeOr(result.Code, "rejected")}, err
	case providercompat.SubmitUnknown:
		return DeliveryResult{Outcome: DeliveryUnknown, Code: codeOr(result.Code, "transport_failure")}, err
	default:
		return DeliveryResult{Outcome: DeliveryRetryable, Code: codeOr(result.Code, "retryable")}, err
	}
}

func codeOr(code, fallback string) string {
	if code != "" {
		return code
	}
	return fallback
}
