package bookingusecases

import (
	"context"
	"testing"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingrepositories "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
)

func TestListStaleMatching_FindsOnlyOldPendingAndAwaiting(t *testing.T) {
	db := lifecycleTestDB(t)
	repo := bookingrepositories.NewPostgresBookingRepository(db)
	staleID, _ := seedLifecycleFixture(t, db, bookingmodels.StatusPendingMatch, "")
	freshID, _ := seedLifecycleFixture(t, db, bookingmodels.StatusAwaitingAccept, "")
	terminalID, _ := seedLifecycleFixture(t, db, bookingmodels.StatusCompleted, "")
	old := time.Now().Add(-time.Hour)
	mustExec(t, db, `UPDATE taxi_bookings SET updated_at = $1 WHERE id = $2`, old, staleID)
	mustExec(t, db, `UPDATE taxi_bookings SET updated_at = $1 WHERE id = $2`, old, terminalID) // old but terminal -- must be excluded regardless of age

	stale, err := repo.ListStaleMatching(context.Background(), time.Now().Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("ListStaleMatching: %v", err)
	}
	foundStale, foundFresh, foundTerminal := false, false, false
	for _, b := range stale {
		switch b.ID {
		case staleID:
			foundStale = true
		case freshID:
			foundFresh = true
		case terminalID:
			foundTerminal = true
		}
	}
	if !foundStale || foundFresh || foundTerminal {
		t.Errorf("foundStale=%v foundFresh=%v foundTerminal=%v, want only the stale pending_match booking", foundStale, foundFresh, foundTerminal)
	}
}

func TestListStaleInProgress_RequiresStartedAt(t *testing.T) {
	db := lifecycleTestDB(t)
	repo := bookingrepositories.NewPostgresBookingRepository(db)
	bookingID, _ := seedLifecycleFixture(t, db, bookingmodels.StatusInProgress, "")
	// started_at is NULL by default in the fixture -- must never match, a
	// nil comparison against a real timestamp would otherwise be undefined.
	stale, err := repo.ListStaleInProgress(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("ListStaleInProgress: %v", err)
	}
	for _, b := range stale {
		if b.ID == bookingID {
			t.Fatal("a booking with no started_at must never be treated as stale in-progress")
		}
	}
}

func TestRecoverStuckInProgressBookings_CompletesEachStuckBooking_Integration(t *testing.T) {
	db := lifecycleTestDB(t)
	bookings := bookingrepositories.NewPostgresBookingRepository(db)
	opState := availabilityrepositories.NewPostgresOperationalStateRepository(db)
	svc := NewBookingService(Options{Bookings: bookings, Markets: newFakeMarkets(), Pricing: newFakePricing(), OperationalState: opState})
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusInProgress, availabilitymodels.StateOnTrip)

	longAgo := time.Now().Add(-time.Hour)
	mustExec(t, db, `UPDATE taxi_bookings SET started_at = $1, distance_km = 5.2 WHERE id = $2`, longAgo, bookingID)

	count, err := svc.RecoverStuckInProgressBookings(context.Background(), 30*time.Minute)
	if err != nil {
		t.Fatalf("RecoverStuckInProgressBookings: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	completed, err := bookings.GetByID(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if completed.Status != bookingmodels.StatusCompleted {
		t.Errorf("status = %q, want completed", completed.Status)
	}

	state, err := opState.Get(context.Background(), providerID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if state.Current != availabilitymodels.StateAvailable {
		t.Errorf("provider state = %q, want AVAILABLE after auto-completion released it", state.Current)
	}

	events, err := bookings.ListEvents(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	foundAutoComplete := false
	for _, e := range events {
		if e.EventType == "taxi.booking.auto_completed" && e.ActorType == "system" && e.ActorID == "recovery-worker" {
			foundAutoComplete = true
		}
	}
	if !foundAutoComplete {
		t.Errorf("events = %+v, want a taxi.booking.auto_completed system event", events)
	}
}

func TestRecoverStaleMatchingBookings_FindsRealStaleRowsInPostgres(t *testing.T) {
	db := lifecycleTestDB(t)
	bookings := bookingrepositories.NewPostgresBookingRepository(db)
	svc := NewBookingService(Options{Bookings: bookings}) // Lease left nil -> spawnMatcher no-ops safely
	staleID, _ := seedLifecycleFixture(t, db, bookingmodels.StatusPendingMatch, "")
	mustExec(t, db, `UPDATE taxi_bookings SET updated_at = $1 WHERE id = $2`, time.Now().Add(-time.Hour), staleID)

	count, err := svc.RecoverStaleMatchingBookings(context.Background(), 30*time.Minute)
	if err != nil {
		t.Fatalf("RecoverStaleMatchingBookings: %v", err)
	}
	if count < 1 {
		t.Errorf("count = %d, want at least 1 (the seeded stale booking)", count)
	}
}
