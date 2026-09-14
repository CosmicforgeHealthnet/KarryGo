// Package tripsettlement implements Dispatch's side of Section 12 managed
// settlement.
//
// Dispatch does not settle corporate work and cannot: the rider's compensation
// plan belongs to Fleet and the money belongs to Wallet. What Dispatch owns is
// the question "did this completed mission's money actually move?", because
// only Dispatch knows which completions ended ambiguously.
package tripsettlement

import (
	"context"
	"fmt"
	"log"
	"time"

	"cosmicforge/logistics/shared/go/managedsettlement"
)

// SettlementReader is the money authority, as Dispatch needs it: read-only.
type SettlementReader interface {
	Lookup(ctx context.Context, identity managedsettlement.Identity) (managedsettlement.Outcome, error)
}

// ManagedHandoff confirms what Wallet did with one completed mission's money
// (WAL-012).
type ManagedHandoff struct {
	Wallet SettlementReader
}

// New builds the handoff over the shared Wallet read client.
func New(client managedsettlement.Client) *ManagedHandoff {
	return &ManagedHandoff{Wallet: client}
}

// Identity is the stable financial identity of a mission's settlement
// (WAL-012).
//
// The TRIP id, not the booking id. Two reasons, and getting this wrong is not
// recoverable by retrying: the trip is what Dispatch reports to Fleet as
// SourceJobID, so it is the only name Wallet will ever hold this settlement
// under; and one booking can produce more than one trip, so a booking-keyed
// identity would let a second mission's settlement answer for the first.
//
// Both of Dispatch's settlement paths -- the synchronous completion and the
// reconciliation worker -- derive the identity through this one function.
// That is what makes them agree: if each built its own, the worker would ask
// about a job under a name Wallet has never seen, conclude nothing was paid,
// and pay the rider a second time.
func Identity(tripID string) managedsettlement.Identity {
	return managedsettlement.IdentityFor(managedsettlement.SourceDispatch, tripID)
}

// HandleManagedCompletion runs once per authoritative managed completion, from
// either settlement path.
//
// bookingID and completedAt are part of the interface Dispatch's completion
// paths already speak; they are carried for logging and are deliberately NOT
// part of the financial identity, so a reconciliation that has one of them
// slightly different from the completion path cannot address a different job.
func (h *ManagedHandoff) HandleManagedCompletion(
	ctx context.Context, tripID, bookingID string, completedAt time.Time,
) error {
	if h == nil || h.Wallet == nil {
		return fmt.Errorf("dispatch managed settlement: no wallet reader configured")
	}
	identity := Identity(tripID)
	if !identity.Valid() {
		return fmt.Errorf("dispatch managed settlement: trip has no usable settlement identity")
	}

	outcome, err := h.Wallet.Lookup(ctx, identity)
	if err != nil {
		// Ambiguous, not failed. Returning the error is what puts this mission
		// back in front of the reconciliation worker; swallowing it would
		// leave money that may or may not have moved with nobody watching.
		return fmt.Errorf("dispatch managed settlement: settlement state unknown for %s: %w", identity.Key(), err)
	}
	if outcome.Settled {
		return nil
	}
	log.Printf("managed trip not yet settled by Fleet trip_id=%s booking_id=%s pending=%t completed_at=%s; no independent payout",
		tripID, bookingID, outcome.Pending, completedAt.UTC().Format(time.RFC3339))
	return nil
}
