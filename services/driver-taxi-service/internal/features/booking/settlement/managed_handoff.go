// Package settlement implements Taxi's side of Section 12 managed settlement.
//
// Taxi does not settle corporate work and cannot: it does not know the
// provider's compensation plan, and the amounts belong to Fleet. What it does
// own is the question "did this completed job's money actually move?", because
// only Taxi knows when a completion ended ambiguously and needs asking again.
package settlement

import (
	"context"
	"fmt"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/managedsettlement"
)

// SettlementReader is the money authority, as Taxi needs it.
//
// Narrow on purpose: an execution service that could do more than read would
// eventually be asked to.
type SettlementReader interface {
	Lookup(ctx context.Context, identity managedsettlement.Identity) (managedsettlement.Outcome, error)
}

// ManagedHandoff confirms, for one completed managed booking, what Wallet did
// with the money (WAL-011).
//
// It never instructs a settlement and never pays anyone. Fleet already holds
// the durable completed-work record by the time this runs, so the settlement
// will happen with or without this call; what this call adds is a definite
// answer for Taxi's own reconciliation, so an ambiguous completion is retried
// rather than assumed.
type ManagedHandoff struct {
	Wallet SettlementReader
}

// New builds the handoff over the shared Wallet read client.
func New(client managedsettlement.Client) *ManagedHandoff {
	return &ManagedHandoff{Wallet: client}
}

// Identity is the stable financial identity of a booking's settlement
// (WAL-011A).
//
// Derived from the booking id alone, deliberately: the completion path and any
// later reconciliation both have it, and neither can drift from the other by
// picking up a version, a timestamp or an attempt counter that the other does
// not have.
func Identity(booking bookingmodels.Booking) managedsettlement.Identity {
	return managedsettlement.IdentityFor(managedsettlement.SourceTaxi, booking.ID)
}

// HandleManagedCompletion runs once per authoritative managed completion.
//
// The three outcomes are kept apart because collapsing any two of them pays
// somebody twice or not at all:
//
//   - settled: the money moved. Nothing left to do.
//   - resolved, not settled: Fleet has not instructed yet. Normal moments
//     after completion; the completed-work record is durable, so Fleet will.
//   - unknown: Wallet could not be asked. This returns an error so the caller
//     retries. It is NOT reported as unsettled, and under no circumstance does
//     it fall through to paying the driver directly (WAL-011B).
func (h *ManagedHandoff) HandleManagedCompletion(ctx context.Context, booking bookingmodels.Booking) error {
	if h == nil || h.Wallet == nil {
		return fmt.Errorf("taxi managed settlement: no wallet reader configured")
	}
	identity := Identity(booking)
	if !identity.Valid() {
		return fmt.Errorf("taxi managed settlement: booking has no usable settlement identity")
	}

	outcome, err := h.Wallet.Lookup(ctx, identity)
	if err != nil {
		// Ambiguous, not failed. Surfacing the error is what gets this
		// completion looked at again; swallowing it here would leave a job
		// that may or may not have been paid with nobody watching it.
		return fmt.Errorf("taxi managed settlement: settlement state unknown for %s: %w", identity.Key(), err)
	}
	if outcome.Settled {
		return nil
	}
	logging.Notice("taxi-settlement",
		"managed booking=%s not yet settled by Fleet (pending=%t); no independent payout", booking.ID, outcome.Pending)
	return nil
}
