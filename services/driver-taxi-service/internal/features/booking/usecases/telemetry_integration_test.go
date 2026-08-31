package bookingusecases

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingrepositories "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	bookingtelemetry "cosmicforge/logistics/services/taxi-service/internal/features/booking/telemetry"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
)

// newTelemetryIntegrationService mirrors newLifecycleService plus a real
// bookingtelemetry.PostgresRepository, so this file proves the whole
// batch->classify->store->(optionally)MarkCompleted chain against real
// Postgres, not fakes.
func newTelemetryIntegrationService(db *pgxpool.Pool) (*BookingService, *bookingtelemetry.PostgresRepository) {
	bookings := bookingrepositories.NewPostgresBookingRepository(db)
	opState := availabilityrepositories.NewPostgresOperationalStateRepository(db)
	telemetry := bookingtelemetry.NewPostgresRepository(db)
	svc := NewBookingService(Options{
		Bookings: bookings, Markets: newFakeMarkets(), Pricing: newFakePricing(), OperationalState: opState,
		Telemetry:       telemetry,
		TelemetryConfig: bookingtelemetry.Config{MaxSpeedKph: 180, StaleThreshold: 120 * time.Second},
	})
	return svc, telemetry
}

func TestIngestLocationPoints_DurablyStoresAgainstRealPostgres(t *testing.T) {
	db := lifecycleTestDB(t)
	svc, telemetry := newTelemetryIntegrationService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusInProgress, availabilitymodels.StateOnTrip)

	results, err := svc.IngestLocationPoints(context.Background(), providerID, []LocationPointInput{
		{BookingID: bookingID, Sequence: 1, Lat: 6.5244, Lng: 3.3792, RecordedAt: time.Now()},
		// A deliberate impossible jump -- must still be durably stored, never dropped.
		{BookingID: bookingID, Sequence: 2, Lat: 51.5072, Lng: -0.1276, RecordedAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("IngestLocationPoints: %v", err)
	}
	if len(results) != 2 || !results[0].Stored || !results[1].Stored {
		t.Fatalf("results = %+v, want both points stored", results)
	}
	if len(results[1].IntegrityFlags) == 0 {
		t.Error("want the second point flagged as an impossible jump")
	}

	latest, ok, err := telemetry.Latest(context.Background(), bookingID, providerID)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if !ok || latest.Sequence != 2 {
		t.Errorf("Latest sequence = %d (ok=%v), want 2 -- the flagged point IS the durable trail's latest entry", latest.Sequence, ok)
	}
}

func TestMarkCompleted_UsesServerComputedDistanceWhenTelemetryExists(t *testing.T) {
	db := lifecycleTestDB(t)
	svc, _ := newTelemetryIntegrationService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusInProgress, availabilitymodels.StateOnTrip)

	// Two closely-spaced fixes (~50m, 3 seconds apart -- a realistic GPS
	// cadence, implying ~60kph, comfortably under the 180kph threshold).
	// Both RecordedAt values also stay within the 120s stale threshold of
	// this call's own ingest time.
	_, err := svc.IngestLocationPoints(context.Background(), providerID, []LocationPointInput{
		{BookingID: bookingID, Sequence: 1, Lat: 6.5244, Lng: 3.3792, RecordedAt: time.Now().Add(-3 * time.Second)},
		{BookingID: bookingID, Sequence: 2, Lat: 6.5249, Lng: 3.3792, RecordedAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("IngestLocationPoints: %v", err)
	}

	// A deliberately absurd client-reported distance, to prove the server
	// value (a real, small haversine sum between two nearby Lagos points)
	// wins, not this.
	completed, err := svc.MarkCompleted(context.Background(), CompleteBookingInput{
		BookingID: bookingID, ProviderID: providerID, ActualDistanceKm: 5000, ActualDurationMin: 20,
	})
	if err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}
	if completed.ActualDistanceKm == nil {
		t.Fatal("ActualDistanceKm is nil")
	}
	if *completed.ActualDistanceKm >= 100 {
		t.Errorf("ActualDistanceKm = %v, want the small server-computed distance, not the 5000km client-reported figure", *completed.ActualDistanceKm)
	}
	if *completed.ActualDistanceKm <= 0 {
		t.Errorf("ActualDistanceKm = %v, want a positive real distance", *completed.ActualDistanceKm)
	}
}

func TestMarkCompleted_FallsBackToClientDistanceWithNoTelemetryPoints(t *testing.T) {
	db := lifecycleTestDB(t)
	svc, _ := newTelemetryIntegrationService(db)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusInProgress, availabilitymodels.StateOnTrip)

	// No IngestLocationPoints call at all for this booking.
	completed, err := svc.MarkCompleted(context.Background(), CompleteBookingInput{
		BookingID: bookingID, ProviderID: providerID, ActualDistanceKm: 12.5, ActualDurationMin: 20,
	})
	if err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}
	if completed.ActualDistanceKm == nil || *completed.ActualDistanceKm != 12.5 {
		t.Errorf("ActualDistanceKm = %v, want the client-reported 12.5 with no telemetry trail", completed.ActualDistanceKm)
	}
}
