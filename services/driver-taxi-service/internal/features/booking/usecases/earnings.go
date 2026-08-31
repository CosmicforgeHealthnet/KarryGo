package bookingusecases

import (
	"context"
	"time"

	bookingearnings "cosmicforge/logistics/services/taxi-service/internal/features/booking/earnings"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

// OperationalHistoryLookup is the slice of provider_availability's history
// table the earnings-insights report needs to reconstruct an online/active-
// trip timeline for one provider over one window. Declared locally rather
// than depending on availabilityusecases, matching every other cross-
// package dependency in this service (see MarketLookup's own doc comment)
// -- and satisfied with no adapter by
// *availabilityrepositories.PostgresOperationalStateRepository, the same
// zero-adapter convention as AvailabilityCandidateStore/PositionUpdater.
type OperationalHistoryLookup interface {
	StateAsOf(ctx context.Context, providerID string, at time.Time) (string, error)
	ListHistory(ctx context.Context, providerID string, from, until time.Time) ([]availabilityrepositories.HistoryEntry, error)
	// ListHistoryForBooking backs phase B10 route replay: every operational-
	// state transition recorded against ONE booking, regardless of window.
	ListHistoryForBooking(ctx context.Context, bookingID string) ([]availabilityrepositories.HistoryEntry, error)
}

// GetProviderEarningsInsights computes the addendum §19 report for one
// provider over [from, until]. Returns apperrors.Unavailable if either
// dependency (bookings repository is always present; OperationalHistory is
// nil-able) is not configured, and apperrors.BadRequest for an inverted or
// zero-width window -- never a confusing empty report for bad input.
func (s *BookingService) GetProviderEarningsInsights(ctx context.Context, providerID string, from, until time.Time) (bookingearnings.Insights, error) {
	if s.operationalHistory == nil {
		return bookingearnings.Insights{}, apperrors.Unavailable("Earnings insights are not available right now.", nil)
	}
	if !until.After(from) {
		return bookingearnings.Insights{}, apperrors.BadRequest("The reporting window must have 'until' after 'from'.", nil)
	}

	bookings, err := s.bookings.ListTerminalByProviderInRange(ctx, providerID, from, until)
	if err != nil {
		return bookingearnings.Insights{}, err
	}
	facts := make([]bookingearnings.BookingFacts, len(bookings))
	for i, b := range bookings {
		facts[i] = bookingearnings.BookingFacts{
			Status: string(b.Status), FareFinalKobo: b.FareFinalKobo, CancelledBy: b.CancelledBy,
			MatchedAt: b.MatchedAt, ArrivedAt: b.ArrivedAt, StartedAt: b.StartedAt, CompletedAt: b.CompletedAt,
		}
	}

	asOfState, err := s.operationalHistory.StateAsOf(ctx, providerID, from)
	if err != nil {
		return bookingearnings.Insights{}, err
	}
	history, err := s.operationalHistory.ListHistory(ctx, providerID, from, until)
	if err != nil {
		return bookingearnings.Insights{}, err
	}
	changes := make([]bookingearnings.StateChange, len(history))
	for i, h := range history {
		changes[i] = bookingearnings.StateChange{ToState: h.ToState, ChangedAt: h.ChangedAt}
	}

	return bookingearnings.Compute(facts, asOfState, changes, from, until), nil
}
