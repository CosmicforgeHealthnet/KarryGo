package booking

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
	"cosmicforge/logistics/shared/go/apperrors"
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

// TripControl cancels an assigned trip on the customer's behalf. Calling the
// trip feature in-process (instead of pub/sub) lets the cancel endpoint verify
// the trip really cancelled before refunding — the trip-side guard no-ops for
// in_progress trips, closing the refund-while-delivering race.
type TripControl interface {
	CancelForCustomer(ctx context.Context, bookingID, reason string) error
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

type Service struct {
	repository Repository
	broadcast  BroadcastControl
	trips      TripControl
	live       LiveStore
	payments   PaymentClient
	events     EventPublisher
	notifier   LifecycleNotifier
	radiusKM   float64
	now        func() time.Time
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
func (s *Service) WithNotifier(notifier LifecycleNotifier) *Service {
	s.notifier = notifier
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
	if err := validateCreateInput(in); err != nil {
		return BookingView{}, err
	}
	distance := haversineKM(in.Pickup.Lat, in.Pickup.Lng, in.Dropoff.Lat, in.Dropoff.Lng)
	fare := calculateFareKobo(distance, in.PackageSize, in.ServiceTier)

	booking := Booking{
		CustomerID:     customerID,
		Status:         StatusSearching,
		PickupAddress:  strings.TrimSpace(in.Pickup.Address),
		PickupLat:      in.Pickup.Lat,
		PickupLng:      in.Pickup.Lng,
		DropoffAddress: strings.TrimSpace(in.Dropoff.Address),
		DropoffLat:     in.Dropoff.Lat,
		DropoffLng:     in.Dropoff.Lng,
		DistanceKM:     roundKM(distance),
		FareAmount:     fare,
		Currency:       currencyNGN,
		PaymentMethod:  PaymentMethodWallet,
		PaymentStatus:  PaymentUnpaid,
		ReceiverName:   strings.TrimSpace(in.ReceiverName),
		ReceiverPhone:  strings.TrimSpace(in.ReceiverPhone),
		PackageDesc:    strings.TrimSpace(in.PackageDesc),
		PackageType:    strings.TrimSpace(in.PackageType),
		PackageSize:    NormalizePackageSize(in.PackageSize),
		IsFragile:      in.IsFragile,
		ServiceTier:    NormalizeServiceTier(in.ServiceTier),
	}
	if in.PackageWeight > 0 {
		weight := in.PackageWeight
		booking.PackageWeight = &weight
	}

	created, err := s.repository.Insert(ctx, booking)
	if err != nil {
		return BookingView{}, err
	}

	// Wallet escrow: hold the fare before matching starts (fail fast).
	if s.payments != nil {
		ref, holdErr := s.payments.Hold(ctx, created.ID, customerID, fare)
		if holdErr != nil {
			now := s.now()
			if _, err := s.repository.SetTerminal(ctx, created.ID, StatusCancelled, PaymentFailed, "payment_failed", now); err != nil {
				log.Printf("booking payment-failed terminal update failed booking_id=%s: %v", created.ID, err)
			}
			return BookingView{}, apperrors.New(http.StatusBadRequest, "payment_failed", holdErr.Error(), holdErr)
		}
		created.PaymentStatus = PaymentHeld
		created.PaymentRef = ref
		if err := s.repository.SetPayment(ctx, created.ID, PaymentHeld, ref); err != nil {
			return BookingView{}, err
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

	return BookingView{Booking: created, Status: WireSearching}, nil
}

func (s *Service) GetBooking(ctx context.Context, customerID, id string) (BookingView, error) {
	state, err := s.getOwnedBooking(ctx, customerID, id)
	if err != nil {
		return BookingView{}, err
	}
	return state.View(), nil
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
	return s.repository.InsertReview(ctx, Review{
		BookingID:          state.Booking.ID,
		ProviderID:         providerID,
		CustomerID:         customerID,
		Rating:             in.Rating,
		ReviewText:         strings.TrimSpace(in.ReviewText),
		RecommendsProvider: in.RecommendsProvider,
	})
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
	providerID := state.TripProviderID
	if providerID == "" {
		providerID = state.Booking.ProviderID
	}
	if providerID == "" {
		return LocationResponse{}, apperrors.NotFound("No rider has been assigned yet.", nil)
	}
	if s.live == nil {
		return LocationResponse{}, apperrors.Unavailable("Live tracking is temporarily unavailable.", nil)
	}
	location, ok, err := s.live.GetLocation(ctx, providerID)
	if err != nil {
		return LocationResponse{}, err
	}
	if !ok {
		return LocationResponse{}, apperrors.NotFound("The rider's location is not available right now.", nil)
	}
	return LocationResponse{
		ProviderID: providerID,
		Lat:        location.Lat,
		Lng:        location.Lng,
		UpdatedAt:  location.UpdatedAt,
	}, nil
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
		return nil
	}
	if s.notifier != nil {
		s.notifier.NotifyCustomerUnmatched(ctx, state.Booking.CustomerID, bookingID, notifyData(state.Booking))
	}
	s.refundHeld(ctx, state, "refund-booking-"+bookingID, "no_provider_found")
	return nil
}

// HandleTripCompleted marks the booking (and its escrow) settled.
func (s *Service) HandleTripCompleted(ctx context.Context, bookingID string, completedAt time.Time) error {
	state, ok, err := s.repository.GetWithDerivedState(ctx, bookingID)
	if err != nil || !ok {
		return err
	}
	paymentStatus := state.Booking.PaymentStatus
	if paymentStatus == PaymentHeld {
		paymentStatus = PaymentPaid
	}
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

func validateCreateInput(in CreateBookingInput) error {
	if err := validateCoords(in.Pickup.Lat, in.Pickup.Lng, "pickup"); err != nil {
		return err
	}
	if err := validateCoords(in.Dropoff.Lat, in.Dropoff.Lng, "dropoff"); err != nil {
		return err
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
	if strings.TrimSpace(in.ReceiverPhone) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "receiver_phone", Message: "Receiver phone is required."})
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
		return err
	}
	return nil
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
