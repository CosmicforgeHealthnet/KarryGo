package recovery

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
)

// ErrReplacementIneligible means the named replacement does not CURRENTLY fit
// this booking's cargo under the same fit engine the original match used.
var ErrReplacementIneligible = errors.New("recovery: named replacement is not currently eligible")

// ErrUnavailable means recovery is not enabled for this deployment.
var ErrUnavailable = errors.New("recovery: not enabled")

// RequirementFor exposes the matcher-equivalent cargo requirement so other
// owner workflows re-validate fit against CURRENT booking inputs rather than a
// historical snapshot.
func RequirementFor(booking BookingSnapshot) cargofit.Requirement { return requirementFor(booking) }

// LoadBookingTx reads the booking under recovery inside a caller transaction.
func (r *Repository) LoadBookingTx(ctx context.Context, tx pgx.Tx, bookingID string) (BookingSnapshot, error) {
	return r.WithTx(tx).LoadBooking(ctx, bookingID)
}

// CaseByIDTx reads a case by id inside a caller transaction, locking it.
func (r *Repository) CaseByIDTx(ctx context.Context, tx pgx.Tx, caseID string) (Case, error) {
	found, err := scanCase(tx.QueryRow(ctx, `SELECT `+caseColumns+` FROM hauling_recovery_cases WHERE id::text=$1 FOR UPDATE`, caseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Case{}, ErrNotFound
	}
	return found, err
}

// SelectNamedReplacementTx binds an operator-named replacement to an open case
// inside the caller's transaction. It is the same decision SelectReplacement
// makes, with the candidate fixed instead of searched: the named truck must be
// an ACTIVE truck of the named provider and must pass cargofit.Evaluate against
// the booking's CURRENT requirement. A case that already has a replacement, a
// terminal case or a booking that moved on is refused with ErrConflict. Events
// are written through the transactional sink; notifications and the payment
// rebind are left to the settlement-time guard (which re-checks the binding by
// recovery case before releasing anything).
func (s *Service) SelectNamedReplacementTx(ctx context.Context, tx pgx.Tx, caseID, providerID, truckID, actor string) (Case, error) {
	if !s.Enabled() {
		return Case{}, ErrUnavailable
	}
	repo := s.repo.WithTx(tx)
	current, err := s.repo.CaseByIDTx(ctx, tx, caseID)
	if err != nil {
		return Case{}, err
	}
	if current.State.Terminal() || current.ReplacementProviderID != "" {
		return Case{}, ErrConflict
	}
	booking, err := repo.LoadBooking(ctx, current.BookingID)
	if err != nil {
		return Case{}, err
	}
	if providerID == booking.ProviderID || providerID == current.OriginalProviderID {
		return Case{}, ErrReplacementIneligible
	}
	requirement := requirementFor(booking)
	requirement.ProductCode, requirement.CertificationRequired = s.capability.ProductForRequirement(requirement)
	fits, err := s.capability.LoadProviderTruckFits(ctx, []string{providerID}, requirement)
	if err != nil {
		return Case{}, err
	}
	var chosen *cargofit.TruckCapability
	for i := range fits[providerID] {
		if fits[providerID][i].TruckID == truckID {
			chosen = &fits[providerID][i]
		}
	}
	if chosen == nil || !cargofit.Evaluate(cargofit.ResolutionFound, *chosen, requirement).Eligible {
		return Case{}, ErrReplacementIneligible
	}
	now := s.now()
	if current.State != StateReplacementSearch {
		if !current.State.CanAdvanceTo(StateReplacementSearch) {
			return Case{}, ErrConflict
		}
		if current, err = repo.Advance(ctx, AdvanceInput{CaseID: current.ID, State: StateReplacementSearch, Actor: actor, ExpectedVersion: current.Version}, now); err != nil {
			return Case{}, err
		}
	}
	next := StateInProgress
	if current.CustodyTransferRequired {
		next = StateTransferRequired
	} else if err := repo.RebindProviderTx(ctx, tx, booking.ID, current.OriginalProviderID, providerID, truckID); err != nil {
		return Case{}, err
	}
	advanced, err := repo.Advance(ctx, AdvanceInput{CaseID: current.ID, State: next, ReplacementProviderID: providerID, ReplacementTruckID: truckID, ExpectedVersion: current.Version, Actor: actor}, now)
	if err != nil {
		return Case{}, err
	}
	s.publish(ctx, tx, EventRecoveryProviderReplaced, advanced, map[string]any{"custody_transfer_required": advanced.CustodyTransferRequired, "operator_directed": true})
	return advanced, nil
}
