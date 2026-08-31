package bookingusecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingrepositories "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	marketmodels "cosmicforge/logistics/services/taxi-service/internal/features/markets/models"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	"cosmicforge/logistics/services/taxi-service/internal/pricing"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/geo"
)

// ─── Fakes ──────────────────────────────────────────────────────────────────

type fakeBookingRepo struct {
	byID       map[string]bookingmodels.Booking
	activeFor  map[string]bool
	createErr  error
	createdSeq int
	events     []bookingmodels.BookingEvent
}

func newFakeBookingRepo() *fakeBookingRepo {
	return &fakeBookingRepo{byID: map[string]bookingmodels.Booking{}, activeFor: map[string]bool{}}
}

func (f *fakeBookingRepo) CreateBooking(ctx context.Context, booking bookingmodels.Booking, stops []bookingmodels.Stop) (bookingmodels.Booking, error) {
	if f.createErr != nil {
		return bookingmodels.Booking{}, f.createErr
	}
	f.createdSeq++
	booking.ID = "booking-" + itoa(f.createdSeq)
	booking.CreatedAt = time.Now()
	booking.UpdatedAt = booking.CreatedAt
	booking.Stops = make([]bookingmodels.Stop, len(stops))
	for i, s := range stops {
		s.ID = "stop-" + itoa(i+1)
		s.BookingID = booking.ID
		booking.Stops[i] = s
	}
	f.byID[booking.ID] = booking
	return booking, nil
}

func (f *fakeBookingRepo) GetByID(ctx context.Context, id string) (bookingmodels.Booking, error) {
	b, ok := f.byID[id]
	if !ok {
		return bookingmodels.Booking{}, apperrors.NotFound("Ride could not be found.", nil)
	}
	return b, nil
}

func (f *fakeBookingRepo) GetByIDForCustomer(ctx context.Context, id, customerID string) (bookingmodels.Booking, error) {
	b, ok := f.byID[id]
	if !ok || b.CustomerID != customerID {
		return bookingmodels.Booking{}, apperrors.NotFound("Ride could not be found.", nil)
	}
	return b, nil
}

func (f *fakeBookingRepo) ListByCustomer(ctx context.Context, customerID string, limit int) ([]bookingmodels.Booking, error) {
	var out []bookingmodels.Booking
	for _, b := range f.byID {
		if b.CustomerID == customerID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeBookingRepo) HasActiveBooking(ctx context.Context, customerID string) (bool, error) {
	return f.activeFor[customerID], nil
}

func (f *fakeBookingRepo) GetByIDForProvider(ctx context.Context, id, providerID string) (bookingmodels.Booking, error) {
	b, ok := f.byID[id]
	if !ok || b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.Booking{}, apperrors.NotFound("Ride could not be found.", nil)
	}
	return b, nil
}

func (f *fakeBookingRepo) ListTerminalByProviderInRange(ctx context.Context, providerID string, from, until time.Time) ([]bookingmodels.Booking, error) {
	var out []bookingmodels.Booking
	for _, b := range f.byID {
		if b.ProviderID == nil || *b.ProviderID != providerID {
			continue
		}
		if b.Status != bookingmodels.StatusCompleted && b.Status != bookingmodels.StatusCancelled {
			continue
		}
		if b.CreatedAt.Before(from) || b.CreatedAt.After(until) {
			continue
		}
		out = append(out, b)
	}
	return out, nil
}

func (f *fakeBookingRepo) ListDueScheduled(ctx context.Context, now time.Time) ([]string, error) {
	var ids []string
	for _, b := range f.byID {
		if b.Status != bookingmodels.StatusScheduled {
			continue
		}
		if b.DispatchStartAt == nil || b.DispatchStartAt.After(now) {
			continue
		}
		ids = append(ids, b.ID)
	}
	return ids, nil
}

func (f *fakeBookingRepo) PromoteScheduledToPendingMatch(ctx context.Context, bookingID string, matchingStartedAt time.Time) (bookingmodels.Booking, error) {
	b, ok := f.byID[bookingID]
	if !ok || b.Status != bookingmodels.StatusScheduled {
		return bookingmodels.Booking{}, apperrors.Conflict("This ride cannot be updated right now.", nil)
	}
	b.Status = bookingmodels.StatusPendingMatch
	b.MatchingStartedAt = &matchingStartedAt
	f.byID[bookingID] = b
	return b, nil
}

func (f *fakeBookingRepo) ListActive(ctx context.Context, marketID string) ([]bookingmodels.Booking, error) {
	var out []bookingmodels.Booking
	for _, b := range f.byID {
		if !b.IsActive() {
			continue
		}
		if marketID != "" && b.MarketID != marketID {
			continue
		}
		out = append(out, b)
	}
	return out, nil
}

func (f *fakeBookingRepo) ListUnmatched(ctx context.Context, marketID, zoneID string, olderThan time.Time) ([]bookingmodels.Booking, error) {
	var out []bookingmodels.Booking
	for _, b := range f.byID {
		if b.Status != bookingmodels.StatusUnmatched {
			continue
		}
		if marketID != "" && b.MarketID != marketID {
			continue
		}
		if zoneID != "" && b.OperatingZoneID != zoneID {
			continue
		}
		if !olderThan.IsZero() && b.CreatedAt.After(olderThan) {
			continue
		}
		out = append(out, b)
	}
	return out, nil
}

func (f *fakeBookingRepo) CountByStatus(ctx context.Context) (map[string]int64, error) {
	out := map[string]int64{}
	for _, b := range f.byID {
		out[string(b.Status)]++
	}
	return out, nil
}

func (f *fakeBookingRepo) ListStaleMatching(ctx context.Context, cutoff time.Time) ([]bookingmodels.Booking, error) {
	var out []bookingmodels.Booking
	for _, b := range f.byID {
		if b.Status != bookingmodels.StatusPendingMatch && b.Status != bookingmodels.StatusAwaitingAccept {
			continue
		}
		if b.UpdatedAt.After(cutoff) {
			continue
		}
		out = append(out, b)
	}
	return out, nil
}

func (f *fakeBookingRepo) ListStaleInProgress(ctx context.Context, cutoff time.Time) ([]bookingmodels.Booking, error) {
	var out []bookingmodels.Booking
	for _, b := range f.byID {
		if b.Status != bookingmodels.StatusInProgress || b.StartedAt == nil {
			continue
		}
		if b.StartedAt.After(cutoff) {
			continue
		}
		out = append(out, b)
	}
	return out, nil
}

func (f *fakeBookingRepo) AdminCancel(ctx context.Context, exec bookingrepositories.Executor, bookingID, reason string) (bookingmodels.Booking, error) {
	b, ok := f.byID[bookingID]
	if !ok || b.IsTerminal() {
		return bookingmodels.Booking{}, apperrors.Conflict("This ride cannot be updated right now.", nil)
	}
	b.Status = bookingmodels.StatusCancelled
	admin := "admin"
	b.CancelledBy = &admin
	b.CancelReason = &reason
	f.byID[bookingID] = b
	return b, nil
}

func (f *fakeBookingRepo) RecordEvent(ctx context.Context, exec bookingrepositories.Executor, event bookingmodels.BookingEvent) error {
	f.events = append(f.events, event)
	return nil
}

func (f *fakeBookingRepo) ListEvents(ctx context.Context, bookingID string) ([]bookingmodels.BookingEvent, error) {
	var out []bookingmodels.BookingEvent
	for _, e := range f.events {
		if e.BookingID == bookingID {
			out = append(out, e)
		}
	}
	return out, nil
}

// BeginTx is not exercised by this package's tests: the lifecycle
// orchestrator's own atomicity (a single pgx.Tx shared with the operational-
// state repository) is only meaningfully provable against a real Postgres
// transaction, and is covered by lifecycle_integration_test.go instead. A
// fake pgx.Tx would have to implement the entire interface for no real
// verification value.
func (f *fakeBookingRepo) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return nil, errors.New("fakeBookingRepo.BeginTx is not supported; see lifecycle_integration_test.go")
}

func (f *fakeBookingRepo) transition(id, providerOrCustomerID string, byProvider bool, from []bookingmodels.Status, to bookingmodels.Status, apply func(*bookingmodels.Booking)) (bookingmodels.Booking, error) {
	b, ok := f.byID[id]
	if !ok {
		return bookingmodels.Booking{}, apperrors.Conflict("This ride cannot be updated right now.", nil)
	}
	if byProvider {
		if b.ProviderID == nil || *b.ProviderID != providerOrCustomerID {
			return bookingmodels.Booking{}, apperrors.Conflict("This ride cannot be updated right now.", nil)
		}
	} else if b.CustomerID != providerOrCustomerID {
		return bookingmodels.Booking{}, apperrors.Conflict("This ride cannot be updated right now.", nil)
	}
	matched := false
	for _, s := range from {
		if b.Status == s {
			matched = true
			break
		}
	}
	if !matched {
		return bookingmodels.Booking{}, apperrors.Conflict("This ride cannot be updated right now.", nil)
	}
	b.Status = to
	if apply != nil {
		apply(&b)
	}
	f.byID[id] = b
	return b, nil
}

func (f *fakeBookingRepo) ListActiveByProvider(ctx context.Context, providerID string) ([]bookingmodels.Booking, error) {
	var out []bookingmodels.Booking
	for _, b := range f.byID {
		if b.ProviderID != nil && *b.ProviderID == providerID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeBookingRepo) MarkMatched(ctx context.Context, exec bookingrepositories.Executor, bookingID, providerID string, paymentIntentID *string) (bookingmodels.Booking, error) {
	b, ok := f.byID[bookingID]
	if !ok || b.Status != bookingmodels.StatusPendingMatch {
		return bookingmodels.Booking{}, apperrors.Conflict("This ride cannot be updated right now.", nil)
	}
	b.Status = bookingmodels.StatusAwaitingAccept
	b.ProviderID = &providerID
	b.PaymentIntentID = paymentIntentID
	f.byID[bookingID] = b
	return b, nil
}

func (f *fakeBookingRepo) SetPaymentStatus(ctx context.Context, exec bookingrepositories.Executor, bookingID, status string) error {
	b, ok := f.byID[bookingID]
	if !ok {
		return apperrors.Conflict("This ride cannot be updated right now.", nil)
	}
	b.PaymentStatus = status
	f.byID[bookingID] = b
	return nil
}

func (f *fakeBookingRepo) MarkAccepted(ctx context.Context, exec bookingrepositories.Executor, bookingID, providerID string) (bookingmodels.Booking, error) {
	return f.transition(bookingID, providerID, true, []bookingmodels.Status{bookingmodels.StatusAwaitingAccept}, bookingmodels.StatusAccepted, nil)
}

func (f *fakeBookingRepo) ResetToMatching(ctx context.Context, exec bookingrepositories.Executor, bookingID, providerID string) (bookingmodels.Booking, error) {
	return f.transition(bookingID, providerID, true, []bookingmodels.Status{bookingmodels.StatusAwaitingAccept}, bookingmodels.StatusPendingMatch, func(b *bookingmodels.Booking) {
		b.ProviderID = nil
	})
}

func (f *fakeBookingRepo) MarkUnmatched(ctx context.Context, exec bookingrepositories.Executor, bookingID, reason string) (bookingmodels.Booking, error) {
	b, ok := f.byID[bookingID]
	if !ok || b.Status != bookingmodels.StatusPendingMatch {
		return bookingmodels.Booking{}, apperrors.Conflict("This ride cannot be updated right now.", nil)
	}
	b.Status = bookingmodels.StatusUnmatched
	b.CancelReason = &reason
	f.byID[bookingID] = b
	return b, nil
}

func (f *fakeBookingRepo) MarkEnRoutePickup(ctx context.Context, exec bookingrepositories.Executor, bookingID, providerID string) (bookingmodels.Booking, error) {
	return f.transition(bookingID, providerID, true, []bookingmodels.Status{bookingmodels.StatusAccepted}, bookingmodels.StatusEnRoutePickup, nil)
}

func (f *fakeBookingRepo) MarkArrivedAtPickup(ctx context.Context, exec bookingrepositories.Executor, bookingID, providerID string) (bookingmodels.Booking, error) {
	return f.transition(bookingID, providerID, true, []bookingmodels.Status{bookingmodels.StatusEnRoutePickup}, bookingmodels.StatusArrivedAtPickup, func(b *bookingmodels.Booking) {
		b.PickupVerificationStatus = bookingmodels.PickupVerificationPending
	})
}

// MarkPickupVerified and MarkInProgress are bespoke (not routed through
// transition) because each needs an EXTRA guard on PickupVerificationStatus,
// not just Status -- mirroring the real repository's two-column WHERE
// clause (phase B5).
func (f *fakeBookingRepo) MarkPickupVerified(ctx context.Context, exec bookingrepositories.Executor, bookingID, providerID string) (bookingmodels.Booking, error) {
	b, ok := f.byID[bookingID]
	if !ok || b.ProviderID == nil || *b.ProviderID != providerID ||
		b.Status != bookingmodels.StatusArrivedAtPickup || b.PickupVerificationStatus != bookingmodels.PickupVerificationPending {
		return bookingmodels.Booking{}, apperrors.Conflict("This ride cannot be updated right now.", nil)
	}
	b.PickupVerificationStatus = bookingmodels.PickupVerificationVerified
	f.byID[bookingID] = b
	return b, nil
}

func (f *fakeBookingRepo) MarkInProgress(ctx context.Context, exec bookingrepositories.Executor, bookingID, providerID string) (bookingmodels.Booking, error) {
	b, ok := f.byID[bookingID]
	if !ok || b.ProviderID == nil || *b.ProviderID != providerID ||
		b.Status != bookingmodels.StatusArrivedAtPickup || b.PickupVerificationStatus != bookingmodels.PickupVerificationVerified {
		return bookingmodels.Booking{}, apperrors.Conflict("This ride cannot be updated right now.", nil)
	}
	b.Status = bookingmodels.StatusInProgress
	f.byID[bookingID] = b
	return b, nil
}

func (f *fakeBookingRepo) MarkCompleted(ctx context.Context, exec bookingrepositories.Executor, bookingID, providerID string, actualDistanceKm float64, fareFinalKobo int64) (bookingmodels.Booking, error) {
	return f.transition(bookingID, providerID, true, []bookingmodels.Status{bookingmodels.StatusInProgress}, bookingmodels.StatusCompleted, func(b *bookingmodels.Booking) {
		b.ActualDistanceKm = &actualDistanceKm
		b.FareFinalKobo = &fareFinalKobo
	})
}

func (f *fakeBookingRepo) CancelByCustomer(ctx context.Context, exec bookingrepositories.Executor, bookingID, customerID, reason string) (bookingmodels.Booking, error) {
	return f.transition(bookingID, customerID, false, bookingmodels.ActiveStatuses, bookingmodels.StatusCancelled, func(b *bookingmodels.Booking) {
		b.CancelReason = &reason
	})
}

func (f *fakeBookingRepo) CancelByProvider(ctx context.Context, exec bookingrepositories.Executor, bookingID, providerID, reason string) (bookingmodels.Booking, error) {
	return f.transition(bookingID, providerID, true, bookingmodels.ActiveStatuses, bookingmodels.StatusCancelled, func(b *bookingmodels.Booking) {
		b.CancelReason = &reason
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// fakeMarkets resolves any point inside the fixed test zone; everything else
// is "outside the service area", mirroring how a real NotFound from
// ResolveZone is meant to be interpreted.
type fakeMarkets struct {
	zone      marketmodels.Zone
	resolveAt func(p geo.Point) bool
}

func newFakeMarkets() *fakeMarkets {
	return &fakeMarkets{
		zone: marketmodels.Zone{
			ID: "zone-1", MarketID: "market-1", Name: "Test Zone", Code: "TEST",
			CenterLat: 1.0, CenterLng: -10.0, RadiusKm: 15,
			SupportedClasses: []string{"economy", "comfort"},
			Status:           "active",
		},
	}
}

func (f *fakeMarkets) ResolveZone(ctx context.Context, point geo.Point) (marketmodels.Zone, error) {
	if f.resolveAt != nil && !f.resolveAt(point) {
		return marketmodels.Zone{}, apperrors.NotFound("No zone covers this location.", nil)
	}
	return f.zone, nil
}

type fakePricing struct {
	fareByClass map[string]int64
	quoteErr    error
	lastReq     pricing.QuoteRequest
}

func newFakePricing() *fakePricing {
	return &fakePricing{fareByClass: map[string]int64{"economy": 150000, "comfort": 220000, "xl": 300000}}
}

func (f *fakePricing) Quote(ctx context.Context, req pricing.QuoteRequest) ([]pricing.Quote, error) {
	f.lastReq = req
	if f.quoteErr != nil {
		return nil, f.quoteErr
	}
	var out []pricing.Quote
	for _, class := range req.VehicleClasses {
		fare, ok := f.fareByClass[class]
		if !ok {
			continue
		}
		out = append(out, pricing.Quote{
			VehicleClass: class, FareKobo: fare, DistanceKm: 8.4, DurationMin: 21,
			QuoteReference: "quote-" + class, RateVersion: "test-v1",
			ExpiresAt: time.Now().Add(5 * time.Minute),
		})
	}
	return out, nil
}

func (f *fakePricing) Reconcile(ctx context.Context, req pricing.ReconcileRequest) (pricing.FinalFare, error) {
	return pricing.FinalFare{
		FareKobo: int64(req.ActualDistanceKm * 10000), QuoteReference: req.QuoteReference, RateVersion: "test-v1",
	}, nil
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func validStops() []StopInput {
	return []StopInput{
		{Address: "Pickup", Lat: 1.0005, Lng: -9.9995},
		{Address: "Dropoff", Lat: 1.02, Lng: -9.98},
	}
}

// fakeOperationalState is only used to satisfy NewBookingService's
// constructor for this file's CreateBooking/EstimateFare tests, which never
// exercise lifecycle transitions -- see lifecycle_integration_test.go for
// real coverage of the operational-state coupling itself.
type fakeOperationalState struct{}

func (fakeOperationalState) Get(ctx context.Context, providerID string) (availabilitymodels.State, error) {
	return availabilitymodels.State{ProviderID: providerID, Current: availabilitymodels.StateOffline, Version: 0}, nil
}

func (fakeOperationalState) Transition(ctx context.Context, exec bookingrepositories.Executor, providerID string, to availabilitymodels.OperationalState, reason string, bookingID *string, expectedVersion int64) (availabilitymodels.State, error) {
	return availabilitymodels.State{ProviderID: providerID, Current: to, Version: expectedVersion + 1}, nil
}

// newService omits every matching dependency (Availability/Providers/
// Vehicles/Routes/Lease) -- CreateBooking's spawnMatcher treats a nil Lease
// as "matching is not configured" and no-ops, so these tests exercise
// CreateBooking/EstimateFare without any of B2's machinery in play.
func newService(repo *fakeBookingRepo, markets *fakeMarkets, pc *fakePricing) *BookingService {
	return NewBookingService(Options{
		Bookings: repo, Markets: markets, Pricing: pc, OperationalState: fakeOperationalState{},
	})
}

func errCode(t *testing.T, err error) apperrors.Code {
	t.Helper()
	appErr := apperrors.From(err)
	if appErr == nil {
		t.Fatalf("expected an apperrors.Error, got %T: %v", err, err)
	}
	return appErr.Code
}

// ─── CreateBooking ──────────────────────────────────────────────────────────

func TestCreateBooking_Success(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	got, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID: "cust-1", VehicleClass: "economy", Stops: validStops(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != bookingmodels.StatusPendingMatch {
		t.Errorf("status = %q, want %q", got.Status, bookingmodels.StatusPendingMatch)
	}
	if got.FareEstimateKobo == nil || *got.FareEstimateKobo != 150000 {
		t.Errorf("fare estimate = %v, want 150000", got.FareEstimateKobo)
	}
	if got.MarketID != "market-1" || got.OperatingZoneID != "zone-1" {
		t.Errorf("zone resolution not applied: market=%s zone=%s", got.MarketID, got.OperatingZoneID)
	}
	if len(got.Stops) != 2 {
		t.Fatalf("stops = %d, want 2", len(got.Stops))
	}
	if got.Stops[0].Type != bookingmodels.StopTypePickup {
		t.Errorf("stops[0].Type = %q, want PICKUP", got.Stops[0].Type)
	}
	if got.Stops[1].Type != bookingmodels.StopTypeDropoff {
		t.Errorf("stops[1].Type = %q, want DROPOFF", got.Stops[1].Type)
	}
	// Booking creation must price only the chosen class, not every class the
	// zone supports -- that is what the separate estimate endpoint is for.
	if len(pc.lastReq.VehicleClasses) != 1 || pc.lastReq.VehicleClasses[0] != "economy" {
		t.Errorf("Quote called with classes %v, want exactly [economy]", pc.lastReq.VehicleClasses)
	}
}

func TestCreateBooking_MultiStopDerivesWaypointsFromPositionNotClientInput(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	stops := []StopInput{
		{Address: "Pickup", Lat: 1.0005, Lng: -9.9995},
		{Address: "Waypoint", Lat: 1.01, Lng: -9.99},
		{Address: "Dropoff", Lat: 1.02, Lng: -9.98},
	}
	got, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID: "cust-1", VehicleClass: "economy", BookingMode: "hourly",
		ReservedDurationMinutes: 60, Stops: stops,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Stops) != 3 {
		t.Fatalf("stops = %d, want 3", len(got.Stops))
	}
	wantTypes := []bookingmodels.StopType{bookingmodels.StopTypePickup, bookingmodels.StopTypeWaypoint, bookingmodels.StopTypeDropoff}
	for i, want := range wantTypes {
		if got.Stops[i].Type != want {
			t.Errorf("stops[%d].Type = %q, want %q", i, got.Stops[i].Type, want)
		}
		if got.Stops[i].Sequence != i+1 {
			t.Errorf("stops[%d].Sequence = %d, want %d", i, got.Stops[i].Sequence, i+1)
		}
	}
}

func TestCreateBooking_RequiresAtLeastTwoStops(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	_, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID: "cust-1", VehicleClass: "economy",
		Stops: []StopInput{{Address: "Only one", Lat: 1.0, Lng: -10.0}},
	})
	if errCode(t, err) != apperrors.CodeValidationFailed {
		t.Fatalf("code = %v, want validation_failed", errCode(t, err))
	}
}

func TestCreateBooking_RejectsInvalidCoordinates(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	stops := validStops()
	stops[1].Lat, stops[1].Lng = 0, 0 // null island: HasFix() is false
	_, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID: "cust-1", VehicleClass: "economy", Stops: stops,
	})
	if errCode(t, err) != apperrors.CodeValidationFailed {
		t.Fatalf("code = %v, want validation_failed", errCode(t, err))
	}
}

func TestCreateBooking_RejectsInvalidVehicleClass(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	_, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID: "cust-1", VehicleClass: "helicopter", Stops: validStops(),
	})
	if errCode(t, err) != apperrors.CodeValidationFailed {
		t.Fatalf("code = %v, want validation_failed", errCode(t, err))
	}
}

func TestCreateBooking_RejectsWhenZoneDoesNotSupportClass(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	// The fake zone supports only economy/comfort.
	_, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID: "cust-1", VehicleClass: "xl", Stops: validStops(),
	})
	if errCode(t, err) != apperrors.CodeValidationFailed {
		t.Fatalf("code = %v, want validation_failed", errCode(t, err))
	}
}

func TestCreateBooking_RejectsUnmappedLocation(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	markets.resolveAt = func(p geo.Point) bool { return false }
	svc := newService(repo, markets, pc)

	_, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID: "cust-1", VehicleClass: "economy", Stops: validStops(),
	})
	// An unmapped location is a fact about what the passenger typed -- surfaced
	// as a validation error, not an internal NotFound.
	if errCode(t, err) != apperrors.CodeValidationFailed {
		t.Fatalf("code = %v, want validation_failed", errCode(t, err))
	}
}

func TestCreateBooking_BlocksSecondActiveBooking(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	repo.activeFor["cust-1"] = true
	svc := newService(repo, markets, pc)

	_, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID: "cust-1", VehicleClass: "economy", Stops: validStops(),
	})
	if errCode(t, err) != apperrors.CodeConflict {
		t.Fatalf("code = %v, want conflict", errCode(t, err))
	}
}

func TestCreateBooking_HourlyRequiresReservedDuration(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	_, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID: "cust-1", VehicleClass: "economy", BookingMode: "hourly", Stops: validStops(),
	})
	if errCode(t, err) != apperrors.CodeValidationFailed {
		t.Fatalf("code = %v, want validation_failed", errCode(t, err))
	}
}

func TestCreateBooking_PassengerCountCappedByVehicleClass(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	// economy.MaxPassengers is 3 (see internal/vehicleclass).
	_, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID: "cust-1", VehicleClass: "economy", PassengerCount: 4, Stops: validStops(),
	})
	if errCode(t, err) != apperrors.CodeValidationFailed {
		t.Fatalf("code = %v, want validation_failed", errCode(t, err))
	}
}

func TestCreateBooking_DefaultsPaymentMethodToWallet(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	got, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID: "cust-1", VehicleClass: "economy", Stops: validStops(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.PaymentMethod != "wallet" {
		t.Errorf("payment method = %q, want wallet", got.PaymentMethod)
	}
	if got.PaymentStatus != "unpaid" {
		t.Errorf("payment status = %q, want unpaid", got.PaymentStatus)
	}
}

func TestCreateBooking_RejectsInvalidPaymentMethod(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	_, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID: "cust-1", VehicleClass: "economy", PaymentMethod: "crypto", Stops: validStops(),
	})
	if errCode(t, err) != apperrors.CodeValidationFailed {
		t.Fatalf("code = %v, want validation_failed", errCode(t, err))
	}
}

func TestCreateBooking_RequiresAuthenticatedCustomer(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	_, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		VehicleClass: "economy", Stops: validStops(),
	})
	if errCode(t, err) != apperrors.CodeUnauthorized {
		t.Fatalf("code = %v, want unauthorized", errCode(t, err))
	}
}

// ─── EstimateFare ───────────────────────────────────────────────────────────

func TestEstimateFare_ReturnsOneTilePerSupportedClass(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	tiles, err := svc.EstimateFare(context.Background(), EstimateFareInput{
		PickupLat: 1.0005, PickupLng: -9.9995, DropoffLat: 1.02, DropoffLng: -9.98,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The fake zone supports economy + comfort only (not xl).
	if len(tiles) != 2 {
		t.Fatalf("tiles = %d, want 2", len(tiles))
	}
	for _, tile := range tiles {
		if tile.VehicleClass != "economy" && tile.VehicleClass != "comfort" {
			t.Errorf("unexpected class in tiles: %s", tile.VehicleClass)
		}
		if tile.Label == "" {
			t.Errorf("tile for %s has no label", tile.VehicleClass)
		}
	}
}

func TestEstimateFare_RequiresBothPickupAndDropoff(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	_, err := svc.EstimateFare(context.Background(), EstimateFareInput{PickupLat: 1.0005, PickupLng: -9.9995})
	if errCode(t, err) != apperrors.CodeValidationFailed {
		t.Fatalf("code = %v, want validation_failed", errCode(t, err))
	}
}

func TestEstimateFare_UnmappedLocationReturnsValidationError(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	markets.resolveAt = func(p geo.Point) bool { return false }
	svc := newService(repo, markets, pc)

	_, err := svc.EstimateFare(context.Background(), EstimateFareInput{
		PickupLat: 1.0005, PickupLng: -9.9995, DropoffLat: 1.02, DropoffLng: -9.98,
	})
	if errCode(t, err) != apperrors.CodeValidationFailed {
		t.Fatalf("code = %v, want validation_failed", errCode(t, err))
	}
}

// ─── Reads ──────────────────────────────────────────────────────────────────

func TestGetByIDForCustomer_EmptyIDReturnsNotFound(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	_, err := svc.GetByIDForCustomer(context.Background(), "", "cust-1")
	if errCode(t, err) != apperrors.CodeNotFound {
		t.Fatalf("code = %v, want not_found", errCode(t, err))
	}
}

func TestGetByIDForCustomer_WrongCustomerReturnsNotFound(t *testing.T) {
	repo, markets, pc := newFakeBookingRepo(), newFakeMarkets(), newFakePricing()
	svc := newService(repo, markets, pc)

	created, err := svc.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID: "cust-1", VehicleClass: "economy", Stops: validStops(),
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err = svc.GetByIDForCustomer(context.Background(), created.ID, "cust-2")
	if errCode(t, err) != apperrors.CodeNotFound {
		t.Fatalf("code = %v, want not_found", errCode(t, err))
	}
}
