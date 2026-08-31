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

func TestAdminCancelBooking_CancelsFromInProgressAndReleasesProvider(t *testing.T) {
	db := lifecycleTestDB(t)
	bookings := bookingrepositories.NewPostgresBookingRepository(db)
	opState := availabilityrepositories.NewPostgresOperationalStateRepository(db)
	audit := &fakeAdminAudit{}
	svc := NewBookingService(Options{Bookings: bookings, OperationalState: opState, AdminAudit: audit})
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusInProgress, availabilitymodels.StateOnTrip)

	cancelled, err := svc.AdminCancelBooking(context.Background(), "operator-1", bookingID, "safety incident reported")
	if err != nil {
		t.Fatalf("AdminCancelBooking: %v", err)
	}
	if cancelled.Status != bookingmodels.StatusCancelled || cancelled.CancelledBy == nil || *cancelled.CancelledBy != "admin" {
		t.Errorf("cancelled = %+v, want status=cancelled cancelled_by=admin", cancelled)
	}

	state, err := opState.Get(context.Background(), providerID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if state.Current != availabilitymodels.StateAvailable {
		t.Errorf("provider state = %q, want AVAILABLE after admin cancel released it", state.Current)
	}

	if len(audit.recorded) != 1 || audit.recorded[0] != "cancel:booking:"+bookingID {
		t.Errorf("audit.recorded = %v, want [cancel:booking:%s]", audit.recorded, bookingID)
	}

	events, err := svc.bookings.ListEvents(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	found := false
	for _, e := range events {
		if e.EventType == "taxi.booking.admin_cancelled" && e.ActorType == "admin" && e.ActorID == "operator-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("events = %+v, want a taxi.booking.admin_cancelled admin event", events)
	}
}

func TestAdminCancelBooking_RefusesAnAlreadyTerminalBooking(t *testing.T) {
	db := lifecycleTestDB(t)
	bookings := bookingrepositories.NewPostgresBookingRepository(db)
	opState := availabilityrepositories.NewPostgresOperationalStateRepository(db)
	svc := NewBookingService(Options{Bookings: bookings, OperationalState: opState, AdminAudit: &fakeAdminAudit{}})
	bookingID, _ := seedLifecycleFixture(t, db, bookingmodels.StatusCompleted, "")

	if _, err := svc.AdminCancelBooking(context.Background(), "operator-1", bookingID, "reason"); err == nil {
		t.Fatal("expected a conflict cancelling an already-completed booking")
	}
}

func TestListActive_ScopedByMarketExcludesTerminal(t *testing.T) {
	db := lifecycleTestDB(t)
	repo := bookingrepositories.NewPostgresBookingRepository(db)
	activeID, _ := seedLifecycleFixture(t, db, bookingmodels.StatusInProgress, availabilitymodels.StateOnTrip)
	_, _ = seedLifecycleFixture(t, db, bookingmodels.StatusCompleted, "")

	var marketID string
	if err := db.QueryRow(context.Background(), `SELECT market_id::text FROM taxi_bookings WHERE id = $1`, activeID).Scan(&marketID); err != nil {
		t.Fatalf("read market_id: %v", err)
	}

	active, err := repo.ListActive(context.Background(), marketID)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 1 || active[0].ID != activeID {
		t.Errorf("active = %+v, want exactly [%s]", active, activeID)
	}
}

func TestListUnmatched_FiltersByAge(t *testing.T) {
	db := lifecycleTestDB(t)
	repo := bookingrepositories.NewPostgresBookingRepository(db)
	oldID, _ := seedLifecycleFixture(t, db, bookingmodels.StatusUnmatched, "")
	recentID, _ := seedLifecycleFixture(t, db, bookingmodels.StatusUnmatched, "")
	mustExec(t, db, `UPDATE taxi_bookings SET created_at = $1 WHERE id = $2`, time.Now().Add(-2*time.Hour), oldID)
	mustExec(t, db, `UPDATE taxi_bookings SET created_at = $1 WHERE id = $2`, time.Now(), recentID)

	out, err := repo.ListUnmatched(context.Background(), "", "", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("ListUnmatched: %v", err)
	}
	foundOld, foundRecent := false, false
	for _, b := range out {
		if b.ID == oldID {
			foundOld = true
		}
		if b.ID == recentID {
			foundRecent = true
		}
	}
	if !foundOld || foundRecent {
		t.Errorf("foundOld=%v foundRecent=%v, want old included and recent excluded", foundOld, foundRecent)
	}
}

func TestCountByStatus_ReflectsRealRows(t *testing.T) {
	db := lifecycleTestDB(t)
	repo := bookingrepositories.NewPostgresBookingRepository(db)
	_, _ = seedLifecycleFixture(t, db, bookingmodels.StatusCompleted, "")
	_, _ = seedLifecycleFixture(t, db, bookingmodels.StatusCompleted, "")

	counts, err := repo.CountByStatus(context.Background())
	if err != nil {
		t.Fatalf("CountByStatus: %v", err)
	}
	if counts["completed"] < 2 {
		t.Errorf("counts[completed] = %d, want at least 2", counts["completed"])
	}
}
