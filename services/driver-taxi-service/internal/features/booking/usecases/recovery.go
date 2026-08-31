package bookingusecases

import (
	"context"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/logging"
)

// RecoverStaleMatchingBookings re-spawns the matcher for every booking still
// pending_match/awaiting_acceptance whose row has not moved in staleAfter --
// the actual crash-recovery mechanism: a service restart loses every
// in-memory matchBooking goroutine, so a booking mid-search at the moment of
// a crash is otherwise stranded forever (its lease simply expires with
// nobody watching it).
//
// Safe to call on a booking whose matcher IS still alive: spawnMatcher's own
// lease acquisition (B2) makes a duplicate attempt a harmless no-op, so this
// worker never needs to distinguish "genuinely orphaned" from "just slow" --
// staleAfter only exists to avoid needless lease-acquisition churn on
// bookings that are still being actively, correctly searched.
func (s *BookingService) RecoverStaleMatchingBookings(ctx context.Context, staleAfter time.Duration) (int, error) {
	cutoff := time.Now().Add(-staleAfter)
	stale, err := s.bookings.ListStaleMatching(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	for _, b := range stale {
		s.spawnMatcher(b.ID)
	}
	return len(stale), nil
}

// RecoverStuckInProgressBookings auto-completes every in_progress booking
// whose trip started more than graceAfter ago and never got a driver-
// confirmed complete call (an app crash after drop-off, a lost connection).
// Reconciles through the SAME MarkCompleted path a real completion uses --
// including its own preference for the server-computed telemetry distance
// (phase B6) over whatever fallback figure is passed in here. The original
// trip estimate (DistanceKm at booking creation) is that fallback: not a
// real "actual" distance, but the best available number when no telemetry
// trail exists AND nobody ever reported one, disclosed via the "system"
// actor this records instead of "provider" on the booking_events row.
func (s *BookingService) RecoverStuckInProgressBookings(ctx context.Context, graceAfter time.Duration) (int, error) {
	cutoff := time.Now().Add(-graceAfter)
	stuck, err := s.bookings.ListStaleInProgress(ctx, cutoff)
	if err != nil {
		return 0, err
	}

	recovered := 0
	for _, b := range stuck {
		if b.ProviderID == nil {
			continue // cannot happen for a real in_progress booking, but never crash a sweep over one bad row
		}
		durationMin := 0
		if b.StartedAt != nil {
			durationMin = int(time.Since(*b.StartedAt).Minutes())
		}
		fallbackDistanceKm := 0.0
		if b.DistanceKm != nil {
			fallbackDistanceKm = *b.DistanceKm
		}

		completed, err := s.MarkCompleted(ctx, CompleteBookingInput{
			BookingID: b.ID, ProviderID: *b.ProviderID,
			ActualDistanceKm: fallbackDistanceKm, ActualDurationMin: durationMin,
		})
		if err != nil {
			logging.Error("taxi-recovery", "auto-complete booking=%s: %v", b.ID, err)
			continue
		}
		if err := s.bookings.RecordEvent(ctx, nil, bookingmodels.BookingEvent{
			BookingID: completed.ID, EventType: "taxi.booking.auto_completed", ActorType: "system", ActorID: "recovery-worker",
		}); err != nil {
			logging.Error("taxi-recovery", "record auto-complete audit booking=%s: %v", b.ID, err)
		}
		recovered++
	}
	return recovered, nil
}

// RunMatchRecoveryWorker sweeps on the given interval, plus once immediately
// on start. Runs until ctx is cancelled.
func (s *BookingService) RunMatchRecoveryWorker(ctx context.Context, interval, staleAfter time.Duration) {
	sweep := func() {
		recovered, err := s.RecoverStaleMatchingBookings(ctx, staleAfter)
		if err != nil {
			logging.Error("taxi-recovery", "match-recovery sweep: %v", err)
			return
		}
		if recovered > 0 {
			logging.Notice("taxi-recovery", "re-spawned matcher for %d stale booking(s)", recovered)
		}
	}
	sweep()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}

// RunAutoCompleteWorker sweeps on the given interval, plus once immediately
// on start. Runs until ctx is cancelled.
func (s *BookingService) RunAutoCompleteWorker(ctx context.Context, interval, graceAfter time.Duration) {
	sweep := func() {
		recovered, err := s.RecoverStuckInProgressBookings(ctx, graceAfter)
		if err != nil {
			logging.Error("taxi-recovery", "auto-complete sweep: %v", err)
			return
		}
		if recovered > 0 {
			logging.Notice("taxi-recovery", "auto-completed %d stuck in-progress booking(s)", recovered)
		}
	}
	sweep()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}
