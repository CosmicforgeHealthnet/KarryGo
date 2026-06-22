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

type BookingService struct {
	bookings      bookingrepo.BookingRepository
	availability  availabilityrepositories.AvailabilityStore
	notifier      *bookingclients.BookingNotifier
	matchTimeout  time.Duration
	activeMatches sync.Map // map[bookingID]context.CancelFunc
}

func NewBookingService(
	bookings bookingrepo.BookingRepository,
	availability availabilityrepositories.AvailabilityStore,
	notifier *bookingclients.BookingNotifier,
	matchTimeoutSeconds int,
) *BookingService {
	return &BookingService{
		bookings:     bookings,
		availability: availability,
		notifier:     notifier,
		matchTimeout: time.Duration(matchTimeoutSeconds) * time.Second,
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
	if input.CargoWeightKg <= 0 {
		fields = append(fields, apperrors.FieldViolation{Field: "cargo_weight_kg", Message: "Cargo weight must be greater than zero."})
	}
	if len(fields) > 0 {
		return bookingmodels.PublicBooking{}, apperrors.Validation("Please check your booking details.", fields)
	}

	count, err := s.availability.CountOnline(ctx)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if count == 0 {
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
		Status:             bookingmodels.StatusPendingMatch,
		ScheduledAt:        input.ScheduledAt,
	}

	created, err := s.bookings.Create(ctx, booking)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}

	_ = s.addEvent(ctx, created.ID, "booking_created", bookingmodels.ActorCustomer, input.CustomerID)
	s.startMatchGoroutine(created.ID, input.PickupLat, input.PickupLng)
	return created.Public(), nil
}

// ─── Matching Engine ──────────────────────────────────────────────────────────

// startMatchGoroutine cancels any existing match goroutine for the booking
// then starts a fresh one with a new cancellable context.
func (s *BookingService) startMatchGoroutine(bookingID string, pickupLat, pickupLng float64) {
	if old, ok := s.activeMatches.LoadAndDelete(bookingID); ok {
		old.(context.CancelFunc)()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.activeMatches.Store(bookingID, cancel)
	go func() {
		defer s.activeMatches.CompareAndDelete(bookingID, cancel)
		s.matchBooking(ctx, bookingID, pickupLat, pickupLng)
	}()
}

// cancelMatch stops the active matching goroutine for a booking, if any.
func (s *BookingService) cancelMatch(bookingID string) {
	if cancel, ok := s.activeMatches.LoadAndDelete(bookingID); ok {
		cancel.(context.CancelFunc)()
	}
}

func (s *BookingService) matchBooking(ctx context.Context, bookingID string, pickupLat, pickupLng float64) {
	// cleanupCtx is used for operations that must complete even after ctx is cancelled
	// (releasing locks, recording events). We don't want those to fail because the
	// parent context was already done.
	cleanupCtx := context.Background()

	providers, err := s.availability.GetOnlineProviders(ctx)
	if err != nil || len(providers) == 0 {
		if ctx.Err() == nil {
			_, _ = s.bookings.UpdateStatus(cleanupCtx, bookingID, bookingmodels.StatusUnmatched)
		}
		return
	}

	sort.Slice(providers, func(i, j int) bool {
		di := haversineKm(pickupLat, pickupLng, providers[i].Lat, providers[i].Lng)
		dj := haversineKm(pickupLat, pickupLng, providers[j].Lat, providers[j].Lng)
		return di < dj
	})

	for _, p := range providers {
		// Check cancellation before attempting to lock a new provider.
		select {
		case <-ctx.Done():
			return
		default:
		}

		acquired, err := s.availability.AcquireMatchLock(ctx, p.ProviderID, bookingID, s.matchTimeout)
		if err != nil || !acquired {
			continue
		}

		_, err = s.bookings.MarkMatched(ctx, bookingID, p.ProviderID, p.TruckID)
		if err != nil {
			_ = s.availability.ReleaseMatchLock(cleanupCtx, p.ProviderID)
			continue
		}
		_ = s.addEvent(cleanupCtx, bookingID, "provider_matched", bookingmodels.ActorSystem, p.ProviderID)
		s.notifier.NotifyProviderMatched(cleanupCtx, p.ProviderID, bookingID, map[string]interface{}{"booking_id": bookingID})

		// Wait for the provider to accept, or exit early on cancellation.
		select {
		case <-time.After(s.matchTimeout):
		case <-ctx.Done():
			_ = s.availability.ReleaseMatchLock(cleanupCtx, p.ProviderID)
			return
		}

		booking, err := s.bookings.GetByID(cleanupCtx, bookingID)
		_ = s.availability.ReleaseMatchLock(cleanupCtx, p.ProviderID)

		if err != nil {
			return
		}
		if booking.Status == bookingmodels.StatusAccepted ||
			booking.Status == bookingmodels.StatusCancelled ||
			booking.Status == bookingmodels.StatusCompleted {
			return
		}

		// Provider did not accept — reset and try the next closest provider.
		_, _ = s.bookings.ResetToMatching(cleanupCtx, bookingID)
		_ = s.addEvent(cleanupCtx, bookingID, "match_timeout", bookingmodels.ActorSystem, p.ProviderID)
	}

	if ctx.Err() == nil {
		_, _ = s.bookings.UpdateStatus(cleanupCtx, bookingID, bookingmodels.StatusUnmatched)
		_ = s.addEvent(cleanupCtx, bookingID, "booking_unmatched", bookingmodels.ActorSystem, "system")
		if b, err := s.bookings.GetByID(cleanupCtx, bookingID); err == nil {
			s.notifier.NotifyCustomerUnmatched(cleanupCtx, b.CustomerID, bookingID, map[string]interface{}{"booking_id": bookingID})
		}
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

func (s *BookingService) AcceptBooking(ctx context.Context, id, providerID string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return bookingmodels.PublicBooking{}, apperrors.Forbidden("You cannot accept this booking.", nil)
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

	_ = s.availability.ReleaseMatchLock(ctx, providerID)
	updated, err := s.bookings.ResetToMatching(ctx, id)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	_ = s.addEvent(ctx, id, "booking_rejected", bookingmodels.ActorProvider, providerID)
	s.startMatchGoroutine(id, b.PickupLat, b.PickupLng)
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
	s.notifier.NotifyCustomerCancelledByProvider(ctx, b.CustomerID, id, map[string]interface{}{"booking_id": id})
	return b.Public(), nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

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
