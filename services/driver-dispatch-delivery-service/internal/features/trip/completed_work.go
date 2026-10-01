package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/providercompat"
)

// CompletedWorkEnqueuer appends one completed-work delivery to Dispatch's
// EXISTING durable outbox, inside the caller's transaction.
//
// Section 9 DSP-009: Dispatch already has outbox leasing, bounded backoff,
// delivery-attempt records, dead-lettering and ambiguous-response handling.
// This is an interface rather than a direct dependency so the trip repository
// stays free of the marketplace package, while the implementation is a thin
// adapter over that outbox -- no second framework.
type CompletedWorkEnqueuer interface {
	EnqueueCompletedWork(ctx context.Context, tx pgx.Tx, request providercompat.CompletedWorkRequest) error
}

// WithCompletedWorkEnqueuer attaches the Fleet completed-work enqueuer.
//
// Without one the repository behaves exactly as before, which is what keeps
// every existing test -- none of which wires an enqueuer -- describing
// unchanged behaviour.
func (r *PostgresRepository) WithCompletedWorkEnqueuer(enqueuer CompletedWorkEnqueuer) *PostgresRepository {
	r.completedWork = enqueuer
	return r
}

// CompletedWorkIdempotencyKey is the stable identity of one mission
// completion (Section 9 DSP-010).
//
// Derived from the trip and its authoritative completion version, so every
// redelivery of the same completion carries the same key and Fleet records
// exactly one canonical result. A genuinely later completion version of the
// same trip would be a distinct record.
func CompletedWorkIdempotencyKey(tripID string, sourceVersion int64) string {
	return fmt.Sprintf("dispatch-completed-work:%s:%d", tripID, sourceVersion)
}

// corporateBinding is the trip's organization-managed identity, read inside
// the completion transaction.
type corporateBinding struct {
	ProviderID        string
	OrganizationID    string
	MembershipID      string
	FleetAssignmentID string
	ExecutionMode     string
	SourceVersion     int64
	CorrelationID     string
	FareAmount        int64
	Currency          string
}

// Managed reports whether this trip settles through Fleet.
//
// A trip is managed only when it names BOTH its organization and its
// membership: a partially bound trip is not a settlement authority, and
// reporting it as completed work would give Fleet a record it cannot resolve.
func (b corporateBinding) Managed() bool {
	return b.ExecutionMode == executionModeOrganizationManaged &&
		b.OrganizationID != "" && b.MembershipID != "" && b.ProviderID != ""
}

// executionModeOrganizationManaged is the trips.execution_mode value written
// by the marketplace execution-start path for corporate work.
const executionModeOrganizationManaged = "ORGANIZATION_MANAGED"

// readCorporateBinding loads the corporate identity for a trip inside the
// caller's transaction, so the snapshot committed alongside the completion is
// the state at completion time and can never drift.
func readCorporateBinding(ctx context.Context, tx pgx.Tx, tripID string) (corporateBinding, error) {
	var b corporateBinding
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(provider_id::text,''), COALESCE(organization_id,''),
		       COALESCE(organization_membership_id,''),
		       COALESCE(fleet_assignment_id,''), COALESCE(execution_mode,''),
		       COALESCE(source_version,1), COALESCE(fleet_correlation_id,''),
		       COALESCE(fare_amount,0), COALESCE(currency,'NGN')
		  FROM trips WHERE id = $1::uuid`, tripID).
		Scan(&b.ProviderID, &b.OrganizationID, &b.MembershipID, &b.FleetAssignmentID, &b.ExecutionMode,
			&b.SourceVersion, &b.CorrelationID, &b.FareAmount, &b.Currency)
	return b, err
}

// enqueueCompletedWorkIfManaged writes the canonical Fleet completed-work
// delivery in the SAME transaction as the mission's authoritative completion.
//
// This is what makes Dispatch's completion independent of Fleet being online
// (Section 9 DSP-008/DSP-009): the delivery row is durable the moment the
// mission is complete, and the existing outbox worker sends it later.
//
// It is deliberately called ONLY from the completion transition. An accepted,
// picked-up, en-route or cancelled mission produces nothing, because none of
// those is an authoritative completion.
func enqueueCompletedWorkIfManaged(ctx context.Context, tx pgx.Tx, enqueuer CompletedWorkEnqueuer,
	tripID string, completedAt time.Time) error {
	if enqueuer == nil {
		return nil
	}
	binding, err := readCorporateBinding(ctx, tx, tripID)
	if err != nil {
		return err
	}
	if !binding.Managed() {
		return nil
	}
	currency := binding.Currency
	if currency == "" {
		currency = "NGN"
	}
	return enqueuer.EnqueueCompletedWork(ctx, tx, providercompat.CompletedWorkRequest{
		SourceService:      providercompat.SourceServiceDispatch,
		SourceJobID:        tripID,
		SourceAssignmentID: binding.FleetAssignmentID,
		ProviderID:         binding.ProviderID,
		MembershipID:       binding.MembershipID,
		OrganizationID:     binding.OrganizationID,
		CompletedAt:        completedAt.UTC(),
		Currency:           currency,
		// The fare Dispatch already settled. Nothing is priced here: this is
		// a snapshot of an amount authorized long before completion.
		EligibleBasisMinor: binding.FareAmount,
		SourceVersion:      binding.SourceVersion,
		IdempotencyKey:     CompletedWorkIdempotencyKey(tripID, binding.SourceVersion),
		CorrelationID:      binding.CorrelationID,
	})
}

// IsOrganizationManaged reports whether a trip settles through Fleet.
//
// Read from the trip's OWN persisted execution_mode, fixed at execution
// start, rather than from a live Fleet lookup at completion time -- see
// ManagedRouteReader's own doc comment for why that distinction is the whole
// point of Section 9 DSP-014.
//
// The organization and membership are checked too: a trip that claims managed
// execution but names neither is not a settlement authority, and must not be
// withheld from the independent path on that claim alone.
func (r *PostgresRepository) IsOrganizationManaged(ctx context.Context, tripID string) (bool, error) {
	var managed bool
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(execution_mode,'') = $2
		   AND COALESCE(organization_id,'') <> ''
		   AND COALESCE(organization_membership_id,'') <> ''
		  FROM trips WHERE id = $1::uuid`, tripID, executionModeOrganizationManaged).Scan(&managed)
	return managed, err
}
