package bookingusecases

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	bookingclients "cosmicforge/logistics/services/hauling-service/internal/features/booking/clients"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	bookingrepo "cosmicforge/logistics/services/hauling-service/internal/features/booking/repositories"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/services/hauling-service/internal/truckcatalog"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/handover"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/routing"
)

const (
	autoCompleteGracePeriod   = 30 * time.Minute
	autoCompleteCheckInterval = 5 * time.Minute

	// matchRecoveryCheckInterval is how often the recovery worker scans for
	// bookings whose matcher goroutine was lost (e.g. across a restart).
	matchRecoveryCheckInterval = 30 * time.Second
	// matchRecoveryPendingGrace keeps the recovery worker away from pending_match
	// bookings so fresh that their matcher may not have registered yet.
	matchRecoveryPendingGrace = 10 * time.Second
	// matchLockSlack pads provider match locks and stale-offer cutoffs beyond the
	// acceptance window so a lock never expires mid-decision.
	matchLockSlack = 15 * time.Second
	// matchCandidateLimit caps how many nearby providers one rescan pulls out
	// of the spatial index. Offers go out nearest-first, so a booking that
	// cannot be placed among the closest 50 trucks in range is not going to
	// be placed by widening the list.
	matchCandidateLimit = 50
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

type RestrictionGate interface {
	Restricted(ctx context.Context, organizationID, subjectType, subjectReference string) (bool, error)
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
	Settle(ctx context.Context, bookingID string, paymentReference string) error
	// Refund reverses a charge on cancel/unmatched.
	Refund(ctx context.Context, paymentIntentID string, amountKobo int64, reason string) error
}

// executionProviderBinder confirms a held payment is bound to the provider
// about to be paid (Section 17). Optional on a PaymentClient.
type executionProviderBinder interface {
	EnsureExecutionProvider(ctx context.Context, bookingID, providerID string) error
}

type CommerceDeliverySettlementAuthority interface {
	ReleaseCommerceDelivery(ctx context.Context, parentOrderID, vendorOrderID, providerID, completionReference string) error
}

type commerceMissionScopeReader interface {
	CommerceMissionScope(ctx context.Context, bookingID string) (string, []string, bool, error)
	ListCompletedCommerceBookings(ctx context.Context, limit int) ([]string, error)
}

type commerceProviderMetadataReader interface {
	CommerceProviderMetadata(ctx context.Context, bookingID string) (*bookingmodels.CommerceOperations, error)
}

var ErrPaymentOutcomeUnknown = errors.New("payment authority outcome is unknown")

type PaymentHoldInput struct {
	BookingID  string
	CustomerID string
	ProviderID string
	AmountKobo int64
}

type BookingService struct {
	bookings           bookingrepo.BookingRepository
	availability       availabilityrepositories.AvailabilityStore
	trucks             TruckLookup
	capability         CapabilityAuthority
	capabilityMatching CapabilityMatchingConfig
	payments           PaymentClient
	commerceSettlement CommerceDeliverySettlementAuthority
	notifier           *bookingclients.BookingNotifier
	matchTimeout       time.Duration // per-provider acceptance window
	searchWindow       time.Duration // total time to keep searching before unmatched
	maxRadiusKm        float64
	allowCashPayments  bool
	restrictions       RestrictionGate
	activeMatches      sync.Map // map[bookingID]*matchHandle
	handoverSecret     []byte
	secureHandover     bookingrepo.SecureHandoverRepository
	routes             routing.Fetcher

	// managedSettlement and managedHandoff are the Section 12 boundary -- see
	// managed_settlement.go. They decide whether a completed corporate job
	// takes the legacy direct-to-provider wallet path or the Fleet managed
	// path, and that decision is never made by a live Fleet lookup at
	// completion time.
	managedSettlement ManagedSettlementConfig
	managedHandoff    ManagedSettlementHandoff
	// completedWorkEnabled gates writing the canonical Fleet completed-work
	// record. Off by default (HAULING_FLEET_COMPLETED_WORK_ENABLED).
	completedWorkEnabled bool
}

// matchHandle wraps a context.CancelFunc so it can be stored in sync.Map and
// compared by pointer identity. context.CancelFunc is uncomparable, so storing
// it directly panics on CompareAndDelete.
type matchHandle struct {
	cancel context.CancelFunc
}

type Options struct {
	Bookings     bookingrepo.BookingRepository
	Availability availabilityrepositories.AvailabilityStore
	Trucks       TruckLookup
	// Capability is Hauling's truck capability store (Section 9). nil keeps
	// the legacy weight-and-type matcher exactly as it was, which is what
	// lets this ship dark.
	Capability          CapabilityAuthority
	CapabilityMatching  CapabilityMatchingConfig
	Payments            PaymentClient // nil disables payment binding (local dev)
	CommerceSettlement  CommerceDeliverySettlementAuthority
	Notifier            *bookingclients.BookingNotifier
	MatchTimeoutSeconds int
	SearchWindowSeconds int
	MaxRadiusKm         float64
	HandoverSecret      []byte // signs/verifies pickup & delivery handover codes
	AllowCashPayments   bool
	Restrictions        RestrictionGate
	// Routes fetches road geometry for the live map and the progress bar.
	// nil disables route fetching; every consumer then falls back to
	// straight-line geometry.
	Routes routing.Fetcher
	// SecureHandover is optional for isolated tests. Production automatically
	// uses the Postgres booking repository's existing handover authority.
	SecureHandover bookingrepo.SecureHandoverRepository

	// ManagedSettlement/ManagedHandoff/CompletedWorkEnabled configure the
	// Section 10 corporate completion path. All three are nil/zero-safe: a
	// service built without them behaves exactly as before, which is what
	// keeps every independent job unchanged.
	ManagedSettlement    ManagedSettlementConfig
	ManagedHandoff       ManagedSettlementHandoff
	CompletedWorkEnabled bool
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
	secureHandover := opts.SecureHandover
	if secureHandover == nil {
		secureHandover, _ = opts.Bookings.(bookingrepo.SecureHandoverRepository)
	}
	return &BookingService{
		bookings:             opts.Bookings,
		availability:         opts.Availability,
		trucks:               opts.Trucks,
		capability:           opts.Capability,
		capabilityMatching:   opts.CapabilityMatching,
		payments:             opts.Payments,
		commerceSettlement:   opts.CommerceSettlement,
		notifier:             opts.Notifier,
		matchTimeout:         matchTimeout,
		searchWindow:         searchWindow,
		maxRadiusKm:          radius,
		allowCashPayments:    opts.AllowCashPayments,
		restrictions:         opts.Restrictions,
		handoverSecret:       opts.HandoverSecret,
		secureHandover:       secureHandover,
		routes:               opts.Routes,
		managedSettlement:    opts.ManagedSettlement,
		managedHandoff:       opts.ManagedHandoff,
		completedWorkEnabled: opts.CompletedWorkEnabled,
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
	// Empty means "any truck" (skips the matcher's type filter). Anything else
	// must be a real truck type, otherwise the booking would be stored fine and
	// then silently never match. The catalogue is the same source provider
	// registration and the matcher read, so the three can no longer diverge.
	if !truckcatalog.ValidForBooking(input.PreferredTruckType) {
		fields = append(fields, apperrors.FieldViolation{Field: "preferred_truck_type", Message: "Choose a valid truck type."})
	}
	paymentMethod := strings.TrimSpace(input.PaymentMethod)
	if paymentMethod == "" {
		paymentMethod = bookingmodels.PaymentMethodWallet
	}
	if !bookingmodels.ValidPaymentMethods[paymentMethod] {
		fields = append(fields, apperrors.FieldViolation{Field: "payment_method", Message: "Choose a valid payment method."})
	} else if paymentMethod == bookingmodels.PaymentMethodCard {
		// Card is disabled until the Paystack webhook flips unpaid->held; without
		// it a card booking can never be accepted (see ensurePaymentSecured).
		fields = append(fields, apperrors.FieldViolation{Field: "payment_method", Message: "Card payment is coming soon. Choose wallet or cash."})
	} else if paymentMethod == bookingmodels.PaymentMethodCash && !s.allowCashPayments {
		fields = append(fields, apperrors.FieldViolation{Field: "payment_method", Message: "Cash payment is not enabled."})
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
		PreferredTruckType: truckcatalog.Canonical(input.PreferredTruckType),
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
	// Both endpoints are known now, so the cargo leg's road geometry can be
	// fetched while matching runs.
	s.fetchDropoffRoute(created.ID,
		geo.Point{Lat: input.PickupLat, Lng: input.PickupLng},
		geo.Point{Lat: input.DropoffLat, Lng: input.DropoffLng})
	s.startMatchGoroutine(created.ID, input.PickupLat, input.PickupLng)
	return created.Public(), nil
}

// ProviderLocation is the live position of the provider assigned to a booking.
//
// Heading, speed and accuracy come from the provider device's GPS stream. They
// let the customer app rotate the vehicle marker and dead-reckon between
// fixes; they are zero when the device did not report them.
type ProviderLocation struct {
	BookingID string  `json:"booking_id"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	Heading   float64 `json:"heading"`
	Speed     float64 `json:"speed"`
	Accuracy  float64 `json:"accuracy"`
	UpdatedAt int64   `json:"updated_at"`
	Available bool    `json:"available"`
}

// TrackTarget identifies whose live position a customer is entitled to watch
// for one booking.
type TrackTarget struct {
	BookingID  string
	ProviderID string
}

// ResolveTrackTarget authorises a customer to stream the assigned provider's
// position for one of their bookings. It is the ownership check behind the
// live-track websocket: without it a customer could subscribe to any
// provider's channel by guessing an id.
func (s *BookingService) ResolveTrackTarget(ctx context.Context, bookingID, customerID string) (TrackTarget, error) {
	b, err := s.bookings.GetByIDForCustomer(ctx, bookingID, customerID)
	if err != nil {
		return TrackTarget{}, err
	}
	if b.ProviderID == nil || *b.ProviderID == "" {
		return TrackTarget{}, apperrors.NotFound("No driver is assigned to this booking yet.", nil)
	}
	return TrackTarget{BookingID: bookingID, ProviderID: *b.ProviderID}, nil
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
		Heading:   st.Heading,
		Speed:     st.Speed,
		Accuracy:  st.Accuracy,
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

// StartCommerceMatching begins matching for a booking created by the Section
// 17F commerce mission contract.
//
// It is the same matcher every other booking uses. The commerce path needs an
// exported entry point because the booking is created by that feature's own
// transaction rather than by CreateBooking, and starting matching in-process
// keeps the customer from waiting for a sweep. The existing match-recovery
// worker remains the safety net: a booking left in pending_match by a crash is
// re-adopted regardless of whether this call ever ran.
func (s *BookingService) StartCommerceMatching(
	_ context.Context, bookingID string, pickupLat, pickupLng float64,
) {
	if s == nil || bookingID == "" {
		return
	}
	s.startMatchGoroutine(bookingID, pickupLat, pickupLng)
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
	// unmatched once the deadline passes with no acceptance. Providers who
	// already rejected this booking are excluded from every subsequent scan
	// (see eligibleProviders) so a rejection never re-dispatches back to them.
	deadline := time.Now().Add(s.searchWindow)
	const rescanInterval = 3 * time.Second

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return
		}

		// Read the geohash cell index rather than every online provider on
		// the platform: the radius filter now happens in Redis, so a rescan
		// costs work proportional to trucks near this pickup.
		providers, err := s.nearbyProviders(ctx, pickupLat, pickupLng)
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
		lockTTL := s.matchTimeout + matchLockSlack
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

		// Decide with a single guarded compare-and-set instead of read-then-reset:
		// ResetToMatching only succeeds while the booking is still awaiting this
		// provider's acceptance. If the provider's accept lands first — even in the
		// instant after the timeout fires — the reset fails and the accepted trip is
		// never clobbered back to matching.
		if _, err := s.bookings.ResetToMatching(cleanupCtx, bookingID, p.ProviderID); err == nil {
			// Provider did not accept in time — free them and try the next closest.
			// A timed-out provider is treated like a rejection for this booking:
			// without it, the next 3s rescan re-sorts nearest-first and re-offers the
			// same silent provider, ping-ponging until the search window dies.
			_ = s.availability.ReleaseMatchLock(cleanupCtx, p.ProviderID)
			_ = s.availability.MarkRejected(cleanupCtx, bookingID, p.ProviderID, s.searchWindow+matchLockSlack)
			_ = s.addEvent(cleanupCtx, bookingID, "match_timeout", bookingmodels.ActorSystem, p.ProviderID)
			continue
		}

		// The reset was refused: the booking was accepted, cancelled, or otherwise
		// moved on. Re-read to decide whether the provider keeps their lock.
		current, err := s.bookings.GetByID(cleanupCtx, bookingID)
		if err != nil {
			_ = s.availability.ReleaseMatchLock(cleanupCtx, p.ProviderID)
			return false, true
		}
		// Keep the lock only when this provider accepted — they own the trip now.
		if current.Status != bookingmodels.StatusAccepted {
			_ = s.availability.ReleaseMatchLock(cleanupCtx, p.ProviderID)
		}
		return current.Status == bookingmodels.StatusAccepted, true
	}
	return false, false
}

// nearbyProviders reads candidate providers from the spatial index, bounded to
// the serviceable radius.
//
// If the index comes back empty while the legacy online set is not, it falls
// back to the full scan. That covers the deploy window in which providers who
// went online before this build have an availability key but no index entry
// yet; their next heartbeat re-indexes them, so the fallback self-heals within
// one heartbeat rather than stranding bookings.
func (s *BookingService) nearbyProviders(ctx context.Context, pickupLat, pickupLng float64) ([]availabilityrepositories.ProviderStatus, error) {
	center := geo.Point{Lat: pickupLat, Lng: pickupLng}

	nearby, err := s.availability.NearbyProviders(ctx, center, s.maxRadiusKm, matchCandidateLimit)
	if err != nil {
		return nil, err
	}
	if len(nearby) > 0 {
		return nearby, nil
	}

	online, err := s.availability.GetOnlineProviders(ctx)
	if err != nil {
		return nil, err
	}
	return online, nil
}

// eligibleProviders filters online providers down to those that can actually do
// the job — within the serviceable radius, with an active truck of a type and
// capacity that fits the cargo, and not someone who already rejected this exact
// booking — and returns them sorted nearest-first.
func (s *BookingService) eligibleProviders(
	ctx context.Context,
	providers []availabilityrepositories.ProviderStatus,
	booking bookingmodels.Booking,
	pickupLat, pickupLng float64,
) []availabilityrepositories.ProviderStatus {
	wantType := strings.ToLower(strings.TrimSpace(booking.PreferredTruckType))

	rejected, err := s.availability.GetRejectedProviders(ctx, booking.ID)
	if err != nil {
		rejected = nil
	}

	type scored struct {
		p    availabilityrepositories.ProviderStatus
		dist float64
	}
	var matches []scored
	distances := make(map[string]float64, len(providers))
	for _, p := range providers {
		if s.restrictions != nil {
			restricted, err := s.restrictions.Restricted(ctx, "", "PROVIDER", p.ProviderID)
			if err != nil || restricted {
				continue
			}
			if p.TruckID != "" {
				restricted, err = s.restrictions.Restricted(ctx, "", "ASSET", p.TruckID)
				if err != nil || restricted {
					continue
				}
			}
		}
		if rejected[p.ProviderID] {
			continue
		}
		dist := haversineKm(pickupLat, pickupLng, p.Lat, p.Lng)
		if dist > s.maxRadiusKm {
			continue
		}
		// Legacy truck eligibility, kept as the pre-capability behaviour so a
		// deployment with HAULING_CAPABILITY_MATCHING_ENABLED off matches
		// exactly as it did before. Note what it does NOT do: an unresolved
		// truck falls through, which is the fail-open the capability filter
		// below replaces.
		if s.trucks != nil && p.TruckID != "" {
			truck, err := s.trucks.GetTruck(ctx, p.TruckID)
			if err == nil {
				if truck.CapacityKg > 0 && booking.CargoWeightKg > truck.CapacityKg {
					continue
				}
				if wantType != "" && !truckcatalog.SatisfiesPreferredType(truck.TruckType, wantType) {
					continue
				}
			}
		}
		distances[p.ProviderID] = dist
		matches = append(matches, scored{p: p, dist: dist})
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].dist < matches[j].dist })

	result := make([]availabilityrepositories.ProviderStatus, len(matches))
	for i, m := range matches {
		result[i] = m.p
	}

	// Hard cargo fit. This is the gate the legacy block above cannot be: it
	// refuses an unresolved truck instead of waving it through, and it reads
	// every physical fact rather than weight and type alone.
	//
	// Hard eligibility and ranking stay separate. The proximity sort has
	// already run, and this only REMOVES candidates from it -- a nearer truck
	// can never be scored back into a job it does not fit.
	if s.capability != nil && s.capabilityMatching.Enabled {
		eligible, decisions := s.capabilityFilter(ctx, booking, result, distances)
		s.recordCapabilityDecisions(ctx, decisions)
		if s.capabilityMatching.FailClosed {
			if len(eligible) < len(result) {
				logging.Notice("hauling-capability",
					"booking %s: %d of %d candidates failed hard cargo fit (%s)",
					booking.ID, len(result)-len(eligible), len(result), capabilityShortfall(decisions))
			}
			return eligible
		}
		// Shadow mode: the decision was computed and recorded, and the legacy
		// selection stands. Anything else here would make the flag pair
		// meaningless.
		if len(eligible) < len(result) {
			logging.Notice("hauling-capability",
				"shadow: booking %s would have lost %d of %d candidates (%s)",
				booking.ID, len(result)-len(eligible), len(result), capabilityShortfall(decisions))
		}
	}
	return result
}

// markUnmatched moves a booking to unmatched, records the event, releases any held
// payment, and notifies the customer. The repo transition is a compare-and-set from
// pending_match, so a deadline firing concurrently with an accept (or cancel) is a
// silent no-op instead of clobbering the booking.
func (s *BookingService) markUnmatched(ctx context.Context, bookingID, customerID string) {
	if _, err := s.bookings.MarkUnmatched(ctx, bookingID); err != nil {
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
			// HLG-017: the canonical Fleet completed-work record is persisted
			// at the authoritative completion, before settlement is routed,
			// so Fleet holds the record even if settlement is withheld.
			s.recordCompletedWork(ctx, b.ID)
			s.settleOnComplete(ctx, b.ID)
			s.notifier.NotifyCustomerCompleted(ctx, b.CustomerID, b.ID, map[string]interface{}{"booking_id": b.ID})
		}
	}
	if scopes, ok := s.bookings.(commerceMissionScopeReader); ok {
		if completed, readErr := scopes.ListCompletedCommerceBookings(ctx, 100); readErr == nil {
			for _, bookingID := range completed {
				s.settleOnComplete(ctx, bookingID)
			}
		}
	}
}

// ─── Match recovery worker ────────────────────────────────────────────────────

// RunMatchRecoveryWorker periodically re-adopts bookings whose matching was
// orphaned. Matchers live in process memory, so a restart strands any booking
// mid-search: pending_match with no goroutine, or awaiting_acceptance whose
// offer window lapsed with nobody left to reset it. Stranded bookings resume
// matching, or go unmatched once their search window (from creation) expired.
// Run in a goroutine; stop by cancelling ctx.
func (s *BookingService) RunMatchRecoveryWorker(ctx context.Context) {
	s.recoverStaleMatching(ctx) // adopt any backlog from before the last restart
	ticker := time.NewTicker(matchRecoveryCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.recoverStaleMatching(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *BookingService) recoverStaleMatching(ctx context.Context) {
	now := time.Now()
	pendingCutoff := now.Add(-matchRecoveryPendingGrace)
	// A healthy matcher resets an unanswered offer right after matchTimeout; an
	// awaiting_acceptance row older than that (plus lock slack) has no live owner.
	awaitingCutoff := now.Add(-(s.matchTimeout + matchLockSlack))
	stale, err := s.bookings.ListStaleMatching(ctx, pendingCutoff, awaitingCutoff)
	if err != nil {
		return
	}
	for _, b := range stale {
		if ctx.Err() != nil {
			return
		}
		// A live matcher in this process owns the booking — leave it alone.
		if _, live := s.activeMatches.Load(b.ID); live {
			continue
		}
		// Legacy card bookings dispatch from InitiateCardPayment; one that was
		// never initiated (no intent) is waiting on the customer, not on us.
		if b.PaymentMethod == bookingmodels.PaymentMethodCard && b.PaymentIntentID == nil {
			continue
		}
		// Orphaned offer: clear the assignment so the booking is back in
		// pending_match, and free the provider for other bookings. The guarded
		// reset refuses if the provider accepted in the meantime.
		if b.Status == bookingmodels.StatusAwaitingAcceptance && b.ProviderID != nil {
			provider := *b.ProviderID
			if _, err := s.bookings.ResetToMatching(ctx, b.ID, provider); err != nil {
				continue
			}
			_ = s.availability.ReleaseMatchLock(ctx, provider)
			_ = s.addEvent(ctx, b.ID, "match_recovered", bookingmodels.ActorSystem, provider)
		}
		// The search window is anchored to creation: a booking that already spent
		// its window (including downtime) goes unmatched instead of resuming.
		if now.After(b.CreatedAt.Add(s.searchWindow)) {
			s.markUnmatched(ctx, b.ID, b.CustomerID)
			continue
		}
		s.startMatchGoroutine(b.ID, b.PickupLat, b.PickupLng)
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
	if _, err := s.bookings.GetReviewByBooking(ctx, input.BookingID); err == nil {
		return bookingmodels.PublicBookingReview{}, apperrors.Conflict("You have already reviewed this trip.", nil)
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
			s.recordCompletedWork(ctx, input.BookingID)
			s.settleOnComplete(ctx, input.BookingID)
			s.notifier.NotifyCustomerCompleted(ctx, input.CustomerID, input.BookingID, map[string]interface{}{"booking_id": input.BookingID})
		}
	}
	_ = s.addEvent(ctx, input.BookingID, "review_submitted", bookingmodels.ActorCustomer, input.CustomerID)
	return review.Public(), nil
}

// GetCustomerBookingReview returns the customer's own review for one of their
// bookings (so the trip detail screen can render a read-only view instead of
// a resubmittable form once a review exists). Ownership is enforced: the
// booking must belong to the customer. Returns a not-found error when no
// review has been left yet.
func (s *BookingService) GetCustomerBookingReview(ctx context.Context, bookingID, customerID string) (bookingmodels.PublicBookingReview, error) {
	if _, err := s.bookings.GetByIDForCustomer(ctx, bookingID, customerID); err != nil {
		return bookingmodels.PublicBookingReview{}, err
	}
	review, err := s.bookings.GetReviewByBooking(ctx, bookingID)
	if err != nil {
		return bookingmodels.PublicBookingReview{}, err
	}
	return review.Public(), nil
}

func (s *BookingService) CancelBooking(ctx context.Context, id, customerID, reason string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.CancelByCustomer(ctx, id, customerID, reason)
	if err != nil {
		// The guard refuses once the cargo is picked up (and for terminal
		// bookings). Tell the customer why instead of a bare not-found.
		if isNotFound(err) {
			if current, getErr := s.bookings.GetByIDForCustomer(ctx, id, customerID); getErr == nil {
				switch current.Status {
				case bookingmodels.StatusPickedUp, bookingmodels.StatusEnRouteDelivery:
					return bookingmodels.PublicBooking{}, apperrors.Conflict("Your cargo is already in transit and can no longer be cancelled. Contact support for help.", err)
				case bookingmodels.StatusDelivered, bookingmodels.StatusCompleted, bookingmodels.StatusCancelled:
					return bookingmodels.PublicBooking{}, apperrors.Conflict("This booking can no longer be cancelled.", err)
				}
			}
		}
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
	return s.providerBookingView(ctx, b)
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
		result[i], err = s.providerBookingView(ctx, b)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *BookingService) providerBookingView(ctx context.Context, b bookingmodels.Booking) (bookingmodels.PublicBooking, error) {
	view := b.Public()
	reader, ok := s.bookings.(commerceProviderMetadataReader)
	if !ok {
		return view, nil
	}
	metadata, err := reader.CommerceProviderMetadata(ctx, b.ID)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if metadata == nil {
		return view, nil
	}
	view.Commerce = metadata
	// The provider has a separate earnings projection. Commerce customer price,
	// payment method and settlement state do not belong on this operational view.
	view.FareEstimateKobo = nil
	view.FareFinalKobo = nil
	view.PaymentMethod = ""
	view.PaymentStatus = ""
	return view, nil
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
	if s.restrictions != nil {
		restricted, gateErr := s.restrictions.Restricted(ctx, "", "PROVIDER", providerID)
		if gateErr != nil {
			return bookingmodels.PublicBooking{}, apperrors.Unavailable("Restriction authority is unavailable.", gateErr)
		}
		if restricted {
			return bookingmodels.PublicBooking{}, apperrors.Forbidden("You are temporarily restricted from accepting work.", nil)
		}
	}

	// Charge-on-acceptance: secure the fare before the provider starts the trip.
	// Wallet funds are held now; card must already be paid up-front; cash is
	// record-only. If we cannot secure payment, do not advance to accepted — the
	// provider should not start an unpaid trip.
	if err := s.ensurePaymentSecured(ctx, b); err != nil {
		return bookingmodels.PublicBooking{}, err
	}

	updated, err := s.bookings.MarkAccepted(ctx, id, providerID)
	if err != nil {
		if isNotFound(err) {
			return bookingmodels.PublicBooking{}, apperrors.Conflict("This request has expired or been reassigned.", err)
		}
		return bookingmodels.PublicBooking{}, err
	}
	_ = s.addEvent(ctx, id, "booking_accepted", bookingmodels.ActorProvider, providerID)
	// The driver's position is only known now, so this is the first point the
	// approach leg can be routed for the customer's map.
	s.fetchPickupRoute(id, providerID, geo.Point{Lat: b.PickupLat, Lng: b.PickupLng})
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
	// Remember this rejection so eligibleProviders never re-offers the booking
	// to the same provider on a later re-scan (TTL covers the rest of the
	// search window, mirroring the lockTTL slack used elsewhere in matching).
	_ = s.availability.MarkRejected(ctx, id, providerID, s.searchWindow+matchLockSlack)
	updated, err := s.bookings.ResetToMatching(ctx, id, providerID)
	if err != nil {
		if isNotFound(err) {
			return bookingmodels.PublicBooking{}, apperrors.Conflict("This request can no longer be rejected.", err)
		}
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

func (s *BookingService) nextSecureHandoverVersion(ctx context.Context, bookingID string, stage handover.Step) (int64, error) {
	current, _, err := s.secureHandover.GetHandoverChallenge(ctx, bookingID, stage)
	if errors.Is(err, bookingrepo.ErrSecureHandoverChallengeNotFound) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	return current.Version + 1, nil
}

// MarkArrivedAtPickup transitions the booking to arrived_at_pickup and
// generates the driver's pickup credential (shown on the driver's own app and
// verified by the customer). The credential is returned to the caller (the
// driver) — it must never be exposed through PublicBooking / customer-facing
// endpoints.
func (s *BookingService) MarkArrivedAtPickup(ctx context.Context, id, providerID string) (ArrivedAtPickupResult, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return ArrivedAtPickupResult{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return ArrivedAtPickupResult{}, apperrors.Forbidden("You cannot update this booking.", nil)
	}
	var rec handover.CodeRecord
	if s.secureHandover != nil {
		version, versionErr := s.nextSecureHandoverVersion(ctx, id, handover.StepPickup)
		if versionErr != nil {
			return ArrivedAtPickupResult{}, versionErr
		}
		rec, err = handover.NewSecureCodeRecord(s.handoverSecret, "hauling", handover.StepPickup, uuid.NewString(), version, id, providerID, b.CustomerID, 0, time.Now())
	} else {
		rec, err = handover.NewCodeRecord(s.handoverSecret, handover.BuildChallengeID("hauling", handover.StepPickup, id), time.Now())
	}
	if err != nil {
		return ArrivedAtPickupResult{}, apperrors.Internal("Could not generate a pickup code.", err)
	}
	updated, err := s.bookings.MarkArrivedAtPickup(ctx, id, rec)
	if err != nil {
		return ArrivedAtPickupResult{}, err
	}
	_ = s.addEvent(ctx, id, "driver_arrived_pickup", bookingmodels.ActorProvider, providerID)
	s.notifier.NotifyCustomerArrivedPickup(ctx, b.CustomerID, id, map[string]interface{}{"booking_id": id})
	s.notifier.NotifyCustomerPickupVerificationRequired(ctx, b.CustomerID, id, map[string]interface{}{"booking_id": id})
	return ArrivedAtPickupResult{
		Booking: updated.Public(),
		PickupCredential: s.withPickupDirection(ctx, id, func() PickupCredentialView {
			if rec.Version > 0 {
				return securePickupCredentialView(rec, "ACTIVE")
			}
			return pickupCredentialViewFrom(updated)
		}()),
	}, nil
}

func (s *BookingService) MarkEnRouteDelivery(ctx context.Context, id, providerID string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.PublicBooking{}, apperrors.Forbidden("You cannot update this booking.", nil)
	}
	var rec handover.CodeRecord
	if s.secureHandover != nil {
		version, versionErr := s.nextSecureHandoverVersion(ctx, id, handover.StepDelivery)
		if versionErr != nil {
			return bookingmodels.PublicBooking{}, versionErr
		}
		rec, err = handover.NewSecureCodeRecord(s.handoverSecret, "hauling", handover.StepDelivery, uuid.NewString(), version, id, providerID, b.CustomerID, 0, time.Now())
	} else {
		rec, err = handover.NewCodeRecord(s.handoverSecret, handover.BuildChallengeID("hauling", handover.StepDelivery, id), time.Now())
	}
	if err != nil {
		return bookingmodels.PublicBooking{}, apperrors.Internal("Could not generate a delivery code.", err)
	}
	updated, err := s.bookings.MarkEnRouteDelivery(ctx, id, rec)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	_ = s.addEvent(ctx, id, "driver_en_route_delivery", bookingmodels.ActorProvider, providerID)
	s.notifier.NotifyCustomerEnRouteDelivery(ctx, b.CustomerID, id, map[string]interface{}{"booking_id": id})
	deliveryPayload := map[string]interface{}{"booking_id": id, "code": rec.Code}
	if rec.Version > 0 {
		deliveryPayload["qr_token"] = rec.QRToken
		deliveryPayload["challenge_ref"] = rec.ChallengeRef
		deliveryPayload["version"] = rec.Version
	}
	s.notifier.NotifyCustomerDeliveryCodeReady(ctx, b.CustomerID, id, deliveryPayload)
	return updated.Public(), nil
}

// ConfirmPickup marks the cargo picked up. The driver submits no code here —
// the actual gate is server-side: the customer must already have verified
// the driver's pickup credential via VerifyDriverAtPickup.
func (s *BookingService) ConfirmPickup(ctx context.Context, id, providerID string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.PublicBooking{}, apperrors.Forbidden("You cannot update this booking.", nil)
	}
	if b.PickupCodeVerifiedAt == nil {
		return bookingmodels.PublicBooking{}, apperrors.Conflict("The customer has not verified your identity yet.", nil)
	}
	updated, err := s.bookings.MarkPickedUp(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	_ = s.addEvent(ctx, id, "cargo_picked_up", bookingmodels.ActorProvider, providerID)
	s.notifier.NotifyCustomerPickedUp(ctx, b.CustomerID, id, map[string]interface{}{"booking_id": id})
	return updated.Public(), nil
}

// ConfirmDelivery marks the cargo delivered, recording the proof-of-delivery
// photo and receiver-signature URLs (media-file-service uploads; either may be
// empty when skipped or when the upload failed — proof is best-effort). The
// driver must submit the customer's delivery code; it is validated before the
// status transition is allowed.
func (s *BookingService) ConfirmDelivery(ctx context.Context, id, providerID, code, proofPhotoURL, signatureURL string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.PublicBooking{}, apperrors.Forbidden("You cannot update this booking.", nil)
	}
	if s.secureHandover != nil {
		updated, consumed, consumeErr := s.secureHandover.ConsumeHandoverChallenge(ctx, id, handover.StepDelivery, providerID, strings.TrimSpace(code), s.handoverSecret, proofPhotoURL, signatureURL)
		if consumeErr != nil && !errors.Is(consumeErr, bookingrepo.ErrSecureHandoverChallengeNotFound) {
			return bookingmodels.PublicBooking{}, consumeErr
		}
		if consumeErr == nil && consumed {
			_ = s.addEvent(ctx, id, "cargo_delivered", bookingmodels.ActorProvider, providerID)
			s.notifier.NotifyCustomerDelivered(ctx, b.CustomerID, id, map[string]interface{}{"booking_id": id})
		}
		if consumeErr == nil {
			return updated.Public(), nil
		}
	}
	if err := s.verifyDeliveryCode(ctx, id, b, code); err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	updated, err := s.bookings.MarkDelivered(ctx, id, strings.TrimSpace(proofPhotoURL), strings.TrimSpace(signatureURL))
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
	// A provider can cancel from awaiting_acceptance, where a matcher goroutine
	// may still be waiting on them — stop it and free their match lock so
	// neither the goroutine nor the lock outlives the cancelled booking.
	s.cancelMatch(id)
	_ = s.availability.ReleaseMatchLock(ctx, providerID)
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
// A missing payment client fails closed. A secured payment marks the booking
// PaymentStatusHeld; an unresolved authoritative write remains UNKNOWN.
func (s *BookingService) ensurePaymentSecured(ctx context.Context, b bookingmodels.Booking) error {
	switch b.PaymentMethod {
	case bookingmodels.PaymentMethodCash:
		return nil
	case bookingmodels.PaymentMethodCard:
		if b.PaymentStatus == bookingmodels.PaymentStatusHeld || b.PaymentStatus == bookingmodels.PaymentStatusPaid {
			return nil
		}
		if s.payments == nil {
			return apperrors.Unavailable("Payment service is not configured.", nil)
		}
		return apperrors.BadRequest("The customer hasn't completed card payment yet. This request will reappear once payment clears — try again shortly.", nil)
	default: // wallet
		if s.payments == nil {
			return apperrors.Unavailable("Payment service is not configured.", nil)
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
			status := bookingmodels.PaymentStatusFailed
			if errors.Is(err, ErrPaymentOutcomeUnknown) {
				status = bookingmodels.PaymentStatusUnknown
			}
			_, _ = s.bookings.SetPayment(ctx, b.ID, status, intentID)
			return apperrors.BadRequest("We couldn't charge your wallet. Check your balance or choose another payment method.", err)
		}
		_, _ = s.bookings.SetPayment(ctx, b.ID, bookingmodels.PaymentStatusHeld, intentID)
		return nil
	}
}

// settleOnComplete releases held funds to the provider on completion. Cash is
// operationally complete without fabricating PAID; a missing client cannot
// change the financial state.
func (s *BookingService) settleOnComplete(ctx context.Context, bookingID string) {
	b, err := s.bookings.GetByID(ctx, bookingID)
	if err != nil {
		return
	}
	// HLG-025: an organization-managed job is NEVER paid directly to the
	// provider once isolation is on. The route is read from the booking's own
	// persisted settlement mode, so a Fleet outage cannot reroute corporate
	// work onto the legacy path. This gate covers BOTH the plain wallet
	// settlement below and the Commerce delivery release, because both move
	// money to the executing provider.
	if !s.routeCompletedSettlement(ctx, bookingID) {
		return
	}
	if scopes, ok := s.bookings.(commerceMissionScopeReader); ok {
		parentID, vendorIDs, commerce, scopeErr := scopes.CommerceMissionScope(ctx, bookingID)
		if scopeErr != nil {
			return
		}
		if commerce {
			if s.commerceSettlement == nil || b.ProviderID == nil || strings.TrimSpace(*b.ProviderID) == "" {
				return
			}
			for _, vendorID := range vendorIDs {
				if err = s.commerceSettlement.ReleaseCommerceDelivery(ctx, parentID, vendorID, *b.ProviderID, bookingID); err != nil {
					return
				}
			}
			return
		}
	}
	if b.PaymentMethod == bookingmodels.PaymentMethodCash || s.payments == nil {
		return
	}
	if b.PaymentStatus != bookingmodels.PaymentStatusHeld {
		return
	}
	reference := ""
	if b.PaymentIntentID != nil {
		reference = *b.PaymentIntentID
	}
	// Section 17: the held payment must be bound to the provider who did the
	// job. After a recovery replacement that is not the provider who accepted,
	// and the rebind that follows recovery may not have landed. An
	// unconfirmable binding leaves the payment for reconciliation rather than
	// releasing it to whoever it happens to point at.
	if binder, ok := s.payments.(executionProviderBinder); ok && b.ProviderID != nil && *b.ProviderID != "" {
		if err := binder.EnsureExecutionProvider(ctx, bookingID, *b.ProviderID); err != nil {
			_, _ = s.bookings.SetPayment(ctx, bookingID, bookingmodels.PaymentStatusUnknown, reference)
			return
		}
	}
	if err := s.payments.Settle(ctx, bookingID, reference); err != nil {
		_, _ = s.bookings.SetPayment(ctx, bookingID, bookingmodels.PaymentStatusUnknown, reference)
		return
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
		_, _ = s.bookings.SetPayment(ctx, bookingID, bookingmodels.PaymentStatusUnknown, *b.PaymentIntentID)
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

// isNotFound reports whether err carries the apperrors not_found code (used to
// tell a failed status-guard compare-and-set apart from a real failure).
func isNotFound(err error) bool {
	appErr := apperrors.From(err)
	return appErr != nil && appErr.Code == apperrors.CodeNotFound
}

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

// haversineKm returns the great-circle distance in kilometres between two
// lat/lng points, delegating to the platform-wide implementation.
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	return geo.HaversineKm(
		geo.Point{Lat: lat1, Lng: lng1},
		geo.Point{Lat: lat2, Lng: lng2},
	)
}
