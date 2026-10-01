package trip

import (
	"context"
	"log"
	"time"
)

// SettlementRoute is the decision every money-moving completion path makes
// exactly once (Section 9 DSP-014).
type SettlementRoute string

const (
	// RouteIndependent is the legacy-compatible direct-to-provider wallet
	// settlement. Unchanged behaviour for every independent rider.
	RouteIndependent SettlementRoute = "INDEPENDENT"
	// RouteManaged is the Fleet managed-settlement path. Dispatch's
	// responsibility ends at reporting completed work.
	RouteManaged SettlementRoute = "ORGANIZATION_MANAGED"
)

// ManagedSettlementConfig gates the managed path.
//
// Until Section 12 exists the managed path deliberately does NOT post to
// Wallet. It does not fake success, does not double-pay, and does not compute
// any corporate compensation inside Dispatch. What it does is refuse to let
// corporate work fall through to the independent direct-payment path.
type ManagedSettlementConfig struct {
	// Enabled isolates corporate settlement. Default false, which preserves
	// the pre-Section-9 behaviour exactly; turning it on stops corporate
	// missions being paid directly to the rider.
	Enabled bool
}

// ManagedSettlementHandoff is the Section 12 seam (DSP-014A).
//
// The final downstream model is:
//
//	Dispatch completion -> Fleet CompletedWork -> Fleet CompensationResolution
//	  -> ManagedWorkSettlementInstruction -> Wallet
//
// Dispatch implements only the first arrow. This interface is where Section
// 12 attaches the rest; nothing here short-circuits it by inventing
// Dispatch-side compensation.
type ManagedSettlementHandoff interface {
	// HandleManagedCompletion runs once per authoritative managed completion,
	// AFTER the canonical completed-work delivery is durable.
	HandleManagedCompletion(ctx context.Context, tripID, bookingID string, completedAt time.Time) error
}

// WithManagedSettlement configures the corporate settlement boundary.
func (s *Service) WithManagedSettlement(config ManagedSettlementConfig, handoff ManagedSettlementHandoff) *Service {
	s.managedSettlement, s.managedHandoff = config, handoff
	return s
}

// ManagedRouteReader answers whether a trip settles through Fleet.
//
// Implemented by the trip repository against the trip's own persisted
// execution_mode, which was fixed at execution start. It is deliberately NOT
// a live Fleet lookup at completion time: that would let a Fleet outage
// silently route corporate work down the independent direct-payment path,
// which is exactly the failure Section 9 DSP-014 forbids.
type ManagedRouteReader interface {
	IsOrganizationManaged(ctx context.Context, tripID string) (bool, error)
}

// settlementRouteFor decides how a completed trip settles.
//
// A read failure resolves to MANAGED, not INDEPENDENT. That is the
// conservative direction: withholding a payout is recoverable, paying the
// wrong party for corporate work is not.
func (s *Service) settlementRouteFor(ctx context.Context, tripID string) SettlementRoute {
	reader, ok := s.repository.(ManagedRouteReader)
	if !ok {
		return RouteIndependent
	}
	managed, err := reader.IsOrganizationManaged(ctx, tripID)
	if err != nil {
		log.Printf("trip settlement route unresolved trip_id=%s; treating as managed", tripID)
		return RouteManaged
	}
	if managed {
		return RouteManaged
	}
	return RouteIndependent
}

// settleCompletedTrip routes settlement for a completed mission.
//
// Independent: the existing settlement reconciler / wallet CompleteJob path,
// byte-for-byte unchanged. Managed with isolation enabled: the legacy path is
// NOT taken; the Section 12 handoff runs if one is wired, and otherwise the
// completion is left for Fleet to settle -- which is correct and honest,
// because the canonical completed-work delivery is already durable.
func (s *Service) settleCompletedTrip(ctx context.Context, tripID, bookingID string, completedAt time.Time) {
	route := RouteIndependent
	if s.managedSettlement.Enabled {
		route = s.settlementRouteFor(ctx, tripID)
	}
	if route == RouteManaged {
		if s.managedHandoff == nil {
			// Section 12 is not implemented. Do NOT fall through to the
			// independent path: that would pay the rider directly for work
			// the organization is contracted to settle, with no way to undo it.
			log.Printf("managed trip awaiting Fleet settlement trip_id=%s; independent payout deliberately skipped", tripID)
			return
		}
		if err := s.managedHandoff.HandleManagedCompletion(ctx, tripID, bookingID, completedAt); err != nil {
			log.Printf("managed settlement handoff pending trip_id=%s", tripID)
		}
		return
	}
	if s.settlement != nil {
		if err := s.settlement.HandleTripCompleted(ctx, bookingID, completedAt); err != nil {
			log.Printf("trip settlement confirmation pending trip_id=%s", tripID)
		}
		return
	}
	if s.wallet != nil {
		// CompleteJobAt, not CompleteJob: the wallet records this completion time
		// on the settlement event, and the referral engine judges its 90-day
		// earning window from when the job finished rather than when the money
		// moved. It is also the only method on this service's wallet interface.
		if _, err := s.wallet.CompleteJobAt(ctx, "dispatch-delivery-service", bookingID, completedAt); err != nil {
			log.Printf("wallet settlement failed trip_id=%s booking_id=%s", tripID, bookingID)
			// Trip is already completed in the DB. Do not roll back.
		}
	}
}
