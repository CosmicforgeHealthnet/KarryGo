// Package settlement implements Hauling's side of Section 12 managed
// settlement.
//
// Hauling does not settle corporate work and cannot: the provider's
// compensation plan belongs to Fleet and the money belongs to Wallet. What
// Hauling owns is the question "did this completed job's money actually
// move?", because only Hauling knows which completions ended ambiguously.
package settlement

import (
	"context"
	"fmt"

	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/managedsettlement"
)

// SettlementReader is the money authority, as Hauling needs it: read-only.
type SettlementReader interface {
	Lookup(ctx context.Context, identity managedsettlement.Identity) (managedsettlement.Outcome, error)
}

// ManagedHandoff confirms what Wallet did with one completed job's money
// (WAL-013).
type ManagedHandoff struct {
	Wallet SettlementReader
}

// New builds the handoff over the shared Wallet read client.
func New(client managedsettlement.Client) *ManagedHandoff {
	return &ManagedHandoff{Wallet: client}
}

// Identity is the stable financial identity of a booking's settlement.
//
// The booking id, which is exactly what Hauling reports to Fleet as
// SourceJobID -- so it is the only name Wallet will ever hold this settlement
// under. Deriving it here rather than at each call site is what keeps the
// completion path and any later retry addressing the same job.
func Identity(bookingID string) managedsettlement.Identity {
	return managedsettlement.IdentityFor(managedsettlement.SourceHauling, bookingID)
}

// HandleManagedCompletion runs once per authoritative managed completion.
//
// Settled means the money moved. Resolved-but-unsettled means Fleet has not
// instructed yet, which is normal moments after completion and needs nothing.
// An error means Wallet could not be asked -- which is not a failure, is never
// read as "nothing settled", and above all never causes an independent payout.
func (h *ManagedHandoff) HandleManagedCompletion(ctx context.Context, bookingID string) error {
	if h == nil || h.Wallet == nil {
		return fmt.Errorf("hauling managed settlement: no wallet reader configured")
	}
	identity := Identity(bookingID)
	if !identity.Valid() {
		return fmt.Errorf("hauling managed settlement: booking has no usable settlement identity")
	}

	outcome, err := h.Wallet.Lookup(ctx, identity)
	if err != nil {
		return fmt.Errorf("hauling managed settlement: settlement state unknown for %s: %w", identity.Key(), err)
	}
	if outcome.Settled {
		return nil
	}
	logging.Notice("hauling-settlement",
		"managed booking=%s not yet settled by Fleet (pending=%t); no independent payout", bookingID, outcome.Pending)
	return nil
}
