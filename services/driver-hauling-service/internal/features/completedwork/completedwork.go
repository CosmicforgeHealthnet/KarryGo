// Package completedwork submits organization-managed Hauling completions to
// Fleet's canonical Section 7 ingestion endpoint (HLG-017 .. HLG-020).
//
// It deliberately owns no completion logic. Hauling decides when a job is
// authoritatively complete; this package only records that fact durably and
// delivers it. That separation is what makes HLG-018 true: Hauling's own
// completion never depends on Fleet being online.
package completedwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/providercompat"
)

// Target is the outbox delivery target for canonical completed work. The
// existing Fleet domain-event rows keep their own target, so one outbox
// serves both without either being delivered to the wrong endpoint.
const Target = "fleet-completed-work"

// EventType is the outbox event type carrying one completed-work submission.
const EventType = "hauling.fleet.completed_work"

// AggregateType names the aggregate a completed-work row belongs to.
const AggregateType = "booking"

// Reconciliation states (HLG-020). Stored explicitly rather than derived, so
// an operator query never has to re-implement the worker's own rules to
// answer "what is stuck and why".
const (
	ReconPending                = "PENDING"
	ReconDelivered              = "DELIVERED"
	ReconRetryScheduled         = "RETRY_SCHEDULED"
	ReconFailedTerminal         = "FAILED_TERMINAL"
	ReconReconciliationRequired = "RECONCILIATION_REQUIRED"
)

// Executor is satisfied by both *pgxpool.Pool and pgx.Tx.
type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconnCommandTag, error)
}

// pgconnCommandTag keeps the Executor shape identical to every other
// repository in this service without importing pgconn here.
type pgconnCommandTag = interface {
	RowsAffected() int64
	String() string
}

// IdempotencyKey is the stable identity of one completion (HLG-019).
//
// Derived from the hauling execution id and its authoritative completion
// version, so the SAME completion always produces the SAME key no matter how
// many times delivery is retried, while a genuinely later completion version
// of the same booking would be a distinct record.
func IdempotencyKey(bookingID string, completionVersion int64) string {
	return fmt.Sprintf("hauling-completed-work:%s:%d", bookingID, completionVersion)
}

// Binding is a booking's corporate identity, read at completion time.
type Binding struct {
	ProviderID        string
	OrganizationID    string
	MembershipID      string
	FleetAssignmentID string
	ExecutionMode     string
	CompletionVersion int64
	CorrelationID     string
	FareMinor         int64
	Currency          string
}

// ExecutionModeManaged is the haulage_bookings.execution_mode value written
// for organization-managed work.
const ExecutionModeManaged = "ORGANIZATION_MANAGED"

// Managed reports whether this booking settles through Fleet.
//
// A booking is managed only when it names BOTH its organization and its
// membership, and an executing provider. A partially bound booking is not a
// settlement authority: reporting it would hand Fleet a record it cannot
// resolve.
func (b Binding) Managed() bool {
	return b.ExecutionMode == ExecutionModeManaged &&
		b.OrganizationID != "" && b.MembershipID != "" && b.ProviderID != ""
}

// ReadBinding loads a booking's corporate identity inside the caller's
// transaction, so the snapshot committed alongside the completion is the
// state at completion time and can never drift.
func ReadBinding(ctx context.Context, tx pgx.Tx, bookingID string) (Binding, error) {
	var b Binding
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(provider_id::text,''), COALESCE(corporate_organization_id,''),
		       COALESCE(membership_id,''), COALESCE(fleet_assignment_id,''),
		       COALESCE(execution_mode,''), COALESCE(completion_version,1),
		       COALESCE(fleet_correlation_id,''),
		       COALESCE(fare_final_kobo, fare_estimate_kobo, 0), 'NGN'
		  FROM haulage_bookings WHERE id = $1::uuid`, bookingID).
		Scan(&b.ProviderID, &b.OrganizationID, &b.MembershipID, &b.FleetAssignmentID,
			&b.ExecutionMode, &b.CompletionVersion, &b.CorrelationID, &b.FareMinor, &b.Currency)
	return b, err
}

// RequestFor builds the canonical Fleet completed-work DTO for a completion.
//
// eligibleBasisMinor carries the fare Hauling already settled. Nothing here
// calculates a price: the amount is a snapshot produced by the pricing engine
// long before this point, carried through unchanged.
func RequestFor(bookingID string, binding Binding, completedAt time.Time) providercompat.CompletedWorkRequest {
	currency := binding.Currency
	if currency == "" {
		currency = "NGN"
	}
	return providercompat.CompletedWorkRequest{
		SourceService:      providercompat.SourceServiceHauling,
		SourceJobID:        bookingID,
		SourceAssignmentID: binding.FleetAssignmentID,
		ProviderID:         binding.ProviderID,
		MembershipID:       binding.MembershipID,
		OrganizationID:     binding.OrganizationID,
		CompletedAt:        completedAt.UTC(),
		Currency:           currency,
		EligibleBasisMinor: binding.FareMinor,
		SourceVersion:      binding.CompletionVersion,
		IdempotencyKey:     IdempotencyKey(bookingID, binding.CompletionVersion),
		CorrelationID:      binding.CorrelationID,
	}
}

// Enqueue writes one canonical completed-work delivery in the caller's
// transaction (HLG-018).
//
// Being in the SAME transaction as the booking's final completion is the
// entire point: Hauling's completion never depends on Fleet being reachable,
// and a committed completion can never fail to produce its completed-work
// record.
//
// The ON CONFLICT clause is the structural half of exactly-once: a completion
// enqueued twice -- by a retry, a worker replay, or a duplicated transition --
// results in one row, not two.
func Enqueue(ctx context.Context, tx pgx.Tx, bookingID string, request providercompat.CompletedWorkRequest) error {
	if err := request.Validate(); err != nil {
		// Unsubmittable by construction. Refusing here keeps an unusable row
		// out of the outbox entirely rather than letting it burn the whole
		// retry budget and then dead-letter long after the job is forgotten.
		return fmt.Errorf("completed work: %w", err)
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO hauling_domain_outbox
			(aggregate_type, aggregate_id, event_type, event_version, payload,
			 correlation_id, target, reconciliation_state, idempotency_key)
		VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (target, idempotency_key) WHERE idempotency_key <> ''
		DO NOTHING`,
		AggregateType, bookingID, EventType, request.SourceVersion, payload,
		request.CorrelationID, Target, ReconPending, request.IdempotencyKey)
	return err
}

// EnqueueIfManaged is the one call a completion path makes.
//
// It reads the corporate binding and writes the canonical record only for an
// organization-managed booking. An independent booking produces nothing,
// which is HLG-024's guarantee that independent execution is untouched.
func EnqueueIfManaged(ctx context.Context, tx pgx.Tx, bookingID string, completedAt time.Time) error {
	binding, err := ReadBinding(ctx, tx, bookingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if !binding.Managed() {
		return nil
	}
	return Enqueue(ctx, tx, bookingID, RequestFor(bookingID, binding, completedAt))
}
