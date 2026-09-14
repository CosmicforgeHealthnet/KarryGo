package bookingusecases

import (
	"context"

	"cosmicforge/logistics/shared/go/logging"

	"cosmicforge/logistics/services/hauling-service/internal/features/completedwork"
)

// SettlementRoute is the decision every money-moving completion path makes
// exactly once (HLG-025).
type SettlementRoute string

const (
	// RouteIndependent is the legacy-compatible direct-to-provider wallet
	// settlement. Unchanged behaviour for every independent hauling provider.
	RouteIndependent SettlementRoute = "INDEPENDENT"

	// RouteManaged is the Fleet managed-settlement path. Hauling's
	// responsibility ends at reporting completed work; Fleet resolves
	// compensation and Section 12 instructs Wallet.
	RouteManaged SettlementRoute = "ORGANIZATION_MANAGED"
)

// ManagedSettlementConfig gates the managed path (HLG-025).
//
// Until Section 12 exists the managed path deliberately does NOT post to
// Wallet. It does not fake success, does not double-pay, and does not compute
// any corporate split inside Hauling. What it does is refuse to let corporate
// work fall through to the legacy direct-to-provider payout.
type ManagedSettlementConfig struct {
	// Enabled turns managed settlement isolation on. Default false preserves
	// the pre-Section-10 behaviour exactly; turning it on stops corporate work
	// being paid directly to the provider.
	Enabled bool
}

// ManagedSettlementHandoff is the Section 12 seam (HLG-026).
//
// The final downstream model is:
//
//	Hauling completion -> Fleet CompletedWork -> Fleet CompensationResolution
//	  -> ManagedWorkSettlementInstruction -> Wallet
//
// Hauling implements only the first arrow. This interface is where Section 12
// attaches the rest; nothing here short-circuits it by inventing
// Hauling-side compensation.
type ManagedSettlementHandoff interface {
	// HandleManagedCompletion runs once per authoritative managed completion,
	// AFTER the canonical completed-work record is durable.
	HandleManagedCompletion(ctx context.Context, bookingID string) error
}

// ManagedRouteReader answers whether a booking settles through Fleet.
//
// Read from the booking's OWN persisted settlement mode, fixed at execution
// start, rather than from a live Fleet lookup at completion time: that would
// let a Fleet outage silently route corporate work down the legacy
// direct-payment path, which is exactly the failure HLG-025 forbids.
type ManagedRouteReader interface {
	IsOrganizationManaged(ctx context.Context, bookingID string) (bool, error)
}

// settlementRouteFor decides how a completed booking settles.
//
// A read failure resolves to MANAGED, not INDEPENDENT. That is the
// conservative direction: withholding a payout is recoverable, paying the
// wrong party for corporate work is not.
func (s *BookingService) settlementRouteFor(ctx context.Context, bookingID string) SettlementRoute {
	reader, ok := s.bookings.(ManagedRouteReader)
	if !ok {
		return RouteIndependent
	}
	managed, err := reader.IsOrganizationManaged(ctx, bookingID)
	if err != nil {
		return RouteManaged
	}
	if managed {
		return RouteManaged
	}
	return RouteIndependent
}

// routeCompletedSettlement decides whether the legacy settlement path runs.
//
// It returns true when the caller should proceed with the existing
// independent settlement, and false when the booking is organization-managed
// and must be left for Fleet. With isolation off the answer is always true,
// which is the pre-Section-10 behaviour byte for byte.
func (s *BookingService) routeCompletedSettlement(ctx context.Context, bookingID string) bool {
	if !s.managedSettlement.Enabled {
		return true
	}
	if s.settlementRouteFor(ctx, bookingID) != RouteManaged {
		return true
	}
	if s.managedHandoff == nil {
		// Section 12 is not implemented. Do NOT fall through to the
		// independent path: that would pay the provider directly for work the
		// organization is contracted to settle, with no way to undo it. The
		// canonical completed-work record is already durable, so Fleet has
		// everything it needs to settle this correctly later.
		return false
	}
	if err := s.managedHandoff.HandleManagedCompletion(ctx, bookingID); err != nil {
		// The handoff only ever reports that Wallet could not be ASKED. That
		// is ambiguity, not failure, and it is deliberately visible: a
		// completion whose settlement state is unknown is the one case an
		// operator needs to look at, and silently discarding it here is how
		// such a job goes unnoticed until the provider asks where their money
		// is. It changes nothing about the routing -- corporate work is still
		// never paid directly to the provider.
		logging.Notice("hauling-settlement",
			"managed booking=%s settlement state unresolved: %v", bookingID, err)
	}
	return false
}

// completedWorkEnqueuer is satisfied by the booking repository once it can
// write the canonical Fleet completed-work row in the completion transaction.
type completedWorkEnqueuer interface {
	EnqueueCompletedWork(ctx context.Context, bookingID string) error
}

// recordCompletedWork persists the canonical Fleet completed-work delivery
// for an authoritative managed completion (HLG-017, HLG-018).
//
// It is deliberately called ONLY from the completion transition. An accepted,
// loaded, in-transit or cancelled booking produces nothing, because none of
// those is an authoritative completion.
func (s *BookingService) recordCompletedWork(ctx context.Context, bookingID string) {
	if !s.completedWorkEnabled {
		return
	}
	enqueuer, ok := s.bookings.(completedWorkEnqueuer)
	if !ok {
		return
	}
	// A failure here must not undo the job's own completion, which is already
	// true for the customer and the driver. The row is recoverable through
	// the reconciliation surface rather than by rolling back a finished job.
	_ = enqueuer.EnqueueCompletedWork(ctx, bookingID)
}

var _ = completedwork.Target
