package bookingusecases

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingrepositories "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
)

// newEarningsIntegrationService mirrors newLifecycleService plus the real
// OperationalStateRepository (which also implements OperationalHistoryLookup
// -- see earnings.go's own doc comment), proving the full repository ->
// usecase -> pure-aggregator chain against real Postgres, not fakes.
func newEarningsIntegrationService(db *pgxpool.Pool) *BookingService {
	bookings := bookingrepositories.NewPostgresBookingRepository(db)
	opState := availabilityrepositories.NewPostgresOperationalStateRepository(db)
	return NewBookingService(Options{
		Bookings: bookings, Markets: newFakeMarkets(), Pricing: newFakePricing(), OperationalState: opState,
		OperationalHistory: opState,
	})
}

func TestGetProviderEarningsInsights_FullChainAgainstRealPostgres(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newEarningsIntegrationService(db)

	// Two completed bookings (one cancelled) already assigned to this
	// provider, seeded directly since there is no B2 matcher in this test.
	completedID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusCompleted, "")
	fare := int64(150000)
	matchedAt := time.Now().Add(-40 * time.Minute)
	arrivedAt := matchedAt.Add(5 * time.Minute)
	startedAt := arrivedAt.Add(2 * time.Minute)
	completedAt := startedAt.Add(20 * time.Minute)
	mustExec(t, db, `
		UPDATE taxi_bookings
		   SET fare_final_kobo = $1, matched_at = $2, arrived_at = $3, started_at = $4, completed_at = $5
		 WHERE id = $6
	`, fare, matchedAt, arrivedAt, startedAt, completedAt, completedID)

	cancelledID, _ := seedLifecycleFixture(t, db, bookingmodels.StatusCancelled, "")
	mustExec(t, db, `UPDATE taxi_bookings SET provider_id = $1, cancelled_by = 'customer' WHERE id = $2`, providerID, cancelledID)

	// A real operational-state timeline for this provider: AVAILABLE at the
	// window start, ON_TRIP for the trip's duration, back to AVAILABLE. Both
	// transitions write changed_at via Postgres's own now(), which runs
	// AFTER any Go-side time.Now() captured before this point -- 'until'
	// must be captured AFTER the transitions commit, or ListHistory's
	// (from, until] filter excludes them entirely.
	from := matchedAt.Add(-10 * time.Minute)
	mustExec(t, db, `INSERT INTO provider_operational_state (provider_id, state, version) VALUES ($1, 'AVAILABLE', 1)`, providerID)
	opState := availabilityrepositories.NewPostgresOperationalStateRepository(db)
	s1, err := opState.Transition(context.Background(), nil, providerID, availabilitymodels.StateOnTrip, "matched", nil, 1)
	if err != nil {
		t.Fatalf("Transition to ON_TRIP: %v", err)
	}
	if _, err := opState.Transition(context.Background(), nil, providerID, availabilitymodels.StateAvailable, "completed", nil, s1.Version); err != nil {
		t.Fatalf("Transition back to AVAILABLE: %v", err)
	}
	until := time.Now()

	insights, err := svc.GetProviderEarningsInsights(context.Background(), providerID, from, until)
	if err != nil {
		t.Fatalf("GetProviderEarningsInsights: %v", err)
	}
	if insights.TripsCompleted != 1 {
		t.Errorf("TripsCompleted = %d, want 1", insights.TripsCompleted)
	}
	if insights.GrossEarningsKobo != fare {
		t.Errorf("GrossEarningsKobo = %d, want %d", insights.GrossEarningsKobo, fare)
	}
	if insights.CancelledByCustomerCount != 1 {
		t.Errorf("CancelledByCustomerCount = %d, want 1", insights.CancelledByCustomerCount)
	}
	if insights.AveragePickupEtaSeconds == nil || *insights.AveragePickupEtaSeconds != 300 {
		t.Errorf("AveragePickupEtaSeconds = %v, want 300 (5 minutes)", insights.AveragePickupEtaSeconds)
	}
	if insights.AverageTripDurationMinutes == nil || *insights.AverageTripDurationMinutes != 20 {
		t.Errorf("AverageTripDurationMinutes = %v, want 20", insights.AverageTripDurationMinutes)
	}
	if insights.OnlineTimeSeconds <= 0 {
		t.Errorf("OnlineTimeSeconds = %v, want > 0 -- the provider was AVAILABLE/ON_TRIP for real, seeded rows", insights.OnlineTimeSeconds)
	}
	if insights.ActiveTripTimeSeconds <= 0 {
		t.Errorf("ActiveTripTimeSeconds = %v, want > 0 -- a real ON_TRIP transition was seeded", insights.ActiveTripTimeSeconds)
	}
	if insights.IsLedger {
		t.Error("IsLedger = true, want unconditionally false")
	}
}

func TestGetProviderEarningsInsights_ExcludesBookingsOutsideTheWindow(t *testing.T) {
	db := lifecycleTestDB(t)
	svc := newEarningsIntegrationService(db)

	completedID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusCompleted, "")
	oldCreatedAt := time.Now().Add(-100 * 24 * time.Hour)
	mustExec(t, db, `UPDATE taxi_bookings SET created_at = $1, fare_final_kobo = 90000 WHERE id = $2`, oldCreatedAt, completedID)

	insights, err := svc.GetProviderEarningsInsights(context.Background(), providerID, time.Now().Add(-30*24*time.Hour), time.Now())
	if err != nil {
		t.Fatalf("GetProviderEarningsInsights: %v", err)
	}
	if insights.TripsCompleted != 0 || insights.GrossEarningsKobo != 0 {
		t.Errorf("insights = %+v, want zero -- the booking is 100 days old, outside the 30-day window", insights)
	}
}
