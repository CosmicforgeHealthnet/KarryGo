package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cosmicforge/logistics/shared/go/providercompat"
)

// Delivery targets. The existing taxi_outbox is shared by Vendor lifecycle
// events and Fleet completed-work rows; the target column is what keeps one
// durable mechanism from delivering a row to the wrong consumer.
const (
	// TargetVendor is the Vendor Commerce lifecycle ingress. It is the
	// default for every pre-existing row, so this extension changes nothing
	// about Taxi Goods delivery.
	TargetVendor = "vendor-service"
	// TargetCustomer is customer-service's movement ingress: the read-model
	// owner of a customer's unified movement history.
	TargetCustomer = "customer-service"
	// TargetFleet is the canonical Fleet completed-work ingestion endpoint.
	TargetFleet = "fleet-partner-service"
)

// EventFleetCompletedWork is the outbox event type carrying one canonical
// Fleet completed-work submission.
const EventFleetCompletedWork = "taxi.fleet.completed_work"

// Reconciliation states (Section 8 TAX-017). These are an operator-facing
// projection of the row's delivery state, stored explicitly rather than
// derived, so an operator query never has to re-implement the worker's own
// rules to answer "what is stuck and why".
const (
	// ReconPending is enqueued, not yet attempted.
	ReconPending = "PENDING"
	// ReconDelivered means Fleet holds the record.
	ReconDelivered = "DELIVERED"
	// ReconRetryScheduled means a retryable failure with a future retry.
	ReconRetryScheduled = "RETRY_SCHEDULED"
	// ReconFailedTerminal means Fleet authoritatively rejected it; no retry
	// can succeed and a human has to act.
	ReconFailedTerminal = "FAILED_TERMINAL"
	// ReconReconciliationRequired means the attempt budget is exhausted after
	// AMBIGUOUS outcomes: Fleet may or may not hold the record, so it must be
	// reconciled rather than silently retried or silently dropped.
	ReconReconciliationRequired = "RECONCILIATION_REQUIRED"
)

// CompletedWorkPayload is the immutable snapshot persisted with the outbox
// row (Section 8 TAX-016A).
//
// Everything needed to rebuild the outbound request lives here. The worker
// never re-reads mutable runtime state at delivery time, so a booking that is
// later amended, refunded or re-opened cannot change what was reported as
// completed -- and a delivery replayed days later reproduces byte-identical
// content under the same idempotency key.
type CompletedWorkPayload struct {
	SourceService      string    `json:"sourceService"`
	SourceJobID        string    `json:"sourceJobId"`
	SourceAssignmentID string    `json:"sourceAssignmentId"`
	ProviderID         string    `json:"providerId"`
	MembershipID       string    `json:"membershipId"`
	OrganizationID     string    `json:"organizationId"`
	CompletedAt        time.Time `json:"completedAt"`
	Currency           string    `json:"currency"`
	EligibleBasisMinor int64     `json:"eligibleBasisMinor"`
	SourceVersion      int64     `json:"sourceVersion"`
	IdempotencyKey     string    `json:"idempotencyKey"`
	CorrelationID      string    `json:"correlationId"`
}

// Request converts the stored snapshot into the canonical Fleet DTO.
func (p CompletedWorkPayload) Request() providercompat.CompletedWorkRequest {
	return providercompat.CompletedWorkRequest{
		SourceService: p.SourceService, SourceJobID: p.SourceJobID,
		SourceAssignmentID: p.SourceAssignmentID, ProviderID: p.ProviderID,
		MembershipID: p.MembershipID, OrganizationID: p.OrganizationID,
		CompletedAt: p.CompletedAt.UTC(), Currency: p.Currency,
		EligibleBasisMinor: p.EligibleBasisMinor, SourceVersion: p.SourceVersion,
		IdempotencyKey: p.IdempotencyKey, CorrelationID: p.CorrelationID,
	}
}

// CompletedWorkIdempotencyKey is the stable identity of one completion.
//
// It is derived from the booking and its authoritative completion version, so
// the SAME completion always produces the SAME key no matter how many times
// delivery is retried (TAX-016A), while a genuinely later completion version
// of the same booking would be a distinct record.
func CompletedWorkIdempotencyKey(bookingID string, sourceVersion int64) string {
	return fmt.Sprintf("taxi-completed-work:%s:%d", bookingID, sourceVersion)
}

// WriteCompletedWork appends one Fleet completed-work row in the caller's
// transaction (Section 8 TAX-016).
//
// Being in the SAME transaction as the booking's final completion is the
// entire point: Taxi's completion never depends on Fleet being reachable, and
// a committed completion can never fail to produce its completed-work record.
//
// The ON CONFLICT clause is the structural half of exactly-once: the unique
// index on (target, idempotency_key) means a completion enqueued twice --
// by a retry, a worker replay, or a duplicated transition -- results in one
// row, not two.
func WriteCompletedWork(ctx context.Context, exec Executor, payload CompletedWorkPayload) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("taxi outbox: encode completed work: %w", err)
	}
	_, err = exec.Exec(ctx, `
		INSERT INTO taxi_outbox
			(aggregate_type, aggregate_id, event_type, payload, target,
			 reconciliation_state, idempotency_key)
		VALUES ('booking', $1::uuid, $2, $3, $4, $5, $6)
		ON CONFLICT (target, idempotency_key) WHERE idempotency_key <> ''
		DO NOTHING`,
		payload.SourceJobID, EventFleetCompletedWork, encoded,
		TargetFleet, ReconPending, payload.IdempotencyKey)
	return err
}
