package bookingusecases

import (
	"context"
	"strings"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/services/taxi-service/internal/outbox"
	"cosmicforge/logistics/shared/go/logging"
)

// SettlementRoute is the decision every money-moving completion path makes
// exactly once (Section 8 TAX-020).
type SettlementRoute string

const (
	// RouteIndependent is the legacy-compatible direct-to-provider wallet
	// settlement. Unchanged behaviour for every independent driver.
	RouteIndependent SettlementRoute = "INDEPENDENT"
	// RouteManaged is the Fleet managed-settlement path. Taxi's own
	// responsibility ends at reporting completed work; Fleet resolves
	// compensation and Section 12 instructs Wallet.
	RouteManaged SettlementRoute = "ORGANIZATION_MANAGED"
)

// ManagedSettlementConfig gates the managed path (Section 8 TAX-020A).
//
// Until Section 12 exists the managed path deliberately does NOT post to
// Wallet. It does not fake success, does not double-pay, and does not compute
// any organization/provider allocation inside Taxi. What it does is refuse to
// let corporate work fall through to the independent direct-payment path.
type ManagedSettlementConfig struct {
	// Enabled turns managed settlement isolation on. When false, corporate
	// work still reports completed work to Fleet but settles through the
	// legacy path, which is the pre-Section-8 behaviour and the safe default
	// during rollout.
	Enabled bool
}

// ManagedSettlementHandoff is the Section 12 seam.
//
// Section 12 implements this against Wallet, driven by Fleet's
// CompensationResolution and ManagedWorkSettlementInstruction. Taxi calls it
// and nothing more: no allocation is computed here, and no amount here is
// authoritative for anyone's payout.
type ManagedSettlementHandoff interface {
	// HandleManagedCompletion is invoked once per authoritative managed
	// completion, AFTER the completed-work outbox row is durable.
	HandleManagedCompletion(ctx context.Context, booking bookingmodels.Booking) error
}

// settlementRouteFor decides how a completed booking settles.
//
// The decision is read from the booking's own persisted settlement mode,
// which was fixed at execution start. It is deliberately NOT re-derived from
// a live Fleet lookup at completion time: doing that would mean a Fleet
// outage silently routes corporate work down the independent direct-payment
// path, which is exactly the failure Section 8 TAX-020 forbids.
func (s *BookingService) settlementRouteFor(booking bookingmodels.Booking) SettlementRoute {
	if booking.IsManagedSettlement() {
		return RouteManaged
	}
	return RouteIndependent
}

// settleCompletedBooking routes settlement for a completed booking.
//
// Independent: the legacy CompleteJob path, byte-for-byte unchanged.
// Managed with isolation enabled: the legacy path is NOT taken. Instead the
// Section 12 handoff runs if one is wired, and otherwise the completion is
// simply left for Fleet to settle -- which is correct and honest, because
// Fleet's completed-work record is already durable by this point.
func (s *BookingService) settleCompletedBooking(ctx context.Context, booking bookingmodels.Booking) (SettlementRoute, error) {
	route := s.settlementRouteFor(booking)
	if route == RouteIndependent {
		return route, s.settlePayment(ctx, booking)
	}
	if !s.managedSettlement.Enabled {
		// Isolation not switched on yet: corporate work continues to settle
		// the legacy way, which is the pre-Section-8 behaviour. This branch
		// exists so enabling isolation is a deliberate, reversible act.
		return RouteIndependent, s.settlePayment(ctx, booking)
	}
	if s.managedHandoff == nil {
		// Section 12 is not implemented. Do NOT fall through to the
		// independent path: that would pay the provider directly for work the
		// organization is contracted to settle, and there is no way to undo it.
		logging.Notice("taxi-settlement",
			"managed booking=%s awaiting Fleet settlement; independent payout deliberately skipped", booking.ID)
		return route, nil
	}
	return route, s.managedHandoff.HandleManagedCompletion(ctx, booking)
}

// completedWorkPayloadFor builds the canonical Fleet completed-work snapshot
// for an authoritative taxi completion (Section 8 TAX-015, TAX-016A).
//
// eligibleBasisMinor carries the fare Taxi already settled -- the reconciled
// final fare when one exists, the original estimate otherwise. Nothing here
// calculates a price: both numbers are snapshots produced by internal/pricing
// long before this point.
//
// Returns ok=false for any booking that is not an authoritative managed
// completion, which is the TAX-015A guard: no completed work is reported for
// an accepted, en-route, arrived, started, cancelled or failed booking.
func completedWorkPayloadFor(booking bookingmodels.Booking) (outbox.CompletedWorkPayload, bool) {
	if booking.Status != bookingmodels.StatusCompleted {
		return outbox.CompletedWorkPayload{}, false
	}
	if !booking.IsCorporateManaged {
		return outbox.CompletedWorkPayload{}, false
	}
	organizationID, membershipID, assignmentID := booking.CorporateBinding()
	if organizationID == "" || membershipID == "" {
		return outbox.CompletedWorkPayload{}, false
	}
	if booking.ProviderID == nil || strings.TrimSpace(*booking.ProviderID) == "" {
		return outbox.CompletedWorkPayload{}, false
	}
	if booking.CompletedAt == nil {
		return outbox.CompletedWorkPayload{}, false
	}
	correlationID := ""
	if booking.FleetCorrelationID != nil {
		correlationID = *booking.FleetCorrelationID
	}
	return outbox.CompletedWorkPayload{
		SourceService:      "TAXI",
		SourceJobID:        booking.ID,
		SourceAssignmentID: assignmentID,
		ProviderID:         *booking.ProviderID,
		MembershipID:       membershipID,
		OrganizationID:     organizationID,
		CompletedAt:        booking.CompletedAt.UTC(),
		Currency:           "NGN",
		EligibleBasisMinor: fareKobo(booking),
		SourceVersion:      booking.SourceVersion,
		IdempotencyKey:     outbox.CompletedWorkIdempotencyKey(booking.ID, booking.SourceVersion),
		CorrelationID:      correlationID,
	}, true
}
