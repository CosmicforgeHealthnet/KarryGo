package bookingusecases

import (
	"context"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	bookingclients "cosmicforge/logistics/services/hauling-service/internal/features/booking/clients"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	bookingrepo "cosmicforge/logistics/services/hauling-service/internal/features/booking/repositories"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	autoCompleteGracePeriod   = 30 * time.Minute
	autoCompleteCheckInterval = 5 * time.Minute
)

// TruckInfo is the minimal truck data the matcher needs to decide eligibility.
type TruckInfo struct {
	TruckType  string
	CapacityKg int
	Status     string
}

// TruckLookup resolves a truck by id during matching, without the booking
// usecase depending on the whole provider_profile repository. Implemented by an
// adapter over the truck repository in cmd wiring.
type TruckLookup interface {
	GetTruck(ctx context.Context, truckID string) (TruckInfo, error)
}

// PaymentClient binds a booking's fare to a payment-wallet-service payment
// intent. Implemented by an adapter over shared/go/walletclient in cmd wiring;
// nil in local dev (payment becomes a no-op and bookings settle as before).
type PaymentClient interface {
	// HoldFromWallet authorizes/holds the fare against the customer wallet for an
	// existing booking-keyed intent (creating it if needed). Returns the intent id.
	HoldFromWallet(ctx context.Context, in PaymentHoldInput) (string, error)
	// CreateCardIntent creates a Paystack payment intent keyed to the booking and
	// returns the intent id and the authorization URL for the checkout WebView.
	CreateCardIntent(ctx context.Context, in PaymentHoldInput, customerEmail string) (intentID, authorizationURL string, err error)
	// Settle releases the held funds to the provider on completion.
	Settle(ctx context.Context, bookingID string) error
	// Refund reverses a charge on cancel/unmatched.
	Refund(ctx context.Context, paymentIntentID string, amountKobo int64, reason string) error
}

type PaymentHoldInput struct {
	BookingID  string
	CustomerID string
	ProviderID string
	AmountKobo int64
}

type BookingService struct {
	bookings      bookingrepo.BookingRepository
	availability  availabilityrepositories.AvailabilityStore
	trucks        TruckLookup
	payments      PaymentClient
	notifier      *bookingclients.BookingNotifier
	matchTimeout  time.Duration // per-provider acceptance window
	searchWindow  time.Duration // total time to keep searching before unmatched
	maxRadiusKm   float64
	activeMatches sync.Map // map[bookingID]*matchHandle
}

// matchHandle wraps a context.CancelFunc so it can be stored in sync.Map and
// compared by pointer identity. context.CancelFunc is uncomparable, so storing
// it directly panics on CompareAndDelete.
type matchHandle struct {
	cancel context.CancelFunc
}

type Options struct {
	Bookings            bookingrepo.BookingRepository
	Availability        availabilityrepositories.AvailabilityStore
	Trucks              TruckLookup
	Payments            PaymentClient // nil disables payment binding (local dev)
	Notifier            *bookingclients.BookingNotifier
	MatchTimeoutSeconds int
	SearchWindowSeconds int
	MaxRadiusKm         float64
}

func NewBookingService(opts Options) *BookingService {
	radius := opts.MaxRadiusKm
	if radius <= 0 {
		radius = 25
	}
	matchTimeout := time.Duration(opts.MatchTimeoutSeconds) * time.Second
	searchWindow := time.Duration(opts.SearchWindowSeconds) * time.Second
	// The search window must be at least one acceptance window long; default to
	// 60s so a provider who comes online mid-search can still be matched.
	if searchWindow < matchTimeout {
		searchWindow = 60 * time.Second
	}
	if searchWindow < matchTimeout {
		searchWindow = matchTimeout
	}
	return &BookingService{
		bookings:     opts.Bookings,
		availability: opts.Availability,
		trucks:       opts.Trucks,
		payments:     opts.Payments,
		notifier:     opts.Notifier,
		matchTimeout: matchTimeout,
		searchWindow: searchWindow,
		maxRadiusKm:  radius,
	}
}

// ─── Fare Estimation ──────────────────────────────────────────────────────────

type EstimateFareInput struct {
	PickupLat     float64
	PickupLng     float64
	DropoffLat    float64
	DropoffLng    float64
	CargoWeightKg int
	HelperCount   int
}

func (s *BookingService) EstimateFare(input EstimateFareInput) bookingmodels.FareEstimate {
	dist := haversineKm(input.PickupLat, input.PickupLng, input.DropoffLat, input.DropoffLng)
	return bookingmodels.CalculateFare(dist, input.CargoWeightKg, input.HelperCount)
}

// ─── Create Booking ───────────────────────────────────────────────────────────

type CreateBookingInput struct {
	CustomerID         string
	PickupAddress      string
	PickupLat          float64
	PickupLng          float64
	DropoffAddress     string
	DropoffLat         float64
	DropoffLng         float64
	PreferredTruckType string
	CargoWeightKg      int
	CargoDescription   string
	RequiresHelpers    bool
	HelperCount        int
	WeightCategory     string
	ReceiverName       string
	ReceiverPhone      string
	PackageContent     string
	PackageSize        string
	IsFragile          bool
	PaymentMethod      string
	ScheduledAt        *time.Time
}

func (s *BookingService) CreateBooking(ctx context.Context, input CreateBookingInput) (bookingmodels.PublicBooking, error) {
	var fields []apperrors.FieldViolation
	if strings.TrimSpace(input.PickupAddress) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "pickup_address", Message: "Pickup address is required."})
	}
	if strings.TrimSpace(input.DropoffAddress) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "dropoff_address", Message: "Dropoff address is required."})
	}
	if !validCoordinate(input.PickupLat, input.PickupLng) {
		fields = append(fields, apperrors.FieldViolation{Field: "pickup_lat", Message: "Pickup location is missing or invalid. Pick it from the suggestions."})
	}
	if !validCoordinate(input.DropoffLat, input.DropoffLng) {
		fields = append(fields, apperrors.FieldViolation{Field: "dropoff_lat", Message: "Dropoff location is missing or invalid. Pick it from the suggestions."})
	}
	if input.CargoWeightKg <= 0 {
		fields = append(fields, apperrors.FieldViolation{Field: "cargo_weight_kg", Message: "Cargo weight must be greater than zero."})
	}
	paymentMethod := strings.TrimSpace(input.PaymentMethod)
	if paymentMethod == "" {
		paymentMethod = bookingmodels.PaymentMethodWallet
	}
	if !bookingmodels.ValidPaymentMethods[paymentMethod] {
		fields = append(fields, apperrors.FieldViolation{Field: "payment_method", Message: "Choose a valid payment method."})
	}
	if len(fields) > 0 {
		return bookingmodels.PublicBooking{}, apperrors.Validation("Please check your booking details.", fields)
	}

	// Gate on the live provider list (which prunes stale entries), not a raw
	// SCARD, so we don't accept a booking off expired online keys.
	online, err := s.availability.GetOnlineProviders(ctx)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if len(online) == 0 {
		return bookingmodels.PublicBooking{}, apperrors.Unavailable("No truck providers are currently available. Please try again shortly.", nil)
	}

	dist := haversineKm(input.PickupLat, input.PickupLng, input.DropoffLat, input.DropoffLng)
	estimate := bookingmodels.CalculateFare(dist, input.CargoWeightKg, input.HelperCount)
	distFloat := estimate.DistanceKm
	fareKobo := estimate.FareEstimateKobo

	booking := bookingmodels.Booking{
		ID:                 uuid.NewString(),
		CustomerID:         input.CustomerID,
		PickupAddress:      strings.TrimSpace(input.PickupAddress),
		PickupLat:          input.PickupLat,
		PickupLng:          input.PickupLng,
		DropoffAddress:     strings.TrimSpace(input.DropoffAddress),
		DropoffLat:         input.DropoffLat,
		DropoffLng:         input.DropoffLng,
		CargoType:          "",
		PreferredTruckType: strings.TrimSpace(input.PreferredTruckType),
		CargoWeightKg:      input.CargoWeightKg,
		CargoDescription:   strings.TrimSpace(input.CargoDescription),
		RequiresHelpers:    input.RequiresHelpers,
		HelperCount:        input.HelperCount,
		WeightCategory:     strings.TrimSpace(input.WeightCategory),
		ReceiverName:       strings.TrimSpace(input.ReceiverName),
		ReceiverPhone:      strings.TrimSpace(input.ReceiverPhone),
		PackageContent:     strings.TrimSpace(input.PackageContent),
		PackageSize:        strings.TrimSpace(input.PackageSize),
		IsFragile:          input.IsFragile,
		DistanceKm:         &distFloat,
		FareEstimateKobo:   &fareKobo,
		PaymentMethod:      paymentMethod,
		PaymentStatus:      bookingmodels.PaymentStatusUnpaid,
		Status:             bookingmodels.StatusPendingMatch,
		ScheduledAt:        input.ScheduledAt,
	}

	created, err := s.bookings.Create(ctx, booking)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}

	_ = s.addEvent(ctx, created.ID, "booking_created", bookingmodels.ActorCustomer, input.CustomerID)
	// Card bookings must be paid up-front before a provider can accept (see
	// ensurePaymentSecured). Don't dispatch yet — otherwise a provider could be
	// matched and tap accept before the customer finishes Paystack checkout, only
	// to be rejected. Matching starts from InitiateCardPayment once the customer
	// has committed to the checkout. Wallet/cash dispatch immediately.
	if paymentMethod != bookingmodels.PaymentMethodCard {
		s.startMatchGoroutine(created.ID, input.PickupLat, input.PickupLng)
	}
	return created.Public(), nil
}

// ProviderLocation is the live position of the provider assigned to a booking.
type ProviderLocation struct {
	BookingID string  `json:"booking_id"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	UpdatedAt int64   `json:"updated_at"`
	Available bool    `json:"available"`
}

// GetBookingLocation returns the live location of the provider assigned to the
// customer's booking, sourced from the provider's availability heartbeat. Used
// to drive the live driver marker on the customer trip map.
func (s *BookingService) GetBookingLocation(ctx context.Context, bookingID, customerID string) (ProviderLocation, error) {
	b, err := s.bookings.GetByIDForCustomer(ctx, bookingID, customerID)
	if err != nil {
		return ProviderLocation{}, err
	}
	if b.ProviderID == nil {
		return ProviderLocation{BookingID: bookingID, Available: false}, nil
	}
	st, ok, err := s.availability.GetProviderStatus(ctx, *b.ProviderID)
	if err != nil {
		return ProviderLocation{}, err
	}
	if !ok {
		return ProviderLocation{BookingID: bookingID, Available: false}, nil
	}
	return ProviderLocation{
		BookingID: bookingID,
		Lat:       st.Lat,
		Lng:       st.Lng,
		UpdatedAt: st.UpdatedAt,
		Available: true,
	}, nil
}

type InitiateCardPaymentInput struct {
	BookingID     string
	CustomerID    string
	CustomerEmail string
}

type CardPaymentResult struct {
	AuthorizationURL string `json:"authorization_url"`
	PaymentIntentID  string `json:"payment_intent_id"`
}

// InitiateCardPayment creates the up-front Paystack intent for a card booking and
// returns the authorization URL for the checkout WebView. The customer completes
// payment while the booking is searching; refunded if it ends unmatched/cancelled.
func (s *BookingService) InitiateCardPayment(ctx context.Context, input InitiateCardPaymentInput) (CardPaymentResult, error) {
	b, err := s.bookings.GetByIDForCustomer(ctx, input.BookingID, input.CustomerID)
	if err != nil {
		return CardPaymentResult{}, err
	}
	if b.PaymentMethod != bookingmodels.PaymentMethodCard {
		return CardPaymentResult{}, apperrors.BadRequest("This booking is not a card payment.", nil)
	}
	if strings.TrimSpace(input.CustomerEmail) == "" {
		return CardPaymentResult{}, apperrors.Validation("A valid email is required to pay by card.", []apperrors.FieldViolation{
			{Field: "customer_email", Message: "Email is required for card payment."},
		})
	}
	if s.payments == nil {
		return CardPaymentResult{}, apperrors.Unavailable("Card payment is not available right now.", nil)
	}
	providerID := ""
	if b.ProviderID != nil {
		providerID = *b.ProviderID
	}
	intentID, authURL, err := s.payments.CreateCardIntent(ctx, PaymentHoldInput{
		BookingID:  b.ID,
		CustomerID: b.CustomerID,
		ProviderID: providerID,
		AmountKobo: fareToCharge(b),
	}, strings.TrimSpace(input.CustomerEmail))
	if err != nil {
		return CardPaymentResult{}, apperrors.Unavailable("Card payment could not be started. Please try again.", err)
	}
	_, _ = s.bookings.SetPayment(ctx, b.ID, bookingmodels.PaymentStatusUnpaid, intentID)
	// The customer has committed to the checkout, so begin matching now. By the
	// time a provider accepts, the up-front Paystack charge should be settled
	// (held/paid); if not, ensurePaymentSecured blocks the accept and the matcher
	// retries. Only dispatch once — a re-initiated payment shouldn't double-match.
	if b.Status == bookingmodels.StatusPendingMatch {
		s.startMatchGoroutine(b.ID, b.PickupLat, b.PickupLng)
	}
	return CardPaymentResult{AuthorizationURL: authURL, PaymentIntentID: intentID}, nil
}

// ─── Matching Engine ──────────────────────────────────────────────────────────

// startMatchGoroutine cancels any existing match goroutine for the booking
// then starts a fresh one with a new cancellable context.
func (s *BookingService) startMatchGoroutine(bookingID string, pickupLat, pickupLng float64) {
	if old, ok := s.activeMatches.LoadAndDelete(bookingID); ok {
		old.(*matchHandle).cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	handle := &matchHandle{cancel: cancel}
	s.activeMatches.Store(bookingID, handle)
	go func() {
		defer s.activeMatches.CompareAndDelete(bookingID, handle)
		s.matchBooking(ctx, bookingID, pickupLat, pickupLng)
	}()
}

// cancelMatch stops the active matching goroutine for a booking, if any.
func (s *BookingService) cancelMatch(bookingID string) {
	if handle, ok := s.activeMatches.LoadAndDelete(bookingID); ok {
		handle.(*matchHandle).cancel()
	}
}

func (s *BookingService) matchBooking(ctx context.Context, bookingID string, pickupLat, pickupLng float64) {
	// cleanupCtx is used for operations that must complete even after ctx is cancelled
	// (releasing locks, recording events). We don't want those to fail because the
	// parent context was already done.
	cleanupCtx := context.Background()

	// Load the booking's requirements once so we can filter providers by truck
	// type and capacity, not just proximity.
	booking, err := s.bookings.GetByID(ctx, bookingID)
	if err != nil {
		return
	}

	// Keep searching until the window closes. When no truck is eligible right
	// now we wait and re-scan rather than giving up — a provider may toggle
	// online (or move into range) during the search. We only mark the booking
	// unmatched once the deadline passes with no acceptance.
	deadline := time.Now().Add(s.searchWindow)
	const rescanInterval = 3 * time.Second

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return
		}

		providers, err := s.availability.GetOnlineProviders(ctx)
		if err != nil {
			return
		}
		// Eligibility: within the serviceable radius and with a truck that can
		// carry the cargo and matches the requested type. A provider being
		// "online" is not enough — we never dispatch a truck that can't do the job.
		eligible := s.eligibleProviders(ctx, providers, booking, pickupLat, pickupLng)
		if len(eligible) == 0 {
			// Nobody eligible yet — wait briefly and re-scan, without overshooting
			// the deadline (unless cancelled).
			wait := rescanInterval
			if remaining := time.Until(deadline); remaining < wait {
				wait = remaining
			}
			if wait <= 0 {
				break
			}
			select {
			case <-time.After(wait):
				continue
			case <-ctx.Done():
				return
			}
		}

		accepted, done := s.tryEligibleProviders(ctx, cleanupCtx, bookingID, eligible)
		if done {
			// Booking was accepted or reached a terminal state; nothing more to do.
			return
		}
		if accepted {
			return
		}
		// All current candidates passed without accepting; loop re-scans for any
		// providers that came online while we were waiting, until the deadline.
	}

	if ctx.Err() == nil {
		s.markUnmatched(cleanupCtx, bookingID, booking.CustomerID)
	}
}

// tryEligibleProviders dispatches the booking to each eligible provider in
// order, waiting matchTimeout for each to accept. Returns done=true when the
// booking reached a terminal/accepted state (caller should stop entirely);
// accepted=true when a provider accepted. Returns (false,false) when every
// candidate timed out without accepting, so the caller can re-scan.
func (s *BookingService) tryEligibleProviders(
	ctx, cleanupCtx context.Context,
	bookingID string,
	eligible []availabilityrepositories.ProviderStatus,
) (accepted bool, done bool) {
	for _, p := range eligible {
		// Check cancellation before attempting to lock a new provider.
		select {
		case <-ctx.Done():
			return false, true
		default:
		}

		// Lock the provider for longer than the acceptance window so the lock
		// cannot expire mid-decision (and let another booking grab them while we
		// are still waiting). We release it explicitly once we have decided.
		lockTTL := s.matchTimeout + 15*time.Second
		acquired, err := s.availability.AcquireMatchLock(ctx, p.ProviderID, bookingID, lockTTL)
		if err != nil || !acquired {
			continue
		}

		// Guarded compare-and-set: only matches from pending_match. If this fails
		// the booking was already claimed/cancelled — stop and free the lock.
		if _, err := s.bookings.MarkMatched(ctx, bookingID, p.ProviderID, p.TruckID); err != nil {
			_ = s.availability.ReleaseMatchLock(cleanupCtx, p.ProviderID)
			return false, true
		}
		_ = s.addEvent(cleanupCtx, bookingID, "provider_matched", bookingmodels.ActorSystem, p.ProviderID)
		s.notifier.NotifyProviderMatched(cleanupCtx, p.ProviderID, bookingID, map[string]interface{}{"booking_id": bookingID})

		// Wait for the provider to accept, or exit early on cancellation.
		select {
		case <-time.After(s.matchTimeout):
		case <-ctx.Done():
			_ = s.availability.ReleaseMatchLock(cleanupCtx, p.ProviderID)
			return false, true
		}

		// Re-read status BEFORE releasing the lock, so a provider that accepts at
		// the boundary cannot have their lock released and be re-matched by a
		// concurrent booking on top of an accepted trip.
		current, err := s.bookings.GetByID(cleanupCtx, bookingID)
		if err != nil {
			_ = s.availability.ReleaseMatchLock(cleanupCtx, p.ProviderID)
			return false, true
		}
		if current.Status == bookingmodels.StatusAccepted ||
			current.Status == bookingmodels.StatusCancelled ||
			current.Status == bookingmodels.StatusCompleted {
			// Provider accepted (keep their lock — they own the trip now) or the
			// booking is terminal. Either way we are done; only release the lock
			// when the booking did not get accepted.
			if current.Status != bookingmodels.StatusAccepted {
				_ = s.availability.ReleaseMatchLock(cleanupCtx, p.ProviderID)
			}
			return current.Status == bookingmodels.StatusAccepted, true
		}

		// Provider did not accept — free them, reset, and try the next closest.
		_ = s.availability.ReleaseMatchLock(cleanupCtx, p.ProviderID)
		_, _ = s.bookings.ResetToMatching(cleanupCtx, bookingID)
		_ = s.addEvent(cleanupCtx, bookingID, "match_timeout", bookingmodels.ActorSystem, p.ProviderID)
	}
	return false, false
}

// eligibleProviders filters online providers down to those that can actually do
// the job — within the serviceable radius, with an active truck of a type and
// capacity that fits the cargo — and returns them sorted nearest-first.
func (s *BookingService) eligibleProviders(
	ctx context.Context,
	providers []availabilityrepositories.ProviderStatus,
	booking bookingmodels.Booking,
	pickupLat, pickupLng float64,
) []availabilityrepositories.ProviderStatus {
	wantType := strings.ToLower(strings.TrimSpace(booking.PreferredTruckType))

	type scored struct {
		p    availabilityrepositories.ProviderStatus
		dist float64
	}
	var matches []scored
	for _, p := range providers {
		dist := haversineKm(pickupLat, pickupLng, p.Lat, p.Lng)
		if dist > s.maxRadiusKm {
			continue
		}
		// Truck eligibility. When we can't resolve the truck (no lookup wired, or
		// the truck record is gone) we fall back to proximity-only so matching
		// degrades gracefully rather than rejecting everyone.
		if s.trucks != nil && p.TruckID != "" {
			truck, err := s.trucks.GetTruck(ctx, p.TruckID)
			if err == nil {
				if truck.CapacityKg > 0 && booking.CargoWeightKg > truck.CapacityKg {
					continue
				}
				if wantType != "" && strings.ToLower(strings.TrimSpace(truck.TruckType)) != wantType {
					continue
				}
			}
		}
		matches = append(matches, scored{p: p, dist: dist})
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].dist < matches[j].dist })

	result := make([]availabilityrepositories.ProviderStatus, len(matches))
	for i, m := range matches {
		result[i] = m.p
	}
	return result
}

// markUnmatched moves a booking to unmatched, records the event, releases any held
// payment, and notifies the customer.
func (s *BookingService) markUnmatched(ctx context.Context, bookingID, customerID string) {
	if _, err := s.bookings.UpdateStatus(ctx, bookingID, bookingmodels.StatusUnmatched); err != nil {
		return
	}
	_ = s.addEvent(ctx, bookingID, "booking_unmatched", bookingmodels.ActorSystem, "system")
	s.refundIfHeld(ctx, bookingID, "booking_unmatched")
	if customerID != "" {
		s.notifier.NotifyCustomerUnmatched(ctx, customerID, bookingID, map[string]interface{}{"booking_id": bookingID})
	}
}

// ─── Auto-complete worker ─────────────────────────────────────────────────────

// RunAutoCompleteWorker periodically promotes delivered bookings to completed once
// the grace period has elapsed. Run this in a goroutine; stop by cancelling ctx.
func (s *BookingService) RunAutoCompleteWorker(ctx context.Context) {
	s.autoCompleteDelivered(ctx) // catch any backlog from before the last restart
	ticker := time.NewTicker(autoCompleteCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.autoCompleteDelivered(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *BookingService) autoCompleteDelivered(ctx context.Context) {
	cutoff := time.Now().Add(-autoCompleteGracePeriod)
	bookings, err := s.bookings.ListDeliveredForAutoComplete(ctx, cutoff)
	if err != nil {
		return
	}
	for _, b := range bookings {
		if _, err := s.bookings.MarkCompleted(ctx, b.ID); err == nil {
			_ = s.addEvent(ctx, b.ID, "booking_completed", bookingmodels.ActorSystem, "system")
			s.settleOnComplete(ctx, b.ID)
			s.notifier.NotifyCustomerCompleted(ctx, b.CustomerID, b.ID, map[string]interface{}{"booking_id": b.ID})
		}
	}
}

// ─── Customer actions ─────────────────────────────────────────────────────────

func (s *BookingService) GetBooking(ctx context.Context, id, customerID string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.GetByIDForCustomer(ctx, id, customerID)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	return b.Public(), nil
}

func (s *BookingService) ListCustomerBookings(ctx context.Context, customerID string, limit, offset int) ([]bookingmodels.PublicBooking, error) {
	bookings, err := s.bookings.ListByCustomer(ctx, customerID, limit, offset)
	if err != nil {
		return nil, err
	}
	result := make([]bookingmodels.PublicBooking, len(bookings))
	for i, b := range bookings {
		result[i] = b.Public()
	}
	return result, nil
}

type SubmitReviewInput struct {
	BookingID        string
	CustomerID       string
	Rating           int
	ReviewText       string
	RecommendsDriver *bool
}

func (s *BookingService) SubmitReview(ctx context.Context, input SubmitReviewInput) (bookingmodels.PublicBookingReview, error) {
	var fields []apperrors.FieldViolation
	if input.Rating < 1 || input.Rating > 5 {
		fields = append(fields, apperrors.FieldViolation{Field: "rating", Message: "Rating must be between 1 and 5."})
	}
	if len(fields) > 0 {
		return bookingmodels.PublicBookingReview{}, apperrors.Validation("Please check your review details.", fields)
	}

	b, err := s.bookings.GetByIDForCustomer(ctx, input.BookingID, input.CustomerID)
	if err != nil {
		return bookingmodels.PublicBookingReview{}, err
	}
	if b.Status != bookingmodels.StatusDelivered && b.Status != bookingmodels.StatusCompleted {
		return bookingmodels.PublicBookingReview{}, apperrors.BadRequest("You can only review a delivered or completed booking.", nil)
	}
	if b.ProviderID == nil {
		return bookingmodels.PublicBookingReview{}, apperrors.BadRequest("This booking has no assigned provider.", nil)
	}

	review, err := s.bookings.CreateReview(ctx, bookingmodels.BookingReview{
		BookingID:        input.BookingID,
		CustomerID:       input.CustomerID,
		ProviderID:       *b.ProviderID,
		Rating:           input.Rating,
		ReviewText:       strings.TrimSpace(input.ReviewText),
		RecommendsDriver: input.RecommendsDriver,
	})
	if err != nil {
		return bookingmodels.PublicBookingReview{}, err
	}

	if b.Status == bookingmodels.StatusDelivered {
		if _, err := s.bookings.MarkCompleted(ctx, input.BookingID); err == nil {
			_ = s.addEvent(ctx, input.BookingID, "booking_completed", bookingmodels.ActorCustomer, input.CustomerID)
			s.settleOnComplete(ctx, input.BookingID)
			s.notifier.NotifyCustomerCompleted(ctx, input.CustomerID, input.BookingID, map[string]interface{}{"booking_id": input.BookingID})
		}
	}
	_ = s.addEvent(ctx, input.BookingID, "review_submitted", bookingmodels.ActorCustomer, input.CustomerID)
	return review.Public(), nil
}

func (s *BookingService) CancelBooking(ctx context.Context, id, customerID, reason string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.CancelByCustomer(ctx, id, customerID, reason)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	s.cancelMatch(id)
	_ = s.addEvent(ctx, id, "booking_cancelled", bookingmodels.ActorCustomer, customerID)
	s.refundIfHeld(ctx, id, "cancelled_by_customer")
	// Notify the assigned provider, if any, that the customer cancelled.
	if b.ProviderID != nil {
		s.notifier.NotifyProviderCancelled(ctx, *b.ProviderID, id, map[string]interface{}{"booking_id": id})
	}
	return b.Public(), nil
}

// ─── Provider actions ─────────────────────────────────────────────────────────

func (s *BookingService) GetProviderBooking(ctx context.Context, id, providerID string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.PublicBooking{}, apperrors.NotFound("Booking could not be found.", nil)
	}
	return b.Public(), nil
}

// GetProviderBookingReview returns the customer's review for one of the
// provider's bookings (the "Customer Review" section on the trip detail screen).
// Ownership is enforced: the booking must belong to the provider. Returns a
// not-found error when the customer has not left a review yet.
func (s *BookingService) GetProviderBookingReview(ctx context.Context, bookingID, providerID string) (bookingmodels.PublicBookingReview, error) {
	b, err := s.bookings.GetByID(ctx, bookingID)
	if err != nil {
		return bookingmodels.PublicBookingReview{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.PublicBookingReview{}, apperrors.NotFound("Booking could not be found.", nil)
	}
	review, err := s.bookings.GetReviewByBooking(ctx, bookingID)
	if err != nil {
		return bookingmodels.PublicBookingReview{}, err
	}
	return review.Public(), nil
}

func (s *BookingService) ListProviderBookings(ctx context.Context, providerID string, limit, offset int) ([]bookingmodels.PublicBooking, error) {
	bookings, err := s.bookings.ListByProvider(ctx, providerID, limit, offset)
	if err != nil {
		return nil, err
	}
	result := make([]bookingmodels.PublicBooking, len(bookings))
	for i, b := range bookings {
		result[i] = b.Public()
	}
	return result, nil
}

// earningsLookback caps how many of the provider's most recent bookings feed the
// earnings projection. Balances and the recent-transactions list are derived from
// these; older trips age out of the list but their fares were already earned.
const earningsLookback = 200

// GetProviderEarnings returns the trip-earnings projection backing the provider
// Earnings/Wallet screen: available/pending balances, today's stats, and a recent
// transactions list. Derived from the provider's own bookings — the authoritative
// wallet ledger remains in payment-wallet-service.
func (s *BookingService) GetProviderEarnings(ctx context.Context, providerID string) (bookingmodels.ProviderEarnings, error) {
	bookings, err := s.bookings.ListByProvider(ctx, providerID, earningsLookback, 0)
	if err != nil {
		return bookingmodels.ProviderEarnings{}, err
	}
	return bookingmodels.ComputeEarnings(bookings, time.Now()), nil
}

func (s *BookingService) AcceptBooking(ctx context.Context, id, providerID string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.PublicBooking{}, apperrors.Forbidden("You cannot accept this booking.", nil)
	}

	// Charge-on-acceptance: secure the fare before the provider starts the trip.
	// Wallet funds are held now; card must already be paid up-front; cash is
	// record-only. If we cannot secure payment, do not advance to accepted — the
	// provider should not start an unpaid trip.
	if err := s.ensurePaymentSecured(ctx, b); err != nil {
		return bookingmodels.PublicBooking{}, err
	}

	updated, err := s.bookings.MarkAccepted(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	_ = s.addEvent(ctx, id, "booking_accepted", bookingmodels.ActorProvider, providerID)
	s.notifier.NotifyCustomerAccepted(ctx, b.CustomerID, id, map[string]interface{}{"booking_id": id})
	return updated.Public(), nil
}

func (s *BookingService) RejectBooking(ctx context.Context, id, providerID string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.PublicBooking{}, apperrors.Forbidden("You cannot reject this booking.", nil)
	}

	// Stop the in-flight matcher for this booking before re-dispatching, so we
	// never have two matching goroutines racing on the same booking.
	s.cancelMatch(id)
	_ = s.availability.ReleaseMatchLock(ctx, providerID)
	updated, err := s.bookings.ResetToMatching(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	_ = s.addEvent(ctx, id, "booking_rejected", bookingmodels.ActorProvider, providerID)
	s.startMatchGoroutine(id, b.PickupLat, b.PickupLng)
	return updated.Public(), nil
}

func (s *BookingService) MarkEnRoutePickup(ctx context.Context, id, providerID string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.PublicBooking{}, apperrors.Forbidden("You cannot update this booking.", nil)
	}
	updated, err := s.bookings.MarkEnRoutePickup(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	_ = s.addEvent(ctx, id, "driver_en_route_pickup", bookingmodels.ActorProvider, providerID)
	s.notifier.NotifyCustomerEnRoutePickup(ctx, b.CustomerID, id, map[string]interface{}{"booking_id": id})
	return updated.Public(), nil
}

func (s *BookingService) MarkArrivedAtPickup(ctx context.Context, id, providerID string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.PublicBooking{}, apperrors.Forbidden("You cannot update this booking.", nil)
	}
	updated, err := s.bookings.MarkArrivedAtPickup(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	_ = s.addEvent(ctx, id, "driver_arrived_pickup", bookingmodels.ActorProvider, providerID)
	s.notifier.NotifyCustomerArrivedPickup(ctx, b.CustomerID, id, map[string]interface{}{"booking_id": id})
	return updated.Public(), nil
}

func (s *BookingService) MarkEnRouteDelivery(ctx context.Context, id, providerID string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.PublicBooking{}, apperrors.Forbidden("You cannot update this booking.", nil)
	}
	updated, err := s.bookings.MarkEnRouteDelivery(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	_ = s.addEvent(ctx, id, "driver_en_route_delivery", bookingmodels.ActorProvider, providerID)
	s.notifier.NotifyCustomerEnRouteDelivery(ctx, b.CustomerID, id, map[string]interface{}{"booking_id": id})
	return updated.Public(), nil
}

func (s *BookingService) ConfirmPickup(ctx context.Context, id, providerID string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.PublicBooking{}, apperrors.Forbidden("You cannot update this booking.", nil)
	}
	updated, err := s.bookings.MarkPickedUp(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	_ = s.addEvent(ctx, id, "cargo_picked_up", bookingmodels.ActorProvider, providerID)
	s.notifier.NotifyCustomerPickedUp(ctx, b.CustomerID, id, map[string]interface{}{"booking_id": id})
	return updated.Public(), nil
}

func (s *BookingService) ConfirmDelivery(ctx context.Context, id, providerID string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.PublicBooking{}, apperrors.Forbidden("You cannot update this booking.", nil)
	}
	updated, err := s.bookings.MarkDelivered(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	_ = s.addEvent(ctx, id, "cargo_delivered", bookingmodels.ActorProvider, providerID)
	s.notifier.NotifyCustomerDelivered(ctx, b.CustomerID, id, map[string]interface{}{"booking_id": id})
	// Auto-completion is handled by RunAutoCompleteWorker, not a goroutine sleep.
	return updated.Public(), nil
}

func (s *BookingService) CancelByProvider(ctx context.Context, id, providerID, reason string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.CancelByProvider(ctx, id, providerID, reason)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	_ = s.addEvent(ctx, id, "booking_cancelled_by_provider", bookingmodels.ActorProvider, providerID)
	s.refundIfHeld(ctx, id, "cancelled_by_provider")
	s.notifier.NotifyCustomerCancelledByProvider(ctx, b.CustomerID, id, map[string]interface{}{"booking_id": id})
	return b.Public(), nil
}

// ─── payment helpers ──────────────────────────────────────────────────────────

// ensurePaymentSecured makes sure the fare is secured before a provider starts a
// trip (charge-on-acceptance). Behaviour by method:
//   - cash:   record-only, nothing to secure.
//   - card:   must already be paid up-front (the customer completed Paystack
//     checkout while searching); we accept held/paid, otherwise reject.
//   - wallet: hold the fare from the customer wallet now.
//
// When no payment client is wired (local dev) payment is a no-op so the flow
// keeps working. A secured payment marks the booking PaymentStatusHeld.
func (s *BookingService) ensurePaymentSecured(ctx context.Context, b bookingmodels.Booking) error {
	switch b.PaymentMethod {
	case bookingmodels.PaymentMethodCash:
		return nil
	case bookingmodels.PaymentMethodCard:
		if b.PaymentStatus == bookingmodels.PaymentStatusHeld || b.PaymentStatus == bookingmodels.PaymentStatusPaid {
			return nil
		}
		if s.payments == nil {
			return nil // local dev without payment-wallet wired
		}
		return apperrors.BadRequest("The customer hasn't completed card payment yet. This request will reappear once payment clears — try again shortly.", nil)
	default: // wallet
		if s.payments == nil {
			return nil
		}
		if b.PaymentStatus == bookingmodels.PaymentStatusHeld || b.PaymentStatus == bookingmodels.PaymentStatusPaid {
			return nil
		}
		fare := fareToCharge(b)
		if fare <= 0 {
			return nil
		}
		providerID := ""
		if b.ProviderID != nil {
			providerID = *b.ProviderID
		}
		intentID, err := s.payments.HoldFromWallet(ctx, PaymentHoldInput{
			BookingID:  b.ID,
			CustomerID: b.CustomerID,
			ProviderID: providerID,
			AmountKobo: fare,
		})
		if err != nil {
			_, _ = s.bookings.SetPayment(ctx, b.ID, bookingmodels.PaymentStatusFailed, "")
			return apperrors.BadRequest("We couldn't charge your wallet. Check your balance or choose another payment method.", err)
		}
		_, _ = s.bookings.SetPayment(ctx, b.ID, bookingmodels.PaymentStatusHeld, intentID)
		return nil
	}
}

// settleOnComplete releases held funds to the provider on completion. Cash and
// no-op (local dev) settlements just mark the booking paid.
func (s *BookingService) settleOnComplete(ctx context.Context, bookingID string) {
	b, err := s.bookings.GetByID(ctx, bookingID)
	if err != nil {
		return
	}
	if b.PaymentMethod == bookingmodels.PaymentMethodCash || s.payments == nil {
		_, _ = s.bookings.SetPayment(ctx, bookingID, bookingmodels.PaymentStatusPaid, "")
		return
	}
	if b.PaymentStatus != bookingmodels.PaymentStatusHeld {
		return
	}
	if err := s.payments.Settle(ctx, bookingID); err != nil {
		return // leave as held; a later reconciliation/retry can settle
	}
	_, _ = s.bookings.SetPayment(ctx, bookingID, bookingmodels.PaymentStatusPaid, "")
}

// refundIfHeld reverses a held charge when a booking is cancelled or unmatched.
func (s *BookingService) refundIfHeld(ctx context.Context, bookingID, reason string) {
	if s.payments == nil {
		return
	}
	b, err := s.bookings.GetByID(ctx, bookingID)
	if err != nil {
		return
	}
	if b.PaymentStatus != bookingmodels.PaymentStatusHeld || b.PaymentIntentID == nil {
		return
	}
	if err := s.payments.Refund(ctx, *b.PaymentIntentID, fareToCharge(b), reason); err != nil {
		return
	}
	_, _ = s.bookings.SetPayment(ctx, bookingID, bookingmodels.PaymentStatusUnpaid, "")
}

// fareToCharge returns the amount to charge for a booking: the final fare if set,
// otherwise the estimate.
func fareToCharge(b bookingmodels.Booking) int64 {
	if b.FareFinalKobo != nil && *b.FareFinalKobo > 0 {
		return *b.FareFinalKobo
	}
	if b.FareEstimateKobo != nil {
		return *b.FareEstimateKobo
	}
	return 0
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// validCoordinate rejects missing/null-island coordinates (0,0) and
// out-of-range lat/lng so a booking can't be created with garbage geometry that
// would produce a bogus distance and fare.
func validCoordinate(lat, lng float64) bool {
	if lat == 0 && lng == 0 {
		return false
	}
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

func (s *BookingService) addEvent(ctx context.Context, bookingID, eventType, actorType, actorID string) error {
	return s.bookings.AddEvent(ctx, bookingmodels.BookingEvent{
		ID:        uuid.NewString(),
		BookingID: bookingID,
		EventType: eventType,
		ActorType: actorType,
		ActorID:   actorID,
	})
}

// haversineKm returns the great-circle distance in kilometres between two lat/lng points.
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	return r * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
