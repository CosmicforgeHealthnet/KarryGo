package booking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/rollout"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/serviceability"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/handover"
	"cosmicforge/logistics/shared/go/journeyshare"
	"cosmicforge/logistics/shared/go/phonenumber"
	"cosmicforge/logistics/shared/go/routing"
)

// BroadcastControl is the request-feature surface the booking usecase drives.
// Satisfied by *request.Service. StartBroadcast is idempotent by booking id;
// calling it in-process (instead of pub/sub) gives synchronous error semantics
// so a failed broadcast can immediately refund the wallet hold.
type BroadcastControl interface {
	StartBroadcast(ctx context.Context, event requestfeature.BookingDispatchCreatedEvent) (requestfeature.RequestBroadcast, error)
	CancelBroadcastForBooking(ctx context.Context, bookingID string) error
}

// LiveStore is the availability surface used for the customer availability
// check and live rider tracking. Satisfied by *availability.RedisLiveStore.
type LiveStore interface {
	GetNearby(ctx context.Context, request availability.NearbyProvidersRequest) ([]availability.NearbyProvider, error)
	GetLocation(ctx context.Context, providerID string) (availability.Location, bool, error)
}

// TripControl cancels an assigned trip on the customer's behalf, and handles
// the customer-facing side of pickup/delivery handover verification. Calling
// the trip feature in-process (instead of pub/sub) lets these endpoints
// verify the outcome synchronously and keeps the handover secret + stored
// hashes inside the trip feature, which owns pickup/delivery execution.
type TripControl interface {
	CancelForCustomer(ctx context.Context, bookingID, reason string) error
	// VerifyPickupCode lets the customer confirm the rider's pickup credential.
	VerifyPickupCode(ctx context.Context, bookingID, code string) error
	// ResendDeliveryCode regenerates the customer's own delivery credential.
	ResendDeliveryCode(ctx context.Context, bookingID string) (handover.CodeRecord, error)
}

// LifecycleNotifier sends booking-lifecycle notifications to the customer and
// rider apps via notification-service. Satisfied by
// *bookingclients.BookingNotifier; all methods are fire-and-forget.
type LifecycleNotifier interface {
	NotifyCustomerAccepted(ctx context.Context, customerID, bookingID string, data map[string]interface{})
	NotifyCustomerUnmatched(ctx context.Context, customerID, bookingID string, data map[string]interface{})
	NotifyCustomerCompleted(ctx context.Context, customerID, bookingID string, data map[string]interface{})
	NotifyCustomerCancelledByProvider(ctx context.Context, customerID, bookingID string, data map[string]interface{})
	NotifyProviderCancelled(ctx context.Context, providerID, bookingID string, data map[string]interface{})
}

type CommerceDeliverySettlementAuthority interface {
	ReleaseCommerceDelivery(ctx context.Context, parentOrderID, vendorOrderID, providerID, completionReference string) error
}

type commerceMissionScopeReader interface {
	CommerceMissionScope(ctx context.Context, bookingID string) (string, []string, bool, error)
}

type serviceabilityQuoteAuthority interface {
	AuthorizeBooking(context.Context, string, string, serviceability.BookingBinding, time.Time) (serviceability.Quote, error)
}

type serviceabilityObserver interface {
	ObserveBooking(context.Context, string, string, CreateBookingInput) (string, string, error)
}

type idempotentBookingCreator interface {
	InsertIdempotent(context.Context, Booking, string, string, string, int64, string) (Booking, bool, error)
}

type bookingOperationsReader interface {
	GetBookingOperations(context.Context, string) (*BookingOperationsProjection, error)
}

type Service struct {
	repository         Repository
	broadcast          BroadcastControl
	trips              TripControl
	live               LiveStore
	payments           PaymentClient
	events             EventPublisher
	notifier           LifecycleNotifier
	commerceSettlement CommerceDeliverySettlementAuthority
	serviceability     serviceabilityQuoteAuthority
	observer           serviceabilityObserver
	protectedCreate    bool
	rolloutPolicy      rollout.Policy
	radiusKM           float64
	now                func() time.Time
	// routes fetches road geometry for the live map and the progress bar.
	// nil disables route fetching; consumers then fall back to straight lines.
	routes routing.Fetcher
	// tripLocation is the trip-bound point projection (the same reader the
	// Share Journey adapter uses). Customer tracking reads only through it.
	tripLocation TripLocationReader
}

// TripLocationReader returns the customer-safe projection of one trip,
// including a location point bound to that trip's CURRENT provider, and
// terminal state. Satisfied by *customershare.Service.
type TripLocationReader interface {
	Read(ctx context.Context, tripRef, customerID string) (journeyshare.Projection, error)
}

// WithTripLocation wires trip-bound customer tracking. Without it the
// customer location route fails closed rather than exposing the rider's
// global position.
func (s *Service) WithTripLocation(reader TripLocationReader) *Service {
	s.tripLocation = reader
	return s
}

func NewService(repository Repository, broadcast BroadcastControl, trips TripControl, live LiveStore, payments PaymentClient, events EventPublisher, radiusKM float64) *Service {
	if radiusKM <= 0 {
		radiusKM = 10
	}
	return &Service{
		repository: repository,
		broadcast:  broadcast,
		trips:      trips,
		live:       live,
		payments:   payments,
		events:     events,
		radiusKM:   radiusKM,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// WithClock overrides the clock for tests.
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// WithNotifier wires the lifecycle notifier (call after NewService). A nil
// notifier disables notifications (local dev without notification-service).
// WithRoutes enables road-route geometry for the live map and the progress
// bar. A nil fetcher leaves routes unfetched and every consumer falling back
// to straight-line geometry.
func (s *Service) WithRoutes(routes routing.Fetcher) *Service {
	s.routes = routes
	return s
}

func (s *Service) WithNotifier(notifier LifecycleNotifier) *Service {
	s.notifier = notifier
	return s
}

func (s *Service) WithCommerceSettlement(authority CommerceDeliverySettlementAuthority) *Service {
	s.commerceSettlement = authority
	return s
}

// WithServiceabilityQuoteAuthority enables the additive protected-create
// policy. Legacy deployment remains unchanged while the Sections 4-6 flags are
// disabled.
func (s *Service) WithServiceabilityQuoteAuthority(authority serviceabilityQuoteAuthority, required bool) *Service {
	s.serviceability = authority
	s.protectedCreate = required
	return s
}

func (s *Service) WithServiceabilityObserver(observer serviceabilityObserver) *Service {
	s.observer = observer
	return s
}

// WithRolloutPolicy configures server-owned cohort selection. The resulting
// protection snapshot is persisted with the Booking and is therefore not
// weakened by a later environment flag change.
func (s *Service) WithRolloutPolicy(policy rollout.Policy) *Service {
	s.rolloutPolicy = policy
	return s
}

// notifyData builds the standard template payload for lifecycle notifications.
func notifyData(booking Booking) map[string]interface{} {
	return map[string]interface{}{
		"booking_id":      booking.ID,
		"service":         "dispatch",
		"pickup_address":  booking.PickupAddress,
		"dropoff_address": booking.DropoffAddress,
		"fare_kobo":       booking.FareAmount,
		"currency":        booking.Currency,
	}
}

func (s *Service) CheckAvailability(ctx context.Context, lat, lng float64) (AvailabilityResponse, error) {
	if err := validateCoords(lat, lng, "pickup"); err != nil {
		return AvailabilityResponse{}, err
	}
	if s.live == nil {
		return AvailabilityResponse{}, apperrors.Unavailable("Rider availability is temporarily unavailable.", nil)
	}
	providers, err := s.live.GetNearby(ctx, availability.NearbyProvidersRequest{
		Latitude: lat, Longitude: lng, RadiusKM: s.radiusKM, Limit: availability.NearbyMaxLimit,
	})
	if err != nil {
		return AvailabilityResponse{}, err
	}
	count := int64(len(providers))
	return AvailabilityResponse{Available: count > 0, Count: count}, nil
}

func (s *Service) Estimate(ctx context.Context, in EstimateRequest) (EstimateResponse, error) {
	if err := validateCoords(in.PickupLat, in.PickupLng, "pickup"); err != nil {
		return EstimateResponse{}, err
	}
	if err := validateCoords(in.DropoffLat, in.DropoffLng, "dropoff"); err != nil {
		return EstimateResponse{}, err
	}
	distance := haversineKM(in.PickupLat, in.PickupLng, in.DropoffLat, in.DropoffLng)
	return EstimateResponse{
		DistanceKM:       roundKM(distance),
		FareEstimateKobo: calculateFareKobo(distance, in.PackageSize, in.ServiceTier),
		Currency:         currencyNGN,
		ETA:              estimateETA(distance),
		Breakdown:        fareBreakdown(distance, in.PackageSize, in.ServiceTier),
	}, nil
}

func (s *Service) CreateBooking(ctx context.Context, customerID string, in CreateBookingInput) (BookingView, error) {
	return s.CreateBookingWithKey(ctx, customerID, "", in)
}

// CreateBookingWithKey adds customer-scoped replay safety while preserving the
// original method for existing in-process callers and tests.
func (s *Service) CreateBookingWithKey(ctx context.Context, customerID, idempotencyKey string, in CreateBookingInput) (BookingView, error) {
	normalizedReceiverPhone, err := validateCreateInput(in)
	if err != nil {
		return BookingView{}, err
	}
	in.ReceiverPhone = normalizedReceiverPhone
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if s.observer != nil && !s.protectedCreate {
		observationKey := idempotencyKey
		if observationKey == "" {
			observationKey = "observe:" + bookingCreateFingerprint(in)
		}
		_, _, _ = s.observer.ObserveBooking(ctx, customerID, observationKey, in)
	}
	quoteRef := strings.TrimSpace(in.ServiceabilityQuoteRef)
	if s.protectedCreate && (idempotencyKey == "" || quoteRef == "") {
		return BookingView{}, apperrors.Validation("Please check your details.", []apperrors.FieldViolation{{Field: "serviceability_quote_ref", Message: "A serviceability quote and Idempotency-Key are required."}})
	}

	distance := haversineKM(in.Pickup.Lat, in.Pickup.Lng, in.Dropoff.Lat, in.Dropoff.Lng)
	fare := calculateFareKobo(distance, in.PackageSize, in.ServiceTier)
	var quoteVersion int64
	var quoteFingerprint string
	if quoteRef != "" {
		if s.serviceability == nil {
			return BookingView{}, apperrors.Unavailable("Serviceability verification is temporarily unavailable.", nil)
		}
		quote, quoteErr := s.serviceability.AuthorizeBooking(ctx, customerID, quoteRef, serviceability.BookingBinding{
			Pickup:      serviceability.Point{Latitude: in.Pickup.Lat, Longitude: in.Pickup.Lng},
			Dropoff:     serviceability.Point{Latitude: in.Dropoff.Lat, Longitude: in.Dropoff.Lng},
			PackageType: in.PackageType, PackageSize: in.PackageSize,
			PackageWeightGrams: int64(math.Round(in.PackageWeight * 1000)), Fragile: in.IsFragile,
			ServiceTier: in.ServiceTier,
		}, s.now())
		if quoteErr != nil {
			switch {
			case errors.Is(quoteErr, serviceability.ErrQuoteNotFound):
				return BookingView{}, apperrors.NotFound("Serviceability quote not found.", nil)
			case errors.Is(quoteErr, serviceability.ErrQuoteExpired):
				return BookingView{}, apperrors.Conflict("Serviceability quote has expired. Request a new quote.", nil)
			case errors.Is(quoteErr, serviceability.ErrQuoteRejected), errors.Is(quoteErr, serviceability.ErrQuoteMismatch):
				return BookingView{}, apperrors.Conflict("Booking details do not match an approved serviceability quote.", nil)
			default:
				return BookingView{}, apperrors.Unavailable("Serviceability verification is temporarily unavailable.", quoteErr)
			}
		}
		fare = quote.FareEstimateKobo
		distance = float64(quote.Route.DistanceMeters) / 1000
		quoteVersion = quote.Version
		quoteFingerprint = quote.RequestFingerprint
	}

	booking := Booking{
		CustomerID:                 customerID,
		Status:                     StatusSearching,
		PickupAddress:              strings.TrimSpace(in.Pickup.Address),
		PickupLat:                  in.Pickup.Lat,
		PickupLng:                  in.Pickup.Lng,
		DropoffAddress:             strings.TrimSpace(in.Dropoff.Address),
		DropoffLat:                 in.Dropoff.Lat,
		DropoffLng:                 in.Dropoff.Lng,
		DistanceKM:                 roundKM(distance),
		FareAmount:                 fare,
		Currency:                   currencyNGN,
		PaymentMethod:              PaymentMethodWallet,
		PaymentStatus:              PaymentUnpaid,
		ReceiverName:               strings.TrimSpace(in.ReceiverName),
		ReceiverPhone:              normalizedReceiverPhone,
		ReceiverEmail:              strings.ToLower(strings.TrimSpace(in.ReceiverEmail)),
		ReceiverCustomerID:         strings.TrimSpace(in.ReceiverCustomerID),
		PackageDesc:                strings.TrimSpace(in.PackageDesc),
		PackageType:                strings.TrimSpace(in.PackageType),
		PackageSize:                NormalizePackageSize(in.PackageSize),
		IsFragile:                  in.IsFragile,
		ServiceTier:                NormalizeServiceTier(in.ServiceTier),
		ServiceabilityQuoteRef:     quoteRef,
		ServiceabilityQuoteVersion: quoteVersion,
	}
	protection := s.rolloutPolicy.Protect(rollout.Context{
		CustomerID:  customerID,
		SourceType:  "CUSTOMER",
		SourceRef:   customerID,
		ServiceTier: booking.ServiceTier,
	})
	booking.RolloutProtectionMode = protection.Mode
	booking.RolloutPolicyVersion = protection.PolicyVersion
	booking.HandoverRequired = protection.HandoverRequired
	booking.PickupEvidenceRequired = protection.PickupEvidenceRequired
	booking.DeliveryEvidenceRequired = protection.DeliveryEvidenceRequired
	booking.ProviderTransferRequired = protection.ProviderTransferRequired
	if in.PackageWeight > 0 {
		weight := in.PackageWeight
		booking.PackageWeight = &weight
	}

	requestFingerprint := bookingCreateFingerprint(in)
	created := Booking{}
	replayed := false
	if idempotencyKey != "" {
		creator, ok := s.repository.(idempotentBookingCreator)
		if !ok {
			return BookingView{}, apperrors.Unavailable("Idempotent booking creation is temporarily unavailable.", nil)
		}
		created, replayed, err = creator.InsertIdempotent(ctx, booking, idempotencyKey, requestFingerprint, quoteRef, quoteVersion, quoteFingerprint)
	} else {
		created, err = s.repository.Insert(ctx, booking)
	}
	if err != nil {
		return BookingView{}, err
	}
	if replayed && created.Status != StatusSearching {
		state, found, stateErr := s.repository.GetWithDerivedState(ctx, created.ID)
		if stateErr != nil {
			return BookingView{}, stateErr
		}
		if !found {
			return BookingView{}, apperrors.NotFound("Delivery booking not found.", nil)
		}
		return state.View(), nil
	}

	// Wallet escrow: hold the fare before matching starts (fail fast).
	if s.payments != nil && created.PaymentStatus == PaymentUnpaid {
		ref, holdErr := s.payments.Hold(ctx, created.ID, customerID, fare)
		if holdErr != nil {
			now := s.now()
			var unknown *UnknownPaymentOutcome
			if errors.As(holdErr, &unknown) {
				// Stop matching, but do not claim failed/refunded money on a timeout.
				if err := s.repository.SetPayment(ctx, created.ID, PaymentUnpaid, ref); err != nil {
					return BookingView{}, err
				}
				if _, err := s.repository.SetTerminal(ctx, created.ID, StatusCancelled, PaymentUnpaid, "payment_outcome_unknown", now); err != nil {
					return BookingView{}, err
				}
				return BookingView{}, apperrors.Unavailable("Wallet confirmation is pending. The booking will not be dispatched; any held payment will be reconciled.", nil)
			}
			if _, err := s.repository.SetTerminal(ctx, created.ID, StatusCancelled, PaymentFailed, "payment_failed", now); err != nil {
				log.Printf("booking payment-failed terminal update failed booking_id=%s: %v", created.ID, err)
			}
			return BookingView{}, apperrors.New(http.StatusBadRequest, "payment_failed", "Wallet payment was not accepted.", holdErr)
		}
		created.PaymentStatus = PaymentHeld
		created.PaymentRef = ref
		if err := s.repository.SetPayment(ctx, created.ID, PaymentHeld, ref); err != nil {
			return BookingView{}, err
		}
		current, found, readErr := s.repository.GetWithDerivedState(ctx, created.ID)
		if readErr != nil {
			return BookingView{}, readErr
		}
		if !found || current.Booking.Status != StatusSearching {
			if found {
				s.refundHeld(ctx, current, "refund-booking-"+created.ID, "payment_outcome_unknown")
			}
			return BookingView{}, apperrors.Conflict("Booking no longer permits dispatch.", nil)
		}
	}

	if s.broadcast == nil {
		return BookingView{}, apperrors.Unavailable("Rider matching is temporarily unavailable.", nil)
	}
	weight := 0.0
	if created.PackageWeight != nil {
		weight = *created.PackageWeight
	}
	event := requestfeature.BookingDispatchCreatedEvent{
		BookingID:      created.ID,
		CustomerID:     customerID,
		PickupLat:      created.PickupLat,
		PickupLng:      created.PickupLng,
		DropoffLat:     created.DropoffLat,
		DropoffLng:     created.DropoffLng,
		PickupAddress:  created.PickupAddress,
		DropoffAddress: created.DropoffAddress,
		FareAmount:     created.FareAmount,
		ServiceType:    "dispatch",
		PaymentMethod:  created.PaymentMethod,
		Currency:       created.Currency,
		ReceiverName:   created.ReceiverName,
		ReceiverPhone:  created.ReceiverPhone,
		PackageDesc:    created.PackageDesc,
		PackageWeight:  weight,
		PackageType:    created.PackageType,
		PackageSize:    created.PackageSize,
		IsFragile:      created.IsFragile,
		ServiceTier:    created.ServiceTier,
		OccurredAt:     s.now(),
	}
	if _, err := s.broadcast.StartBroadcast(ctx, event); err != nil {
		now := s.now()
		state := DerivedState{Booking: created}
		s.refundHeld(ctx, state, "refund-booking-"+created.ID, "broadcast_failed")
		if _, terminalErr := s.repository.SetTerminal(ctx, created.ID, StatusCancelled, s.currentPaymentStatus(ctx, created.ID, created.PaymentStatus), "broadcast_failed", now); terminalErr != nil {
			log.Printf("booking broadcast-failed terminal update failed booking_id=%s: %v", created.ID, terminalErr)
		}
		return BookingView{}, apperrors.Unavailable("We couldn't start looking for a rider. Your wallet was not charged.", err)
	}

	// Both endpoints are known now, so the delivery leg's road geometry can be
	// fetched in the background while a rider is found.
	s.fetchDropoffRoute(created.ID,
		geo.Point{Lat: created.PickupLat, Lng: created.PickupLng},
		geo.Point{Lat: created.DropoffLat, Lng: created.DropoffLng})

	return BookingView{Booking: created, Status: WireSearching}, nil
}

func (s *Service) GetBooking(ctx context.Context, customerID, id string) (BookingView, error) {
	state, err := s.getOwnedBooking(ctx, customerID, id)
	if err != nil {
		return BookingView{}, err
	}
	view := state.View()
	if reader, ok := s.repository.(bookingOperationsReader); ok {
		projection, projectionErr := reader.GetBookingOperations(ctx, state.Booking.ID)
		if projectionErr != nil {
			return BookingView{}, projectionErr
		}
		view.Operations = projection
	}
	return view, nil
}

func bookingCreateFingerprint(in CreateBookingInput) string {
	in.Pickup.Address = strings.TrimSpace(in.Pickup.Address)
	in.Dropoff.Address = strings.TrimSpace(in.Dropoff.Address)
	in.ReceiverName = strings.TrimSpace(in.ReceiverName)
	in.ReceiverPhone = strings.TrimSpace(in.ReceiverPhone)
	in.ReceiverEmail = strings.ToLower(strings.TrimSpace(in.ReceiverEmail))
	in.ReceiverCustomerID = strings.TrimSpace(in.ReceiverCustomerID)
	in.PackageDesc = strings.TrimSpace(in.PackageDesc)
	in.PackageType = strings.ToLower(strings.TrimSpace(in.PackageType))
	in.PackageSize = NormalizePackageSize(in.PackageSize)
	in.ServiceTier = NormalizeServiceTier(in.ServiceTier)
	in.PaymentMethod = PaymentMethodWallet
	in.ServiceabilityQuoteRef = strings.TrimSpace(in.ServiceabilityQuoteRef)
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Service) ListBookings(ctx context.Context, customerID string, options ListBookingsOptions) ([]BookingView, error) {
	states, err := s.repository.ListWithDerivedState(ctx, customerID, options)
	if err != nil {
		return nil, err
	}
	views := make([]BookingView, 0, len(states))
	for _, state := range states {
		views = append(views, state.View())
	}
	return views, nil
}

func (s *Service) CancelBooking(ctx context.Context, customerID, id, reason string) (BookingView, error) {
	state, err := s.getOwnedBooking(ctx, customerID, id)
	if err != nil {
		return BookingView{}, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "customer_cancelled"
	}

	switch state.WireStatus() {
	case WireCancelled, WireUnmatched, WireCompleted:
		return BookingView{}, apperrors.Conflict("This delivery has already ended.", nil)
	case WireInTransit, WireDelivered:
		return BookingView{}, apperrors.Conflict("Your package is already in transit and can no longer be cancelled. Contact support if you need help.", nil)
	}

	now := s.now()
	if state.TripID != "" {
		// Rider already assigned: cancel the trip synchronously and verify it
		// actually cancelled before refunding — the trip-side guard no-ops for
		// trips that just moved to in_progress. The refund shares the
		// trip-keyed idempotency key so a concurrent provider cancel dedupes
		// wallet-side.
		if s.trips == nil {
			return BookingView{}, apperrors.Unavailable("The delivery could not be cancelled. Please try again.", nil)
		}
		if err := s.trips.CancelForCustomer(ctx, state.Booking.ID, reason); err != nil {
			return BookingView{}, err
		}
		refreshed, ok, err := s.repository.GetWithDerivedState(ctx, state.Booking.ID)
		if err != nil {
			return BookingView{}, err
		}
		if ok && refreshed.TripStatus != "" && refreshed.TripStatus != "cancelled" && refreshed.TripStatus != "failed" {
			return BookingView{}, apperrors.Conflict("Your package is already in transit and can no longer be cancelled. Contact support if you need help.", nil)
		}
		s.refundHeld(ctx, state, "refund-"+state.TripID, "customer_cancelled")
	} else {
		if state.BroadcastStatus == "accepted" {
			// Accept landed but the trip row is not created yet; retry shortly.
			return BookingView{}, apperrors.Conflict("A rider was just assigned. Please try again in a moment.", nil)
		}
		if s.broadcast != nil {
			if err := s.broadcast.CancelBroadcastForBooking(ctx, state.Booking.ID); err != nil {
				return BookingView{}, err
			}
		}
		// Close the accept race: if a rider accepted between the status read
		// and the (no-op) broadcast cancel, do not refund with the pre-accept key.
		refreshed, ok, err := s.repository.GetWithDerivedState(ctx, state.Booking.ID)
		if err != nil {
			return BookingView{}, err
		}
		if ok && (refreshed.TripID != "" || refreshed.BroadcastStatus == "accepted") {
			return BookingView{}, apperrors.Conflict("A rider was just assigned. Please try again in a moment.", nil)
		}
		if s.events != nil {
			_ = s.events.PublishBookingCancelled(ctx, state.Booking.ID, reason, now)
		}
		s.refundHeld(ctx, state, "refund-booking-"+state.Booking.ID, "customer_cancelled")
	}

	if _, err := s.repository.SetTerminal(ctx, state.Booking.ID, StatusCancelled, s.currentPaymentStatus(ctx, state.Booking.ID, state.Booking.PaymentStatus), reason, now); err != nil {
		return BookingView{}, err
	}
	// Tell the assigned rider (if any) the customer cancelled.
	if s.notifier != nil {
		riderID := state.TripProviderID
		if riderID == "" {
			riderID = state.Booking.ProviderID
		}
		s.notifier.NotifyProviderCancelled(ctx, riderID, state.Booking.ID, notifyData(state.Booking))
	}
	refreshed, ok, err := s.repository.GetWithDerivedState(ctx, state.Booking.ID)
	if err != nil || !ok {
		state.Booking.Status = StatusCancelled
		state.Booking.CancelledAt = &now
		return state.View(), nil
	}
	return refreshed.View(), nil
}

func (s *Service) SubmitReview(ctx context.Context, customerID, id string, in ReviewInput) (Review, error) {
	if in.Rating < 1 || in.Rating > 5 {
		return Review{}, validationError("rating", "Rating must be between 1 and 5.")
	}
	state, err := s.getOwnedBooking(ctx, customerID, id)
	if err != nil {
		return Review{}, err
	}
	switch state.WireStatus() {
	case WireDelivered, WireCompleted:
	default:
		return Review{}, apperrors.Conflict("You can review this delivery once it has been delivered.", nil)
	}
	providerID := state.TripProviderID
	if providerID == "" {
		providerID = state.Booking.ProviderID
	}
	if providerID == "" {
		return Review{}, apperrors.Conflict("No rider is linked to this delivery yet.", nil)
	}
	wasDelivered := state.WireStatus() == WireDelivered
	review, err := s.repository.InsertReview(ctx, Review{
		BookingID:          state.Booking.ID,
		ProviderID:         providerID,
		CustomerID:         customerID,
		Rating:             in.Rating,
		ReviewText:         strings.TrimSpace(in.ReviewText),
		RecommendsProvider: in.RecommendsProvider,
	})
	if err != nil {
		return Review{}, err
	}
	// Best-effort: a review always saves even if this settlement step fails —
	// the auto-complete worker in the trip feature is the safety net for a
	// booking left held after a successful review. The review itself must
	// never be lost because of a downstream settlement hiccup.
	if wasDelivered {
		if cerr := s.HandleTripCompleted(ctx, state.Booking.ID, s.now()); cerr != nil {
			log.Printf("booking review-triggered completion failed booking_id=%s: %v", state.Booking.ID, cerr)
		}
	}
	return review, nil
}

func (s *Service) GetReview(ctx context.Context, customerID, id string) (Review, error) {
	if _, err := s.getOwnedBooking(ctx, customerID, id); err != nil {
		return Review{}, err
	}
	review, ok, err := s.repository.GetReviewByBookingID(ctx, id)
	if err != nil {
		return Review{}, err
	}
	if !ok {
		return Review{}, apperrors.NotFound("No review found for this delivery.", nil)
	}
	return review, nil
}

func (s *Service) GetLiveLocation(ctx context.Context, customerID, id string) (LocationResponse, error) {
	state, err := s.getOwnedBooking(ctx, customerID, id)
	if err != nil {
		return LocationResponse{}, err
	}
	// Only the rider CURRENTLY assigned to this booking's trip is ever
	// tracked. The booking's own provider_id is historical and is never a
	// fallback: after reassignment it names somebody else's rider.
	if state.TripID == "" || state.TripProviderID == "" {
		return LocationResponse{}, apperrors.NotFound("No rider has been assigned yet.", nil)
	}
	if s.tripLocation == nil {
		return LocationResponse{}, apperrors.Unavailable("Live tracking is temporarily unavailable.", nil)
	}
	projection, err := s.tripLocation.Read(ctx, state.TripID, customerID)
	if err != nil {
		return LocationResponse{}, err
	}
	// A finished trip stops exposing where the rider is, and a point that is
	// not bound to this trip's current assignment is never returned.
	if projection.Terminal || !projection.Location.Available || projection.Location.Lat == nil || projection.Location.Lng == nil || projection.Location.UpdatedAt == nil {
		return LocationResponse{}, apperrors.NotFound("The rider's location is not available right now.", nil)
	}
	response := LocationResponse{
		Lat:       *projection.Location.Lat,
		Lng:       *projection.Location.Lng,
		UpdatedAt: *projection.Location.UpdatedAt,
		State:     projection.Location.State,
	}
	if projection.Location.Heading != nil {
		response.Heading = *projection.Location.Heading
	}
	if projection.Location.AccuracyM != nil {
		response.Accuracy = *projection.Location.AccuracyM
	}
	return response, nil
}

// ResolveTrackTarget authorises a customer to stream their rider's live
// position for one booking. It is the ownership check behind the live-track
// websocket: without it a customer could subscribe to any rider's channel by
// guessing an id.
func (s *Service) ResolveTrackTarget(ctx context.Context, customerID, id string) (string, error) {
	state, err := s.getOwnedBooking(ctx, customerID, id)
	if err != nil {
		return "", err
	}
	// The stream is the rider's own channel, so it is authorised only while
	// that rider is the CURRENT provider of an ACTIVE trip for this booking.
	// A finished or reassigned trip must not keep streaming the rider.
	if state.TripProviderID == "" || !activeTripStatuses[state.TripStatus] {
		return "", apperrors.NotFound("No rider is currently assigned.", nil)
	}
	return state.TripProviderID, nil
}

// activeTripStatuses are the trip states in which the assigned rider is on
// the customer's job and may be tracked.
var activeTripStatuses = map[string]bool{
	"assigned": true, "en_route_pickup": true, "arrived_pickup": true, "in_progress": true,
}

// VerifyDriverAtPickup lets the customer confirm the rider's identity using
// the pickup credential displayed on the rider's app (entered manually or
// scanned as a QR). On success, the rider's StartTrip becomes unblocked.
func (s *Service) VerifyDriverAtPickup(ctx context.Context, customerID, id, code string) (BookingView, error) {
	state, err := s.getOwnedBooking(ctx, customerID, id)
	if err != nil {
		return BookingView{}, err
	}
	if s.trips == nil {
		return BookingView{}, apperrors.Unavailable("Driver verification is temporarily unavailable.", nil)
	}
	if err := s.trips.VerifyPickupCode(ctx, state.Booking.ID, code); err != nil {
		return BookingView{}, err
	}
	refreshed, ok, err := s.repository.GetWithDerivedState(ctx, state.Booking.ID)
	if err != nil {
		return BookingView{}, err
	}
	if !ok {
		return state.View(), nil
	}
	return refreshed.View(), nil
}

// GetVerificationCode returns the customer's own delivery credential for
// redisplay. Only the delivery step is readable by the customer — the
// pickup credential belongs to the rider and is never exposed here.
func (s *Service) GetVerificationCode(ctx context.Context, customerID, id, step string) (VerificationCodeView, error) {
	if step == string(handover.StepPickup) {
		return VerificationCodeView{}, apperrors.Forbidden("You cannot view this code.", nil)
	}
	if step != string(handover.StepDelivery) {
		return VerificationCodeView{}, apperrors.BadRequest("Invalid verification step.", nil)
	}
	state, err := s.getOwnedBooking(ctx, customerID, id)
	if err != nil {
		return VerificationCodeView{}, err
	}
	if state.DeliveryCode == "" {
		return VerificationCodeView{}, apperrors.NotFound("No delivery code has been generated yet.", nil)
	}
	remaining := handover.MaxVerifyAttempts - state.DeliveryCodeAttempts
	if remaining < 0 {
		remaining = 0
	}
	code := state.DeliveryCode
	if decoder, ok := s.trips.(interface {
		DecodeDeliveryCode(string, string) (string, error)
	}); ok {
		code, err = decoder.DecodeDeliveryCode(state.TripID, code)
		if err != nil {
			return VerificationCodeView{}, apperrors.Unavailable("Delivery credential is unavailable.", nil)
		}
	} else if strings.HasPrefix(code, "handover_") {
		return VerificationCodeView{}, apperrors.Unavailable("Delivery credential is unavailable.", nil)
	}
	view := VerificationCodeView{Code: code, AttemptsRemaining: remaining}
	if state.DeliveryCodeExpiresAt != nil {
		view.ExpiresAt = state.DeliveryCodeExpiresAt
	}
	return view, nil
}

// ResendVerificationCode regenerates the customer's delivery credential.
// Only the delivery step may be resent by the customer.
func (s *Service) ResendVerificationCode(ctx context.Context, customerID, id, step string) (VerificationCodeView, error) {
	if step == string(handover.StepPickup) {
		return VerificationCodeView{}, apperrors.Forbidden("You cannot resend this code.", nil)
	}
	if step != string(handover.StepDelivery) {
		return VerificationCodeView{}, apperrors.BadRequest("Invalid verification step.", nil)
	}
	state, err := s.getOwnedBooking(ctx, customerID, id)
	if err != nil {
		return VerificationCodeView{}, err
	}
	if s.trips == nil {
		return VerificationCodeView{}, apperrors.Unavailable("Verification codes are temporarily unavailable.", nil)
	}
	rec, err := s.trips.ResendDeliveryCode(ctx, state.Booking.ID)
	if err != nil {
		return VerificationCodeView{}, err
	}
	remaining := handover.MaxVerifyAttempts - rec.Attempts
	if remaining < 0 {
		remaining = 0
	}
	view := VerificationCodeView{Code: rec.Code, AttemptsRemaining: remaining}
	if !rec.ExpiresAt.IsZero() {
		expires := rec.ExpiresAt
		view.ExpiresAt = &expires
	}
	return view, nil
}

// VerificationCodeView is the customer-facing view of the delivery
// credential — never exposed for the pickup step (the rider's credential is
// not readable by the customer through this surface).
type VerificationCodeView struct {
	Code              string     `json:"code,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	AttemptsRemaining int        `json:"attempts_remaining"`
}

// --- event subscriber handlers ---

// HandleRequestAccepted records the assigned rider and attaches them to the
// wallet hold so trip completion can settle escrow. Partner-created bookings
// have no dispatch_bookings row and no-op here.
func (s *Service) HandleRequestAccepted(ctx context.Context, bookingID, providerID string) error {
	updated, err := s.repository.SetAccepted(ctx, bookingID, providerID)
	if err != nil {
		return err
	}
	if !updated {
		return nil
	}
	state, ok, err := s.repository.GetWithDerivedState(ctx, bookingID)
	if err != nil || !ok {
		return err
	}
	if s.notifier != nil {
		s.notifier.NotifyCustomerAccepted(ctx, state.Booking.CustomerID, bookingID, notifyData(state.Booking))
	}
	// The rider's position is only known now, so this is the first point the
	// approach leg can be routed for the customer's map.
	s.fetchPickupRoute(bookingID, providerID,
		geo.Point{Lat: state.Booking.PickupLat, Lng: state.Booking.PickupLng})
	if s.payments == nil {
		return nil
	}
	if state.Booking.PaymentRef == "" {
		return nil
	}
	if err := s.payments.AssignProvider(ctx, state.Booking.PaymentRef, providerID); err != nil {
		// Settlement is retriable on trip completion; log and continue.
		log.Printf("booking wallet assign-provider failed booking_id=%s provider_id=%s payment_ref=%s: %v",
			bookingID, providerID, state.Booking.PaymentRef, err)
	}
	return nil
}

// HandleNoProviderFound marks the booking unmatched and refunds the hold.
// The transition is a strict CAS from 'searching', so a stray or late event
// can never clobber (or refund) a booking a rider already accepted.
func (s *Service) HandleNoProviderFound(ctx context.Context, bookingID string) error {
	state, ok, err := s.repository.GetWithDerivedState(ctx, bookingID)
	if err != nil || !ok {
		return err
	}
	updated, err := s.repository.SetUnmatched(ctx, bookingID, s.now())
	if err != nil {
		return err
	}
	if !updated {
		// A previous attempt may have committed the unmatched state and then
		// lost the Wallet response. Re-issuing the same deterministic refund
		// key is safe and lets reconciliation complete without a second ledger
		// operation.
		if state.Booking.Status == StatusUnmatched && state.Booking.PaymentStatus == PaymentHeld {
			s.refundHeld(ctx, state, "refund-booking-"+bookingID, "no_provider_found")
		}
		return nil
	}
	if s.notifier != nil {
		s.notifier.NotifyCustomerUnmatched(ctx, state.Booking.CustomerID, bookingID, notifyData(state.Booking))
	}
	s.refundHeld(ctx, state, "refund-booking-"+bookingID, "no_provider_found")
	return nil
}

// HandleTripCompleted projects physical completion separately from Wallet acknowledgement.
func (s *Service) HandleTripCompleted(ctx context.Context, bookingID string, completedAt time.Time) error {
	state, ok, err := s.repository.GetWithDerivedState(ctx, bookingID)
	if err != nil || !ok {
		return err
	}
	paymentStatus := state.Booking.PaymentStatus
	if completedAt.IsZero() {
		completedAt = s.now()
	}
	updated, err := s.repository.SetCompleted(ctx, bookingID, completedAt, paymentStatus)
	if err != nil {
		return err
	}
	if updated && s.notifier != nil {
		s.notifier.NotifyCustomerCompleted(ctx, state.Booking.CustomerID, bookingID, notifyData(state.Booking))
	}
	commerceBooking := false
	{
		if scopes, ok := s.repository.(commerceMissionScopeReader); ok {
			parentID, vendorIDs, commerce, scopeErr := scopes.CommerceMissionScope(ctx, bookingID)
			if scopeErr != nil {
				return scopeErr
			}
			if commerce {
				commerceBooking = true
				if s.commerceSettlement == nil {
					return apperrors.Unavailable("Commerce settlement authority is unavailable.", nil)
				}
				providerID := state.Booking.ProviderID
				if providerID == "" {
					providerID = state.TripProviderID
				}
				if providerID == "" {
					return apperrors.Unavailable("Commerce provider settlement identity is unavailable.", nil)
				}
				for _, vendorID := range vendorIDs {
					if err = s.commerceSettlement.ReleaseCommerceDelivery(ctx, parentID, vendorID, providerID, bookingID); err != nil {
						return err
					}
				}
			}
		}
	}
	if !commerceBooking && paymentStatus == PaymentHeld {
		authority, ok := s.payments.(SettlementPaymentClient)
		if !ok {
			return apperrors.Unavailable("Wallet settlement authority is unavailable.", nil)
		}
		if err = authority.CompleteSettlement(ctx, bookingID); err != nil {
			return apperrors.Unavailable("Wallet settlement confirmation is pending.", err)
		}
		return s.repository.SetPayment(ctx, bookingID, PaymentPaid, state.Booking.PaymentRef)
	}
	return nil
}

// HandleTripCancelled syncs a trip-level cancellation onto the booking. For
// provider cancels from pre-pickup states the trip service fires its own
// auto-refund; we re-issue with the same trip-keyed idempotency key (wallet
// dedupes) so the payment bookkeeping only flips after a confirmed refund.
func (s *Service) HandleTripCancelled(ctx context.Context, bookingID, tripID, cancelledBy, reasonCode string, requiresAdminInvestigation bool, cancelledAt time.Time) error {
	state, ok, err := s.repository.GetWithDerivedState(ctx, bookingID)
	if err != nil || !ok {
		return err
	}
	if cancelledAt.IsZero() {
		cancelledAt = s.now()
	}
	reason := strings.TrimSpace(reasonCode)
	if reason == "" {
		reason = "trip_cancelled"
	}
	if _, err := s.repository.SetTerminal(ctx, bookingID, StatusCancelled, state.Booking.PaymentStatus, reason, cancelledAt); err != nil {
		return err
	}
	if s.notifier != nil && cancelledBy == "provider" {
		s.notifier.NotifyCustomerCancelledByProvider(ctx, state.Booking.CustomerID, bookingID, notifyData(state.Booking))
	}
	if !requiresAdminInvestigation && tripID != "" {
		s.refundHeld(ctx, state, "refund-"+tripID, "trip_cancelled_"+reason)
	}
	return nil
}

// --- helpers ---

func (s *Service) getOwnedBooking(ctx context.Context, customerID, id string) (DerivedState, error) {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return DerivedState{}, validationError("id", "Booking ID must be a valid UUID.")
	}
	state, ok, err := s.repository.GetWithDerivedState(ctx, id)
	if err != nil {
		return DerivedState{}, err
	}
	if !ok || state.Booking.CustomerID != customerID {
		return DerivedState{}, apperrors.NotFound("Delivery booking was not found.", nil)
	}
	return state, nil
}

// refundHeld releases held escrow back to the customer and flips the payment
// bookkeeping only after the wallet confirmed (or deduped) the refund.
func (s *Service) refundHeld(ctx context.Context, state DerivedState, idempotencyKey, reason string) {
	if s.payments == nil || state.Booking.PaymentStatus != PaymentHeld {
		return
	}
	if err := s.payments.Refund(ctx, state.Booking.ID, state.Booking.PaymentRef, state.Booking.FareAmount, state.Booking.Currency, reason, idempotencyKey); err != nil {
		log.Printf("booking wallet refund failed booking_id=%s idempotency_key=%s: %v", state.Booking.ID, idempotencyKey, err)
		return
	}
	if err := s.repository.SetPayment(ctx, state.Booking.ID, PaymentRefunded, state.Booking.PaymentRef); err != nil {
		log.Printf("booking refund bookkeeping failed booking_id=%s: %v", state.Booking.ID, err)
	}
}

// currentPaymentStatus re-reads the payment status so terminal updates written
// after a refund carry the refunded flag rather than the stale held one.
func (s *Service) currentPaymentStatus(ctx context.Context, id, fallback string) string {
	state, ok, err := s.repository.GetWithDerivedState(ctx, id)
	if err != nil || !ok {
		return fallback
	}
	return state.Booking.PaymentStatus
}

// validateCreateInput validates the booking creation payload and returns the
// receiver phone normalized to +234 E.164 form. Normalizing here (rather than
// a bare non-empty check) is what keeps the number in a format the trip
// service's proof-of-delivery match can actually compare against later.
func validateCreateInput(in CreateBookingInput) (string, error) {
	if err := validateCoords(in.Pickup.Lat, in.Pickup.Lng, "pickup"); err != nil {
		return "", err
	}
	if err := validateCoords(in.Dropoff.Lat, in.Dropoff.Lng, "dropoff"); err != nil {
		return "", err
	}
	var fields []apperrors.FieldViolation
	if strings.TrimSpace(in.Pickup.Address) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "pickup.address", Message: "Pickup address is required."})
	}
	if strings.TrimSpace(in.Dropoff.Address) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "dropoff.address", Message: "Dropoff address is required."})
	}
	if strings.TrimSpace(in.ReceiverName) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "receiver_name", Message: "Receiver name is required."})
	}
	normalizedPhone := ""
	if strings.TrimSpace(in.ReceiverPhone) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "receiver_phone", Message: "Receiver phone is required."})
	} else if normalized, err := phonenumber.NormalizeNigerianPhoneNumber(in.ReceiverPhone); err != nil {
		fields = append(fields, apperrors.FieldViolation{Field: "receiver_phone", Message: "Enter a valid Nigerian phone number."})
	} else {
		normalizedPhone = normalized
	}
	if email := strings.TrimSpace(in.ReceiverEmail); email != "" {
		parsed, err := mail.ParseAddress(email)
		if err != nil || parsed.Address != email || len(email) > 254 {
			fields = append(fields, apperrors.FieldViolation{Field: "receiver_email", Message: "Enter a valid receiver email."})
		}
	}
	if account := strings.TrimSpace(in.ReceiverCustomerID); account != "" {
		if _, err := uuid.Parse(account); err != nil {
			fields = append(fields, apperrors.FieldViolation{Field: "receiver_customer_id", Message: "Enter a valid receiver account reference."})
		}
	}
	if strings.TrimSpace(in.PackageDesc) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "package_desc", Message: "Describe the package you are sending."})
	}
	if in.PackageWeight < 0 {
		fields = append(fields, apperrors.FieldViolation{Field: "package_weight", Message: "Package weight cannot be negative."})
	}
	if in.PaymentMethod != "" && !strings.EqualFold(strings.TrimSpace(in.PaymentMethod), PaymentMethodWallet) {
		fields = append(fields, apperrors.FieldViolation{Field: "payment_method", Message: "Only wallet payment is supported right now."})
	}
	if len(fields) > 0 {
		err := apperrors.BadRequest("Check your details.", nil)
		err.Fields = fields
		err.Code = apperrors.CodeValidationFailed
		return "", err
	}
	return normalizedPhone, nil
}

func validateCoords(lat, lng float64, field string) error {
	if lat == 0 && lng == 0 {
		return validationError(field, "Coordinates are required.")
	}
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return validationError(field, "Coordinates are out of range.")
	}
	return nil
}

func validationError(field, message string) error {
	err := apperrors.BadRequest("Check your details.", nil)
	err.Fields = []apperrors.FieldViolation{{Field: field, Message: message}}
	err.Code = apperrors.CodeValidationFailed
	return err
}
