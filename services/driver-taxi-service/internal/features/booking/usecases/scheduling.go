package bookingusecases

import (
	"context"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/logging"
)

// PromoteDueScheduledBookings finds every scheduled booking whose dispatch
// window has opened (DispatchStartAt <= now) and promotes each to
// pending_match via a guarded CAS, then spawns the normal matcher for it --
// from this point on a scheduled ride is indistinguishable from an
// immediate one (same matcher, same offer waves, same search window,
// anchored to the promotion instant via MatchingStartedAt, never
// CreatedAt -- see that field's own doc comment).
//
// A promotion refused by the guard (apperrors.CodeConflict: another sweep,
// in this instance or a concurrently-running one, already promoted it) is
// not an error -- exactly one promotion succeeds per booking regardless of
// how many sweeps observe it as due, which is what makes this worker safe
// to run from every service instance without a leader election.
//
// Returns the count actually promoted, for the caller's own logging.
func (s *BookingService) PromoteDueScheduledBookings(ctx context.Context, now time.Time) (int, error) {
	ids, err := s.bookings.ListDueScheduled(ctx, now)
	if err != nil {
		return 0, err
	}

	promoted := 0
	for _, id := range ids {
		if _, err := s.bookings.PromoteScheduledToPendingMatch(ctx, id, now); err != nil {
			if appErr := apperrors.From(err); appErr != nil && appErr.Code == apperrors.CodeConflict {
				continue
			}
			return promoted, err
		}
		s.spawnMatcher(id)
		promoted++
	}
	return promoted, nil
}

// RunScheduledDispatchWorker sweeps for due scheduled bookings on a fixed
// interval, plus once immediately on start -- the same "sweep-on-start,
// then tick" shape as every other recovery-style worker on this platform
// (see project memory's Go goroutine patterns note: no time.Sleep for the
// interval itself, a real ticker so the caller can shut it down via ctx).
// Runs until ctx is cancelled.
func (s *BookingService) RunScheduledDispatchWorker(ctx context.Context, interval time.Duration) {
	sweep := func() {
		promoted, err := s.PromoteDueScheduledBookings(ctx, time.Now())
		if err != nil {
			logging.Error("taxi-scheduler", "sweep: %v", err)
			return
		}
		if promoted > 0 {
			logging.Notice("taxi-scheduler", "promoted %d scheduled booking(s) to matching", promoted)
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
