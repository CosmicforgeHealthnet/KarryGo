package bookingusecases

import (
	"context"
	"errors"
	"testing"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingtelemetry "cosmicforge/logistics/services/taxi-service/internal/features/booking/telemetry"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────

type fakeTelemetryRepo struct {
	stored      []bookingtelemetry.Point
	latestByKey map[string]bookingtelemetry.Point
	insertErr   error
	latestErr   error
}

func newFakeTelemetryRepo() *fakeTelemetryRepo {
	return &fakeTelemetryRepo{latestByKey: map[string]bookingtelemetry.Point{}}
}

func telemetryKey(bookingID, providerID string) string { return bookingID + "|" + providerID }

func (f *fakeTelemetryRepo) Insert(ctx context.Context, p bookingtelemetry.Point) (bool, error) {
	if f.insertErr != nil {
		return false, f.insertErr
	}
	key := telemetryKey(p.BookingID, p.ProviderID)
	if existing, ok := f.latestByKey[key]; ok && existing.Sequence == p.Sequence {
		return false, nil // mirrors the real ON CONFLICT DO NOTHING semantics
	}
	f.stored = append(f.stored, p)
	if existing, ok := f.latestByKey[key]; !ok || p.Sequence > existing.Sequence {
		f.latestByKey[key] = p
	}
	return true, nil
}

func (f *fakeTelemetryRepo) Latest(ctx context.Context, bookingID, providerID string) (bookingtelemetry.Point, bool, error) {
	if f.latestErr != nil {
		return bookingtelemetry.Point{}, false, f.latestErr
	}
	p, ok := f.latestByKey[telemetryKey(bookingID, providerID)]
	return p, ok, nil
}

func (f *fakeTelemetryRepo) ValidatedTrail(ctx context.Context, bookingID string) ([]bookingtelemetry.Point, error) {
	var out []bookingtelemetry.Point
	for _, p := range f.stored {
		if p.BookingID == bookingID && len(p.IntegrityFlags) == 0 {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeTelemetryRepo) ListAll(ctx context.Context, bookingID string) ([]bookingtelemetry.Point, error) {
	var out []bookingtelemetry.Point
	for _, p := range f.stored {
		if p.BookingID == bookingID {
			out = append(out, p)
		}
	}
	return out, nil
}

type fakePositionUpdater struct {
	calls []availabilityrepositories.ProviderStatus
	err   error
}

func (f *fakePositionUpdater) Heartbeat(ctx context.Context, status availabilityrepositories.ProviderStatus) error {
	f.calls = append(f.calls, status)
	return f.err
}

func newTelemetryTestService(bookings *fakeBookingRepo, telemetry bookingtelemetry.Repository, positions PositionUpdater) *BookingService {
	return NewBookingService(Options{
		Bookings:  bookings,
		Telemetry: telemetry,
		Positions: positions,
		TelemetryConfig: bookingtelemetry.Config{
			MaxSpeedKph: 180, StaleThreshold: 120 * time.Second,
		},
	})
}

func activeBooking(id, providerID string) bookingmodels.Booking {
	pid := providerID
	return bookingmodels.Booking{
		ID: id, ProviderID: &pid, Status: bookingmodels.StatusInProgress,
		MarketID: "market-1", VehicleClass: "economy",
	}
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func TestIngestLocationPoints_NilTelemetryIsUnavailable(t *testing.T) {
	svc := NewBookingService(Options{Bookings: newFakeBookingRepo()})
	_, err := svc.IngestLocationPoints(context.Background(), "provider-1", []LocationPointInput{{BookingID: "b1", Sequence: 1, Lat: 6.5, Lng: 3.4, RecordedAt: time.Now()}})
	if err == nil {
		t.Fatal("expected an error when Telemetry is not configured")
	}
}

func TestIngestLocationPoints_EmptyBatchIsRejected(t *testing.T) {
	svc := newTelemetryTestService(newFakeBookingRepo(), newFakeTelemetryRepo(), nil)
	_, err := svc.IngestLocationPoints(context.Background(), "provider-1", nil)
	if err == nil {
		t.Fatal("expected an error for an empty batch")
	}
}

func TestIngestLocationPoints_InvalidCoordinateRejectedPerPoint(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["b1"] = activeBooking("b1", "provider-1")
	svc := newTelemetryTestService(bookings, newFakeTelemetryRepo(), nil)

	results, err := svc.IngestLocationPoints(context.Background(), "provider-1", []LocationPointInput{
		{BookingID: "b1", Sequence: 1, Lat: 0, Lng: 0, RecordedAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("unexpected batch-level error: %v", err)
	}
	if len(results) != 1 || results[0].Error == "" || results[0].Stored {
		t.Errorf("results = %+v, want one rejected (not stored) result for null-island", results)
	}
}

func TestIngestLocationPoints_WrongProviderRejectedPerPoint(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["b1"] = activeBooking("b1", "provider-1")
	svc := newTelemetryTestService(bookings, newFakeTelemetryRepo(), nil)

	results, err := svc.IngestLocationPoints(context.Background(), "someone-else", []LocationPointInput{
		{BookingID: "b1", Sequence: 1, Lat: 6.5, Lng: 3.4, RecordedAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("unexpected batch-level error: %v", err)
	}
	if len(results) != 1 || results[0].Error == "" || results[0].Stored {
		t.Errorf("results = %+v, want the point rejected as not assigned to this provider", results)
	}
}

func TestIngestLocationPoints_InactiveBookingRejectedPerPoint(t *testing.T) {
	bookings := newFakeBookingRepo()
	b := activeBooking("b1", "provider-1")
	b.Status = bookingmodels.StatusCompleted
	bookings.byID["b1"] = b
	svc := newTelemetryTestService(bookings, newFakeTelemetryRepo(), nil)

	results, err := svc.IngestLocationPoints(context.Background(), "provider-1", []LocationPointInput{
		{BookingID: "b1", Sequence: 1, Lat: 6.5, Lng: 3.4, RecordedAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("unexpected batch-level error: %v", err)
	}
	if len(results) != 1 || results[0].Error == "" || results[0].Stored {
		t.Errorf("results = %+v, want the point rejected -- booking is completed, not active", results)
	}
}

func TestIngestLocationPoints_CleanPointStoredAndPositionUpdated(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["b1"] = activeBooking("b1", "provider-1")
	telemetry := newFakeTelemetryRepo()
	positions := &fakePositionUpdater{}
	svc := newTelemetryTestService(bookings, telemetry, positions)

	results, err := svc.IngestLocationPoints(context.Background(), "provider-1", []LocationPointInput{
		{BookingID: "b1", Sequence: 1, Lat: 6.5244, Lng: 3.3792, RecordedAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("IngestLocationPoints: %v", err)
	}
	if len(results) != 1 || !results[0].Stored || results[0].Error != "" || len(results[0].IntegrityFlags) != 0 {
		t.Errorf("results = %+v, want one clean stored point", results)
	}
	if len(telemetry.stored) != 1 {
		t.Fatalf("stored = %d points, want 1", len(telemetry.stored))
	}
	if len(positions.calls) != 1 {
		t.Fatalf("position updater calls = %d, want 1 for a clean point", len(positions.calls))
	}
	if positions.calls[0].MarketID != "market-1" || positions.calls[0].VehicleClass != "economy" {
		t.Errorf("position update = %+v, want market/class sourced from the BOOKING, not the request", positions.calls[0])
	}
}

func TestIngestLocationPoints_FlaggedPointStoredButPositionNotUpdated(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["b1"] = activeBooking("b1", "provider-1")
	telemetry := newFakeTelemetryRepo()
	// Seed a "last known" point far away so the very next point in this
	// batch reads as an impossible jump.
	telemetry.latestByKey[telemetryKey("b1", "provider-1")] = bookingtelemetry.Point{
		BookingID: "b1", ProviderID: "provider-1", Sequence: 1,
		Lat: 6.5244, Lng: 3.3792, RecordedAt: time.Now().Add(-5 * time.Second),
	}
	telemetry.stored = append(telemetry.stored, telemetry.latestByKey[telemetryKey("b1", "provider-1")])
	positions := &fakePositionUpdater{}
	svc := newTelemetryTestService(bookings, telemetry, positions)

	results, err := svc.IngestLocationPoints(context.Background(), "provider-1", []LocationPointInput{
		{BookingID: "b1", Sequence: 2, Lat: 51.5072, Lng: -0.1276, RecordedAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("IngestLocationPoints: %v", err)
	}
	if len(results) != 1 || !results[0].Stored {
		t.Fatalf("results = %+v, want the flagged point still Stored=true", results)
	}
	if len(results[0].IntegrityFlags) == 0 {
		t.Error("want the impossible-jump flag surfaced in the result")
	}
	if len(positions.calls) != 0 {
		t.Error("position updater must not be called for a flagged point")
	}
}

func TestIngestLocationPoints_BatchContinuityClassifiesAgainstThePriorPointInTheSameBatch(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["b1"] = activeBooking("b1", "provider-1")
	telemetry := newFakeTelemetryRepo()
	positions := &fakePositionUpdater{}
	svc := newTelemetryTestService(bookings, telemetry, positions)

	now := time.Now()
	results, err := svc.IngestLocationPoints(context.Background(), "provider-1", []LocationPointInput{
		{BookingID: "b1", Sequence: 1, Lat: 6.5244, Lng: 3.3792, RecordedAt: now.Add(-10 * time.Second)},
		// A huge jump from POINT 1 (same batch), not from whatever was in the DB before this call.
		{BookingID: "b1", Sequence: 2, Lat: 51.5072, Lng: -0.1276, RecordedAt: now},
	})
	if err != nil {
		t.Fatalf("IngestLocationPoints: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if len(results[0].IntegrityFlags) != 0 {
		t.Errorf("first point flags = %v, want none (nothing to compare it against yet)", results[0].IntegrityFlags)
	}
	if len(results[1].IntegrityFlags) == 0 {
		t.Error("second point should be flagged as an impossible jump from the first point IN THIS BATCH")
	}
	// Only the first (clean) point should have updated the live position.
	if len(positions.calls) != 1 {
		t.Errorf("position updater calls = %d, want exactly 1 (only the clean first point)", len(positions.calls))
	}
}

func TestIngestLocationPoints_DuplicateResendNotRestored(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["b1"] = activeBooking("b1", "provider-1")
	telemetry := newFakeTelemetryRepo()
	svc := newTelemetryTestService(bookings, telemetry, nil)

	in := []LocationPointInput{{BookingID: "b1", Sequence: 1, Lat: 6.5, Lng: 3.4, RecordedAt: time.Now()}}
	if _, err := svc.IngestLocationPoints(context.Background(), "provider-1", in); err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	results, err := svc.IngestLocationPoints(context.Background(), "provider-1", in)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if results[0].Stored {
		t.Error("resending the same sequence must report Stored=false, not a second insert")
	}
}

func TestIngestLocationPoints_PositionUpdateFailureDoesNotFailIngestion(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["b1"] = activeBooking("b1", "provider-1")
	telemetry := newFakeTelemetryRepo()
	positions := &fakePositionUpdater{err: errors.New("redis unreachable")}
	svc := newTelemetryTestService(bookings, telemetry, positions)

	results, err := svc.IngestLocationPoints(context.Background(), "provider-1", []LocationPointInput{
		{BookingID: "b1", Sequence: 1, Lat: 6.5, Lng: 3.4, RecordedAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("IngestLocationPoints must not fail when the position mirror fails: %v", err)
	}
	if !results[0].Stored {
		t.Error("the durable point must still be reported stored even though the live-position mirror failed")
	}
}

func TestIngestLocationPoints_NilPositionUpdaterDoesNotPanic(t *testing.T) {
	bookings := newFakeBookingRepo()
	bookings.byID["b1"] = activeBooking("b1", "provider-1")
	svc := newTelemetryTestService(bookings, newFakeTelemetryRepo(), nil)

	results, err := svc.IngestLocationPoints(context.Background(), "provider-1", []LocationPointInput{
		{BookingID: "b1", Sequence: 1, Lat: 6.5, Lng: 3.4, RecordedAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("IngestLocationPoints: %v", err)
	}
	if !results[0].Stored {
		t.Error("want the point stored even with no PositionUpdater configured")
	}
}

// ─── resolveActualDistanceKm / MarkCompleted preference ────────────────────

func TestResolveActualDistanceKm_FallsBackWhenTelemetryNil(t *testing.T) {
	svc := NewBookingService(Options{Bookings: newFakeBookingRepo()})
	got := svc.resolveActualDistanceKm(context.Background(), "b1", 12.5)
	if got != 12.5 {
		t.Errorf("got %v, want the client-reported value 12.5 when telemetry is nil", got)
	}
}

func TestResolveActualDistanceKm_PrefersValidatedTrail(t *testing.T) {
	telemetry := newFakeTelemetryRepo()
	telemetry.stored = []bookingtelemetry.Point{
		{BookingID: "b1", Sequence: 1, Lat: 6.5244, Lng: 3.3792},
		{BookingID: "b1", Sequence: 2, Lat: 6.6000, Lng: 3.4000},
	}
	svc := newTelemetryTestService(newFakeBookingRepo(), telemetry, nil)

	clientReported := 999.0 // deliberately absurd, to prove it is NOT what gets used
	got := svc.resolveActualDistanceKm(context.Background(), "b1", clientReported)
	if got == clientReported {
		t.Error("want the server-computed trail distance, not the client-reported figure")
	}
	if got <= 0 {
		t.Errorf("got %v, want a positive trail-derived distance", got)
	}
}

func TestResolveActualDistanceKm_FallsBackWhenTrailHasFewerThanTwoCleanPoints(t *testing.T) {
	telemetry := newFakeTelemetryRepo()
	telemetry.stored = []bookingtelemetry.Point{
		{BookingID: "b1", Sequence: 1, Lat: 6.5244, Lng: 3.3792},
	}
	svc := newTelemetryTestService(newFakeBookingRepo(), telemetry, nil)

	got := svc.resolveActualDistanceKm(context.Background(), "b1", 12.5)
	if got != 12.5 {
		t.Errorf("got %v, want the client-reported fallback with fewer than 2 clean points", got)
	}
}
