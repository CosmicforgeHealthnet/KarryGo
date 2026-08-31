package bookingusecases

import (
	"context"
	"testing"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
)

func TestListDueScheduled_FindsOnlyPastDispatchStart(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	dueID, dueProvider := seedLifecycleFixture(t, db, bookingmodels.StatusScheduled, "")
	notDueID, notDueProvider := seedLifecycleFixture(t, db, bookingmodels.StatusScheduled, "")
	_ = dueProvider
	_ = notDueProvider

	now := time.Now()
	mustExec(t, db, `UPDATE taxi_bookings SET dispatch_start_at = $1 WHERE id = $2`, now.Add(-time.Minute), dueID)
	mustExec(t, db, `UPDATE taxi_bookings SET dispatch_start_at = $1 WHERE id = $2`, now.Add(time.Hour), notDueID)

	promoted, err := svc.PromoteDueScheduledBookings(context.Background(), now)
	if err != nil {
		t.Fatalf("PromoteDueScheduledBookings: %v", err)
	}
	if promoted != 1 {
		t.Fatalf("promoted = %d, want 1", promoted)
	}

	due, err := svc.bookings.GetByID(context.Background(), dueID)
	if err != nil {
		t.Fatalf("GetByID(due): %v", err)
	}
	if due.Status != bookingmodels.StatusPendingMatch {
		t.Errorf("due booking status = %q, want pending_match", due.Status)
	}
	if due.MatchingStartedAt == nil {
		t.Error("due booking MatchingStartedAt was not persisted")
	}

	notDue, err := svc.bookings.GetByID(context.Background(), notDueID)
	if err != nil {
		t.Fatalf("GetByID(notDue): %v", err)
	}
	if notDue.Status != bookingmodels.StatusScheduled {
		t.Errorf("not-due booking status = %q, want unchanged scheduled", notDue.Status)
	}
}

// TestPromoteScheduledToPendingMatch_SecondAttemptOnAlreadyPromotedRowIsRefused
// proves the guarded CAS for real: exactly one promotion succeeds per
// booking, the second is a real Postgres zero-row RETURNING, not a silent
// double-promotion -- the property that makes running this sweep from every
// service instance safe without a leader election.
func TestPromoteScheduledToPendingMatch_SecondAttemptOnAlreadyPromotedRowIsRefused(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	bookingID, _ := seedLifecycleFixture(t, db, bookingmodels.StatusScheduled, "")
	now := time.Now()

	if _, err := svc.bookings.PromoteScheduledToPendingMatch(context.Background(), bookingID, now); err != nil {
		t.Fatalf("first PromoteScheduledToPendingMatch: %v", err)
	}

	_, err := svc.bookings.PromoteScheduledToPendingMatch(context.Background(), bookingID, now)
	if err == nil {
		t.Fatal("expected the second promotion attempt on an already-promoted booking to be refused")
	}
}

func TestPromoteScheduledToPendingMatch_MatchingStartedAtIsPromotionTimeNotCreatedAt(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newLifecycleService(db)
	bookingID, _ := seedLifecycleFixture(t, db, bookingmodels.StatusScheduled, "")

	// Simulate a booking created a long time ago (as a real scheduled
	// booking's row would be by the time its dispatch window opens).
	longAgo := time.Now().Add(-72 * time.Hour)
	mustExec(t, db, `UPDATE taxi_bookings SET created_at = $1 WHERE id = $2`, longAgo, bookingID)

	promotionTime := time.Now()
	promoted, err := svc.bookings.PromoteScheduledToPendingMatch(context.Background(), bookingID, promotionTime)
	if err != nil {
		t.Fatalf("PromoteScheduledToPendingMatch: %v", err)
	}
	if promoted.MatchingStartedAt == nil {
		t.Fatal("MatchingStartedAt is nil after promotion")
	}
	if promoted.MatchingStartedAt.Sub(promotionTime).Abs() > time.Second {
		t.Errorf("MatchingStartedAt = %v, want ~%v (the PROMOTION time)", promoted.MatchingStartedAt, promotionTime)
	}
	// The exact B9 bug this whole feature exists to avoid: a search deadline
	// computed from this booking's real, 3-day-old CreatedAt would already
	// be expired.
	deadline := searchDeadlineFor(promoted, 60*time.Second)
	if deadline.Before(time.Now()) {
		t.Error("searchDeadlineFor(promoted, ...) is already in the past -- MatchingStartedAt was not correctly anchored to promotion time")
	}
}
