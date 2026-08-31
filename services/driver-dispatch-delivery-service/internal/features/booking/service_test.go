package booking

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/serviceability"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/handover"
)

// --- fakes ---

type fakeRepo struct {
	mu           sync.Mutex
	bookings     map[string]*Booking
	broadcast    map[string]string // bookingID -> broadcast status
	tripID       map[string]string
	tripState    map[string]string
	tripProv     map[string]string
	reviews      map[string]Review
	idempotency  map[string]string
	fingerprints map[string]string
	operations   *BookingOperationsProjection
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		bookings:     map[string]*Booking{},
		broadcast:    map[string]string{},
		tripID:       map[string]string{},
		tripState:    map[string]string{},
		tripProv:     map[string]string{},
		reviews:      map[string]Review{},
		idempotency:  map[string]string{},
		fingerprints: map[string]string{},
	}
}

func (r *fakeRepo) InsertIdempotent(ctx context.Context, booking Booking, key, fingerprint, _ string, _ int64, _ string) (Booking, bool, error) {
	r.mu.Lock()
	if id := r.idempotency[key]; id != "" {
		if r.fingerprints[key] != fingerprint {
			r.mu.Unlock()
			return Booking{}, false, apperrors.Conflict("Idempotency-Key was already used for different booking details.", nil)
		}
		stored := *r.bookings[id]
		r.mu.Unlock()
		return stored, true, nil
	}
	r.mu.Unlock()
	created, err := r.Insert(ctx, booking)
	if err != nil {
		return Booking{}, false, err
	}
	r.mu.Lock()
	r.idempotency[key] = created.ID
	r.fingerprints[key] = fingerprint
	r.mu.Unlock()
	return created, false, nil
}

func (r *fakeRepo) Insert(_ context.Context, booking Booking) (Booking, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	booking.ID = uuid.NewString()
	booking.CreatedAt = time.Now().UTC()
	booking.UpdatedAt = booking.CreatedAt
	saved := booking
	r.bookings[booking.ID] = &saved
	return booking, nil
}

func (r *fakeRepo) state(id string) (DerivedState, bool) {
	booking, ok := r.bookings[id]
	if !ok {
		return DerivedState{}, false
	}
	return DerivedState{
		Booking:         *booking,
		BroadcastStatus: r.broadcast[id],
		TripID:          r.tripID[id],
		TripStatus:      r.tripState[id],
		TripProviderID:  r.tripProv[id],
	}, true
}

func (r *fakeRepo) GetWithDerivedState(_ context.Context, id string) (DerivedState, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state, ok := r.state(id)
	return state, ok, nil
}

func (r *fakeRepo) GetBookingOperations(_ context.Context, _ string) (*BookingOperationsProjection, error) {
	return r.operations, nil
}

func (r *fakeRepo) ListWithDerivedState(_ context.Context, customerID string, _ ListBookingsOptions) ([]DerivedState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var states []DerivedState
	for id, booking := range r.bookings {
		if booking.CustomerID == customerID {
			state, _ := r.state(id)
			states = append(states, state)
		}
	}
	return states, nil
}

func (r *fakeRepo) SetPayment(_ context.Context, id, paymentStatus, paymentRef string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if booking, ok := r.bookings[id]; ok {
		booking.PaymentStatus = paymentStatus
		booking.PaymentRef = paymentRef
	}
	return nil
}

func (r *fakeRepo) SetAccepted(_ context.Context, id, providerID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	booking, ok := r.bookings[id]
	if !ok || booking.Status != StatusSearching {
		return false, nil
	}
	booking.Status = StatusAccepted
	booking.ProviderID = providerID
	return true, nil
}

func (r *fakeRepo) SetUnmatched(_ context.Context, id string, at time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	booking, ok := r.bookings[id]
	if !ok || booking.Status != StatusSearching {
		return false, nil
	}
	booking.Status = StatusUnmatched
	booking.CancelReason = "no_provider_found"
	booking.CancelledAt = &at
	return true, nil
}

func (r *fakeRepo) SetCompleted(_ context.Context, id string, completedAt time.Time, paymentStatus string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	booking, ok := r.bookings[id]
	if !ok || booking.Status == StatusCompleted || booking.Status == StatusCancelled {
		return false, nil
	}
	booking.Status = StatusCompleted
	booking.CompletedAt = &completedAt
	booking.PaymentStatus = paymentStatus
	return true, nil
}

func (r *fakeRepo) SetTerminal(_ context.Context, id, status, paymentStatus, reason string, at time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	booking, ok := r.bookings[id]
	if !ok {
		return false, nil
	}
	switch booking.Status {
	case StatusCompleted, StatusCancelled, StatusUnmatched:
		return false, nil
	}
	booking.Status = status
	booking.PaymentStatus = paymentStatus
	booking.CancelReason = reason
	booking.CancelledAt = &at
	return true, nil
}

func (r *fakeRepo) InsertReview(_ context.Context, review Review) (Review, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.reviews[review.BookingID]; exists {
		return Review{}, apperrors.Conflict("This delivery has already been reviewed.", nil)
	}
	review.ID = uuid.NewString()
	review.CreatedAt = time.Now().UTC()
	r.reviews[review.BookingID] = review
	return review, nil
}

func (r *fakeRepo) GetReviewByBookingID(_ context.Context, bookingID string) (Review, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	review, ok := r.reviews[bookingID]
	return review, ok, nil
}

type fakeBroadcast struct {
	mu           sync.Mutex
	startErr     error
	started      []string
	cancelled    []string
	acceptOnRead func(bookingID string) // simulate accept race during cancel
	repo         *fakeRepo
}

func (b *fakeBroadcast) StartBroadcast(_ context.Context, event requestfeature.BookingDispatchCreatedEvent) (requestfeature.RequestBroadcast, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.startErr != nil {
		return requestfeature.RequestBroadcast{}, b.startErr
	}
	b.started = append(b.started, event.BookingID)
	if b.repo != nil {
		b.repo.mu.Lock()
		b.repo.broadcast[event.BookingID] = "broadcasting"
		b.repo.mu.Unlock()
	}
	return requestfeature.RequestBroadcast{BookingID: event.BookingID, Status: requestfeature.BroadcastStatusBroadcasting}, nil
}

func (b *fakeBroadcast) CancelBroadcastForBooking(_ context.Context, bookingID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.acceptOnRead != nil {
		b.acceptOnRead(bookingID)
		return nil
	}
	b.cancelled = append(b.cancelled, bookingID)
	if b.repo != nil {
		b.repo.mu.Lock()
		if b.repo.broadcast[bookingID] == "broadcasting" || b.repo.broadcast[bookingID] == "" {
			b.repo.broadcast[bookingID] = "cancelled"
		}
		b.repo.mu.Unlock()
	}
	return nil
}

type refundCall struct {
	bookingID string
	idemKey   string
}

type fakePayments struct {
	mu            sync.Mutex
	holdErr       error
	holds         []string
	assigns       []string
	refunds       []refundCall
	refundErr     error
	settlementErr error
	settlements   []string
	holdRefFor    func(bookingID string) string
}

func (p *fakePayments) CompleteSettlement(_ context.Context, bookingID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.settlements = append(p.settlements, bookingID)
	return p.settlementErr
}

func (p *fakePayments) Hold(_ context.Context, bookingID, _ string, _ int64) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.holdErr != nil {
		return "", p.holdErr
	}
	p.holds = append(p.holds, bookingID)
	if p.holdRefFor != nil {
		return p.holdRefFor(bookingID), nil
	}
	return "pi_" + bookingID, nil
}

func (p *fakePayments) AssignProvider(_ context.Context, paymentRef, providerID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.assigns = append(p.assigns, paymentRef+":"+providerID)
	return nil
}

func (p *fakePayments) Refund(_ context.Context, bookingID, _ string, _ int64, _, _, idempotencyKey string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.refundErr != nil {
		return p.refundErr
	}
	p.refunds = append(p.refunds, refundCall{bookingID: bookingID, idemKey: idempotencyKey})
	return nil
}

type fakeTrips struct {
	mu        sync.Mutex
	cancelled []string
	// afterCancel lets tests set the resulting trip status (e.g. still in_progress).
	afterCancel func(bookingID string)

	// verifyPickupErr, when set, is returned by every VerifyPickupCode call.
	// Otherwise a call succeeds only when the submitted code matches validPickupCode.
	verifyPickupErr  error
	validPickupCode  string
	verifiedBookings []string

	// resendDeliveryErr, when set, is returned by every ResendDeliveryCode call.
	resendDeliveryErr  error
	resendDeliveryCode handover.CodeRecord
	resentBookings     []string
}

func (t *fakeTrips) CancelForCustomer(_ context.Context, bookingID, _ string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cancelled = append(t.cancelled, bookingID)
	if t.afterCancel != nil {
		t.afterCancel(bookingID)
	}
	return nil
}

func (t *fakeTrips) VerifyPickupCode(_ context.Context, bookingID, code string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.verifyPickupErr != nil {
		return t.verifyPickupErr
	}
	if t.validPickupCode != "" && code != t.validPickupCode {
		return apperrors.Unauthorized("Incorrect code. Please try again.", nil)
	}
	t.verifiedBookings = append(t.verifiedBookings, bookingID)
	return nil
}

func (t *fakeTrips) ResendDeliveryCode(_ context.Context, bookingID string) (handover.CodeRecord, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.resendDeliveryErr != nil {
		return handover.CodeRecord{}, t.resendDeliveryErr
	}
	t.resentBookings = append(t.resentBookings, bookingID)
	return t.resendDeliveryCode, nil
}

type fakeLive struct {
	nearby   []availability.NearbyProvider
	location availability.Location
	hasLoc   bool
}

type fakeQuoteAuthority struct {
	quote serviceability.Quote
	err   error
}

func (a fakeQuoteAuthority) AuthorizeBooking(_ context.Context, _, _ string, _ serviceability.BookingBinding, _ time.Time) (serviceability.Quote, error) {
	return a.quote, a.err
}

func (l *fakeLive) GetNearby(_ context.Context, _ availability.NearbyProvidersRequest) ([]availability.NearbyProvider, error) {
	return l.nearby, nil
}

func (l *fakeLive) GetLocation(_ context.Context, providerID string) (availability.Location, bool, error) {
	if !l.hasLoc {
		return availability.Location{}, false, nil
	}
	loc := l.location
	loc.ProviderID = providerID
	return loc, true, nil
}

type noopEvents struct{ published []string }

func (e *noopEvents) PublishBookingCancelled(_ context.Context, bookingID, _ string, _ time.Time) error {
	e.published = append(e.published, bookingID)
	return nil
}

func newTestService(repo *fakeRepo, broadcast *fakeBroadcast, trips *fakeTrips, live *fakeLive, payments *fakePayments) *Service {
	return NewService(repo, broadcast, trips, live, payments, &noopEvents{}, 10)
}

const testCustomer = "11111111-1111-1111-1111-111111111111"

func validCreateInput() CreateBookingInput {
	return CreateBookingInput{
		Pickup:        LatLngAddress{Lat: 6.5244, Lng: 3.3792, Address: "Surulere, Lagos"},
		Dropoff:       LatLngAddress{Lat: 6.6018, Lng: 3.3515, Address: "Ikeja, Lagos"},
		ReceiverName:  "Ada Obi",
		ReceiverPhone: "+2348012345678",
		PackageDesc:   "Documents",
		PackageSize:   "small",
		ServiceTier:   "standard",
		PaymentMethod: "wallet",
	}
}

func TestGetBookingIncludesAdditiveOperationsProjection(t *testing.T) {
	repository := newFakeRepo()
	service := newTestService(repository, &fakeBroadcast{repo: repository}, &fakeTrips{}, &fakeLive{}, &fakePayments{})
	created := createSearchingBooking(t, service)
	repository.operations = &BookingOperationsProjection{
		DeliveryGroup: &DeliveryGroupSummary{ID: "group-1", State: "ACTIVE", Version: 3},
		Handover:      &HandoverSummary{Stage: "DELIVERY", Status: "ACTIVE"},
	}

	result, err := service.GetBooking(context.Background(), testCustomer, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Operations == nil || result.Operations.DeliveryGroup == nil || result.Operations.DeliveryGroup.ID != "group-1" {
		t.Fatalf("operations projection missing: %+v", result.Operations)
	}
}

// --- pricing ---

func TestCalculateFareMatchesGatewayFormula(t *testing.T) {
	cases := []struct {
		name string
		km   float64
		size string
		tier string
		want int64
	}{
		{"min fare short trip", 0.5, "small", "standard", 100000},
		{"5km small standard", 5, "small", "standard", 170000},
		{"5km medium standard", 5, "medium", "standard", 200000},
		{"5km large standard", 5, "large", "standard", 240000},
		{"5km small express", 5, "small", "express", 229500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := calculateFareKobo(tc.km, tc.size, tc.tier); got != tc.want {
				t.Fatalf("calculateFareKobo(%v,%s,%s) = %d, want %d", tc.km, tc.size, tc.tier, got, tc.want)
			}
		})
	}
}

// --- status derivation ---

func TestWireStatusDerivation(t *testing.T) {
	cases := []struct {
		name            string
		bookingStatus   string
		cancelled       bool
		broadcastStatus string
		tripStatus      string
		want            string
	}{
		{"no broadcast yet", StatusSearching, false, "", "", WireSearching},
		{"broadcasting", StatusSearching, false, "broadcasting", "", WireSearching},
		{"no provider found", StatusUnmatched, false, "no_provider_found", "", WireUnmatched},
		{"unmatched with cancelled_at set", StatusUnmatched, true, "no_provider_found", "", WireUnmatched},
		{"broadcast expired only", StatusSearching, false, "expired", "", WireUnmatched},
		{"broadcast cancelled", StatusCancelled, true, "cancelled", "", WireCancelled},
		{"booking cancelled before broadcast", StatusCancelled, true, "", "", WireCancelled},
		{"accepted no trip yet", StatusSearching, false, "accepted", "", WireAccepted},
		{"trip assigned", StatusAccepted, false, "accepted", "assigned", WireAccepted},
		{"en route pickup", StatusAccepted, false, "accepted", "en_route_pickup", WireEnRoutePickup},
		{"arrived pickup", StatusAccepted, false, "accepted", "arrived_pickup", WireArrivedAtPickup},
		{"in progress", StatusAccepted, false, "accepted", "in_progress", WireInTransit},
		{"proof submitted", StatusAccepted, false, "accepted", "proof_submitted", WireDelivered},
		{"completed", StatusCompleted, false, "accepted", "completed", WireCompleted},
		{"trip cancelled", StatusCancelled, false, "accepted", "cancelled", WireCancelled},
		{"trip failed", StatusCancelled, false, "accepted", "failed", WireCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := DerivedState{
				Booking:         Booking{Status: tc.bookingStatus},
				BroadcastStatus: tc.broadcastStatus,
				TripStatus:      tc.tripStatus,
			}
			if tc.cancelled {
				now := time.Now()
				state.Booking.CancelledAt = &now
			}
			if got := state.WireStatus(); got != tc.want {
				t.Fatalf("WireStatus() = %s, want %s", got, tc.want)
			}
		})
	}
}

// --- create ---

func TestCreateBookingHappyPath(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	payments := &fakePayments{}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, payments)

	view, err := service.CreateBooking(context.Background(), testCustomer, validCreateInput())
	if err != nil {
		t.Fatalf("CreateBooking: %v", err)
	}
	if view.Status != WireSearching {
		t.Fatalf("status = %s, want searching", view.Status)
	}
	if view.FareAmount < 100000 {
		t.Fatalf("fare = %d, want >= min fare", view.FareAmount)
	}
	if len(payments.holds) != 1 || payments.holds[0] != view.ID {
		t.Fatalf("expected one wallet hold for %s, got %v", view.ID, payments.holds)
	}
	if len(broadcast.started) != 1 || broadcast.started[0] != view.ID {
		t.Fatalf("expected broadcast started for %s, got %v", view.ID, broadcast.started)
	}
	if repo.bookings[view.ID].PaymentStatus != PaymentHeld {
		t.Fatalf("payment status = %s, want held", repo.bookings[view.ID].PaymentStatus)
	}
}

func TestProtectedCreateBindsQuoteAndReplaysOneBooking(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	payments := &fakePayments{}
	now := time.Now().UTC()
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, payments).
		WithClock(func() time.Time { return now }).
		WithServiceabilityQuoteAuthority(fakeQuoteAuthority{quote: serviceability.Quote{
			ID: "dsq-protected", RequestFingerprint: "quote-fingerprint", Version: 3,
			FareEstimateKobo: 245000, Route: serviceability.RouteResult{DistanceMeters: 12000},
		}}, true)
	input := validCreateInput()
	input.PackageType = "parcel"
	input.PackageWeight = 1.5
	input.ServiceabilityQuoteRef = "dsq-protected"

	first, err := service.CreateBookingWithKey(context.Background(), testCustomer, "create-1", input)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	second, err := service.CreateBookingWithKey(context.Background(), testCustomer, "create-1", input)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if first.ID != second.ID || len(repo.bookings) != 1 {
		t.Fatalf("first=%s second=%s bookings=%d", first.ID, second.ID, len(repo.bookings))
	}
	if len(payments.holds) != 1 {
		t.Fatalf("wallet holds=%d, want 1", len(payments.holds))
	}
	if first.FareAmount != 245000 || first.ServiceabilityQuoteRef != "dsq-protected" || first.ServiceabilityQuoteVersion != 3 {
		t.Fatalf("protected snapshot=%+v", first.Booking)
	}

	changed := input
	changed.Dropoff.Address = "Different address"
	if _, err := service.CreateBookingWithKey(context.Background(), testCustomer, "create-1", changed); err == nil {
		t.Fatal("changed payload must conflict")
	}
}

func TestProtectedCreateRequiresQuoteAndKey(t *testing.T) {
	service := newTestService(newFakeRepo(), &fakeBroadcast{}, &fakeTrips{}, &fakeLive{}, &fakePayments{}).
		WithServiceabilityQuoteAuthority(fakeQuoteAuthority{}, true)
	if _, err := service.CreateBookingWithKey(context.Background(), testCustomer, "", validCreateInput()); err == nil {
		t.Fatal("protected create without quote/key must fail")
	}
}

func TestCreateBookingPaymentFailureFailsFast(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	payments := &fakePayments{holdErr: errors.New("Wallet balance is not enough for this payment.")}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, payments)

	_, err := service.CreateBooking(context.Background(), testCustomer, validCreateInput())
	if err == nil {
		t.Fatal("expected payment error")
	}
	if !strings.Contains(err.Error(), "Wallet payment was not accepted") {
		t.Fatalf("expected safe payment error, got %v", err)
	}
	if len(broadcast.started) != 0 {
		t.Fatal("broadcast must not start when payment fails")
	}
	for _, booking := range repo.bookings {
		if booking.Status != StatusCancelled || booking.PaymentStatus != PaymentFailed {
			t.Fatalf("booking left in %s/%s, want cancelled/failed", booking.Status, booking.PaymentStatus)
		}
	}
}

func TestCreateBookingBroadcastFailureRefunds(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo, startErr: errors.New("nearby lookup down")}
	payments := &fakePayments{}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, payments)

	_, err := service.CreateBooking(context.Background(), testCustomer, validCreateInput())
	if err == nil {
		t.Fatal("expected broadcast error")
	}
	if len(payments.refunds) != 1 {
		t.Fatalf("expected exactly one refund, got %v", payments.refunds)
	}
	if !strings.HasPrefix(payments.refunds[0].idemKey, "refund-booking-") {
		t.Fatalf("refund key = %s, want refund-booking- prefix", payments.refunds[0].idemKey)
	}
	for _, booking := range repo.bookings {
		if booking.Status != StatusCancelled || booking.PaymentStatus != PaymentRefunded {
			t.Fatalf("booking left in %s/%s, want cancelled/refunded", booking.Status, booking.PaymentStatus)
		}
	}
}

func TestCreateBookingValidation(t *testing.T) {
	service := newTestService(newFakeRepo(), &fakeBroadcast{}, &fakeTrips{}, &fakeLive{}, &fakePayments{})
	cases := []struct {
		name   string
		mutate func(*CreateBookingInput)
	}{
		{"zero pickup coords", func(in *CreateBookingInput) { in.Pickup.Lat, in.Pickup.Lng = 0, 0 }},
		{"out of range dropoff", func(in *CreateBookingInput) { in.Dropoff.Lat = 120 }},
		{"missing receiver name", func(in *CreateBookingInput) { in.ReceiverName = " " }},
		{"missing receiver phone", func(in *CreateBookingInput) { in.ReceiverPhone = "" }},
		{"missing package desc", func(in *CreateBookingInput) { in.PackageDesc = "" }},
		{"card payment rejected", func(in *CreateBookingInput) { in.PaymentMethod = "card" }},
		{"negative weight", func(in *CreateBookingInput) { in.PackageWeight = -2 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := validCreateInput()
			tc.mutate(&input)
			if _, err := service.CreateBooking(context.Background(), testCustomer, input); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// TestCreateBookingReceiverPhoneNormalization is the regression guard for the
// bug where a locally-formatted receiver phone ("0801...") was persisted
// as-is and could never be matched later at proof-of-delivery time (which
// requires strict E.164). CreateBooking must normalize through the shared
// Nigerian-phone helper before the row is inserted.
func TestCreateBookingReceiverPhoneNormalization(t *testing.T) {
	cases := []struct {
		name    string
		phone   string
		want    string
		wantErr bool
	}{
		{name: "local 0-prefixed", phone: "0801 234 5678", want: "+2348012345678"},
		{name: "already E.164", phone: "+2348012345678", want: "+2348012345678"},
		{name: "234-prefixed no plus", phone: "2348012345678", want: "+2348012345678"},
		{name: "junk", phone: "not-a-phone", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepo()
			broadcast := &fakeBroadcast{repo: repo}
			service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, &fakePayments{})

			input := validCreateInput()
			input.ReceiverPhone = tc.phone
			view, err := service.CreateBooking(context.Background(), testCustomer, input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected validation error for malformed phone")
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateBooking: %v", err)
			}
			if got := repo.bookings[view.ID].ReceiverPhone; got != tc.want {
				t.Fatalf("persisted receiver_phone = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- cancel ---

func createSearchingBooking(t *testing.T, service *Service) BookingView {
	t.Helper()
	view, err := service.CreateBooking(context.Background(), testCustomer, validCreateInput())
	if err != nil {
		t.Fatalf("CreateBooking: %v", err)
	}
	return view
}

func TestCancelBeforeAcceptRefundsWithBookingKey(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	payments := &fakePayments{}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, payments)
	view := createSearchingBooking(t, service)

	result, err := service.CancelBooking(context.Background(), testCustomer, view.ID, "changed_mind")
	if err != nil {
		t.Fatalf("CancelBooking: %v", err)
	}
	if result.Status != WireCancelled {
		t.Fatalf("status = %s, want cancelled", result.Status)
	}
	if len(payments.refunds) != 1 || payments.refunds[0].idemKey != "refund-booking-"+view.ID {
		t.Fatalf("refunds = %v, want one refund-booking key", payments.refunds)
	}
	if len(broadcast.cancelled) != 1 {
		t.Fatalf("broadcast cancel calls = %v, want 1", broadcast.cancelled)
	}
}

func TestCancelAcceptRaceReturnsConflictWithoutRefund(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	payments := &fakePayments{}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, payments)
	view := createSearchingBooking(t, service)

	// A rider accepts exactly while the cancel is running.
	broadcast.acceptOnRead = func(bookingID string) {
		repo.mu.Lock()
		repo.broadcast[bookingID] = "accepted"
		repo.mu.Unlock()
	}

	_, err := service.CancelBooking(context.Background(), testCustomer, view.ID, "changed_mind")
	if err == nil {
		t.Fatal("expected conflict")
	}
	if appErr := apperrors.From(err); appErr == nil || appErr.Code != apperrors.CodeConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
	if len(payments.refunds) != 0 {
		t.Fatalf("no refund may fire in the accept race, got %v", payments.refunds)
	}
}

func TestCancelWithAssignedTripUsesTripKey(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	payments := &fakePayments{}
	trips := &fakeTrips{}
	service := newTestService(repo, broadcast, trips, &fakeLive{}, payments)
	view := createSearchingBooking(t, service)

	tripID := uuid.NewString()
	repo.mu.Lock()
	repo.broadcast[view.ID] = "accepted"
	repo.tripID[view.ID] = tripID
	repo.tripState[view.ID] = "assigned"
	repo.tripProv[view.ID] = "provider-1"
	repo.mu.Unlock()
	trips.afterCancel = func(bookingID string) {
		repo.mu.Lock()
		repo.tripState[bookingID] = "cancelled"
		repo.mu.Unlock()
	}

	result, err := service.CancelBooking(context.Background(), testCustomer, view.ID, "changed_mind")
	if err != nil {
		t.Fatalf("CancelBooking: %v", err)
	}
	if result.Status != WireCancelled {
		t.Fatalf("status = %s, want cancelled", result.Status)
	}
	if len(trips.cancelled) != 1 {
		t.Fatalf("trip cancel calls = %v, want 1", trips.cancelled)
	}
	if len(payments.refunds) != 1 || payments.refunds[0].idemKey != "refund-"+tripID {
		t.Fatalf("refunds = %v, want one refund-%s", payments.refunds, tripID)
	}
}

func TestCancelInTransitRejected(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	payments := &fakePayments{}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, payments)
	view := createSearchingBooking(t, service)

	repo.mu.Lock()
	repo.broadcast[view.ID] = "accepted"
	repo.tripID[view.ID] = uuid.NewString()
	repo.tripState[view.ID] = "in_progress"
	repo.mu.Unlock()

	_, err := service.CancelBooking(context.Background(), testCustomer, view.ID, "changed_mind")
	if appErr := apperrors.From(err); appErr == nil || appErr.Code != apperrors.CodeConflict {
		t.Fatalf("expected conflict for in_transit cancel, got %v", err)
	}
	if len(payments.refunds) != 0 {
		t.Fatalf("no refund may fire for in_transit cancel, got %v", payments.refunds)
	}
}

func TestCancelTripRaceToInProgressDoesNotRefund(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	payments := &fakePayments{}
	trips := &fakeTrips{}
	service := newTestService(repo, broadcast, trips, &fakeLive{}, payments)
	view := createSearchingBooking(t, service)

	repo.mu.Lock()
	repo.broadcast[view.ID] = "accepted"
	repo.tripID[view.ID] = uuid.NewString()
	repo.tripState[view.ID] = "arrived_pickup"
	repo.mu.Unlock()
	// The rider starts the trip while the cancel is in flight: the trip-side
	// guard no-ops and the trip stays in_progress.
	trips.afterCancel = func(bookingID string) {
		repo.mu.Lock()
		repo.tripState[bookingID] = "in_progress"
		repo.mu.Unlock()
	}

	_, err := service.CancelBooking(context.Background(), testCustomer, view.ID, "changed_mind")
	if appErr := apperrors.From(err); appErr == nil || appErr.Code != apperrors.CodeConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
	if len(payments.refunds) != 0 {
		t.Fatalf("no refund may fire when the trip kept going, got %v", payments.refunds)
	}
}

func TestCancelByNonOwnerNotFound(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, &fakePayments{})
	view := createSearchingBooking(t, service)

	_, err := service.CancelBooking(context.Background(), uuid.NewString(), view.ID, "changed_mind")
	if appErr := apperrors.From(err); appErr == nil || appErr.Code != apperrors.CodeNotFound {
		t.Fatalf("expected not found for foreign customer, got %v", err)
	}
}

// --- subscribers ---

func TestHandleRequestAcceptedAssignsProvider(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	payments := &fakePayments{}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, payments)
	view := createSearchingBooking(t, service)

	if err := service.HandleRequestAccepted(context.Background(), view.ID, "provider-9"); err != nil {
		t.Fatalf("HandleRequestAccepted: %v", err)
	}
	if repo.bookings[view.ID].Status != StatusAccepted || repo.bookings[view.ID].ProviderID != "provider-9" {
		t.Fatalf("booking not accepted: %+v", repo.bookings[view.ID])
	}
	if len(payments.assigns) != 1 || payments.assigns[0] != "pi_"+view.ID+":provider-9" {
		t.Fatalf("assigns = %v", payments.assigns)
	}
	// A second (duplicate) event is a no-op.
	if err := service.HandleRequestAccepted(context.Background(), view.ID, "provider-10"); err != nil {
		t.Fatalf("duplicate HandleRequestAccepted: %v", err)
	}
	if repo.bookings[view.ID].ProviderID != "provider-9" {
		t.Fatal("duplicate accept overwrote the provider")
	}
}

func TestHandleNoProviderFoundRefundsOnce(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	payments := &fakePayments{}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, payments)
	view := createSearchingBooking(t, service)

	if err := service.HandleNoProviderFound(context.Background(), view.ID); err != nil {
		t.Fatalf("HandleNoProviderFound: %v", err)
	}
	if repo.bookings[view.ID].Status != StatusUnmatched {
		t.Fatalf("status = %s, want unmatched", repo.bookings[view.ID].Status)
	}
	if repo.bookings[view.ID].PaymentStatus != PaymentRefunded {
		t.Fatalf("payment = %s, want refunded", repo.bookings[view.ID].PaymentStatus)
	}
	// Duplicate event must not refund twice.
	if err := service.HandleNoProviderFound(context.Background(), view.ID); err != nil {
		t.Fatalf("duplicate HandleNoProviderFound: %v", err)
	}
	if len(payments.refunds) != 1 {
		t.Fatalf("refunds = %v, want exactly 1", payments.refunds)
	}
}

func TestHandleNoProviderFoundRetriesUnknownWalletOutcomeWithSameKey(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	payments := &fakePayments{refundErr: errors.New("timeout")}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, payments)
	view := createSearchingBooking(t, service)
	if err := service.HandleNoProviderFound(context.Background(), view.ID); err != nil {
		t.Fatal(err)
	}
	if repo.bookings[view.ID].Status != StatusUnmatched || repo.bookings[view.ID].PaymentStatus != PaymentHeld {
		t.Fatalf("booking=%+v", repo.bookings[view.ID])
	}
	payments.refundErr = nil
	if err := service.HandleNoProviderFound(context.Background(), view.ID); err != nil {
		t.Fatal(err)
	}
	if len(payments.refunds) != 1 || payments.refunds[0].idemKey != "refund-booking-"+view.ID {
		t.Fatalf("refunds=%+v", payments.refunds)
	}
	if repo.bookings[view.ID].PaymentStatus != PaymentRefunded {
		t.Fatalf("payment=%s", repo.bookings[view.ID].PaymentStatus)
	}
}

func TestHandleNoProviderFoundNeverClobbersAcceptedBooking(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	payments := &fakePayments{}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, payments)
	view := createSearchingBooking(t, service)

	// A rider accepts, then a stale no_provider_found event arrives.
	if err := service.HandleRequestAccepted(context.Background(), view.ID, "provider-1"); err != nil {
		t.Fatalf("HandleRequestAccepted: %v", err)
	}
	if err := service.HandleNoProviderFound(context.Background(), view.ID); err != nil {
		t.Fatalf("HandleNoProviderFound: %v", err)
	}
	if repo.bookings[view.ID].Status != StatusAccepted {
		t.Fatalf("status = %s, accepted booking must not be clobbered", repo.bookings[view.ID].Status)
	}
	if len(payments.refunds) != 0 {
		t.Fatalf("no refund may fire for an accepted booking, got %v", payments.refunds)
	}
}

func TestHandleTripCompletedMarksPaid(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, &fakePayments{})
	view := createSearchingBooking(t, service)

	completedAt := time.Now().UTC()
	if err := service.HandleTripCompleted(context.Background(), view.ID, completedAt); err != nil {
		t.Fatalf("HandleTripCompleted: %v", err)
	}
	booking := repo.bookings[view.ID]
	if booking.Status != StatusCompleted || booking.PaymentStatus != PaymentPaid {
		t.Fatalf("booking = %s/%s, want completed/paid", booking.Status, booking.PaymentStatus)
	}
}

// --- review ---

func TestSubmitReviewFlow(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, &fakePayments{})
	view := createSearchingBooking(t, service)

	// Not delivered yet.
	if _, err := service.SubmitReview(context.Background(), testCustomer, view.ID, ReviewInput{Rating: 5}); err == nil {
		t.Fatal("expected conflict before delivery")
	}

	repo.mu.Lock()
	repo.broadcast[view.ID] = "accepted"
	repo.tripID[view.ID] = uuid.NewString()
	repo.tripState[view.ID] = "proof_submitted"
	repo.tripProv[view.ID] = "provider-1"
	repo.mu.Unlock()

	if _, err := service.SubmitReview(context.Background(), testCustomer, view.ID, ReviewInput{Rating: 0}); err == nil {
		t.Fatal("expected rating validation error")
	}
	recommends := true
	review, err := service.SubmitReview(context.Background(), testCustomer, view.ID, ReviewInput{Rating: 5, ReviewText: "Great rider", RecommendsProvider: &recommends})
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if review.ProviderID != "provider-1" || review.Rating != 5 {
		t.Fatalf("review = %+v", review)
	}
	// Duplicate review rejected.
	if _, err := service.SubmitReview(context.Background(), testCustomer, view.ID, ReviewInput{Rating: 4}); err == nil {
		t.Fatal("expected duplicate review conflict")
	}
	// GetReview round-trips.
	fetched, err := service.GetReview(context.Background(), testCustomer, view.ID)
	if err != nil || fetched.Rating != 5 {
		t.Fatalf("GetReview = %+v, %v", fetched, err)
	}
}

// TestSubmitReviewSettlesDeliveredBooking is the regression guard for the bug
// where dispatch had no path from 'delivered' back to 'completed' other than
// the rider tapping Complete — a review on a booking whose escrow was still
// held left the fare stuck forever even though the customer had confirmed
// delivery. SubmitReview must now settle the booking itself when it finds the
// booking still delivered (mirrors hauling's review-driven completion).
func TestSubmitReviewSettlesDeliveredBooking(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, &fakePayments{})
	view := createSearchingBooking(t, service)

	repo.mu.Lock()
	repo.broadcast[view.ID] = "accepted"
	repo.tripID[view.ID] = uuid.NewString()
	repo.tripState[view.ID] = "proof_submitted" // WireDelivered
	repo.tripProv[view.ID] = "provider-1"
	repo.mu.Unlock()

	if repo.bookings[view.ID].PaymentStatus != PaymentHeld {
		t.Fatalf("precondition: expected escrow held before review, got %s", repo.bookings[view.ID].PaymentStatus)
	}

	if _, err := service.SubmitReview(context.Background(), testCustomer, view.ID, ReviewInput{Rating: 5}); err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}

	booking := repo.bookings[view.ID]
	if booking.Status != StatusCompleted {
		t.Fatalf("booking status = %s, want completed", booking.Status)
	}
	if booking.PaymentStatus != PaymentPaid {
		t.Fatalf("payment status = %s, want paid", booking.PaymentStatus)
	}
	if booking.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
}

// TestSubmitReviewOnAlreadyCompletedBookingStillSavesReview covers the review
// path when the rider already completed the trip before the customer got to
// reviewing it: the review must still save, and the redundant completion
// attempt must be a safe no-op rather than an error surfaced to the customer.
func TestSubmitReviewOnAlreadyCompletedBookingStillSavesReview(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	service := newTestService(repo, broadcast, &fakeTrips{}, &fakeLive{}, &fakePayments{})
	view := createSearchingBooking(t, service)

	repo.mu.Lock()
	repo.broadcast[view.ID] = "accepted"
	repo.tripID[view.ID] = uuid.NewString()
	repo.tripState[view.ID] = "completed" // WireCompleted — rider already completed it
	repo.tripProv[view.ID] = "provider-1"
	repo.bookings[view.ID].Status = StatusCompleted
	repo.bookings[view.ID].PaymentStatus = PaymentPaid
	repo.mu.Unlock()

	review, err := service.SubmitReview(context.Background(), testCustomer, view.ID, ReviewInput{Rating: 4})
	if err != nil {
		t.Fatalf("SubmitReview on already-completed booking: %v", err)
	}
	if review.Rating != 4 {
		t.Fatalf("review = %+v", review)
	}
	if repo.bookings[view.ID].PaymentStatus != PaymentPaid {
		t.Fatalf("payment status changed unexpectedly: %s", repo.bookings[view.ID].PaymentStatus)
	}
}

// --- availability + location ---

func TestCheckAvailability(t *testing.T) {
	live := &fakeLive{nearby: []availability.NearbyProvider{{ProviderID: "p1"}, {ProviderID: "p2"}}}
	service := newTestService(newFakeRepo(), &fakeBroadcast{}, &fakeTrips{}, live, &fakePayments{})
	result, err := service.CheckAvailability(context.Background(), 6.5, 3.4)
	if err != nil {
		t.Fatalf("CheckAvailability: %v", err)
	}
	if !result.Available || result.Count != 2 {
		t.Fatalf("result = %+v", result)
	}
	if _, err := service.CheckAvailability(context.Background(), 0, 0); err == nil {
		t.Fatal("expected coord validation error")
	}
}

func TestGetLiveLocation(t *testing.T) {
	repo := newFakeRepo()
	broadcast := &fakeBroadcast{repo: repo}
	live := &fakeLive{hasLoc: true, location: availability.Location{Lat: 6.51, Lng: 3.38, UpdatedAt: time.Now()}}
	service := newTestService(repo, broadcast, &fakeTrips{}, live, &fakePayments{})
	view := createSearchingBooking(t, service)

	// No rider yet.
	if _, err := service.GetLiveLocation(context.Background(), testCustomer, view.ID); err == nil {
		t.Fatal("expected not found before assignment")
	}
	repo.mu.Lock()
	repo.tripProv[view.ID] = "provider-1"
	repo.tripID[view.ID] = uuid.NewString()
	repo.tripState[view.ID] = "en_route_pickup"
	repo.mu.Unlock()
	location, err := service.GetLiveLocation(context.Background(), testCustomer, view.ID)
	if err != nil {
		t.Fatalf("GetLiveLocation: %v", err)
	}
	if location.ProviderID != "provider-1" || location.Lat != 6.51 {
		t.Fatalf("location = %+v", location)
	}
}
