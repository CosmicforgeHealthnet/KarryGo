package trip

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/walletclient"
)

// WalletSettlementClient is the subset of walletclient.Client used by the trip service.
// Defined as an interface so tests can inject a fake without making real HTTP calls.
type WalletSettlementClient interface {
	CompleteJob(ctx context.Context, sourceService, sourceReference string) (walletclient.PaymentIntent, error)
	RequestRefund(ctx context.Context, request walletclient.RefundRequest) (walletclient.Refund, error)
}

// Phase 7F - Mark Arrived
type TripEventPublisher interface {
	PublishTripCreated(ctx context.Context, event TripCreatedEvent) error
	PublishProviderArrived(ctx context.Context, event TripProviderArrivedEvent) error
	PublishTripStarted(ctx context.Context, event TripStartedEvent) error
	PublishProofSubmitted(ctx context.Context, event TripProofSubmittedEvent) error
	PublishTripCompleted(ctx context.Context, event TripCompletedEvent) error
	PublishTripCancelled(ctx context.Context, event TripCancelledEvent) error
	PublishSuspensionFlag(ctx context.Context, event SuspensionFlagEvent) error
	PublishCustomerRated(ctx context.Context, event CustomerRatedEvent) error
}

const MaxReasonTextLength = 500

// ProofSubmitInput carries parsed multipart form data for the SubmitProof service method.
type ProofSubmitInput struct {
	TripID        string
	ProviderID    string
	ReceiverName  string
	ReceiverPhone string
	PhotoFile     multipart.File
	PhotoHeader   *multipart.FileHeader
	SigFile       multipart.File
	SigHeader     *multipart.FileHeader
}

const (
	MaxDeliveryPhotoSize int64 = 5 << 20 // 5 MB
	MaxSignatureFileSize int64 = 3 << 20 // 3 MB
)

type Service struct {
	repository Repository
	storage    ProofStorage
	events     TripEventPublisher
	wallet     WalletSettlementClient
	now        func() time.Time
}

func NewService(repository Repository, storage ProofStorage) *Service {
	return &Service{
		repository: repository,
		storage:    storage,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// WithEventPublisher wires the outbound event publisher (call after NewService).
func (s *Service) WithEventPublisher(publisher TripEventPublisher) *Service {
	s.events = publisher
	return s
}

// WithWalletClient makes the shared wallet client available for Phase 8D/8E.
func (s *Service) WithWalletClient(client WalletSettlementClient) *Service {
	s.wallet = client
	return s
}

func (s *Service) HandleRequestAccepted(ctx context.Context, event RequestAcceptedEvent) (*Trip, error) {
	if err := validateAcceptedEvent(event); err != nil {
		return nil, err
	}
	trip, err := s.repository.CreateTripFromAcceptedRequest(ctx, CreateTripInput{
		BookingID: event.BookingID, ProviderID: event.ProviderID, CustomerID: event.CustomerID,
		PickupAddress: event.PickupAddress, PickupLat: event.PickupLat, PickupLng: event.PickupLng,
		DropoffAddress: event.DropoffAddress, DropoffLat: event.DropoffLat, DropoffLng: event.DropoffLng,
		DistanceKM: event.DistanceKM, FareAmount: event.FareAmount, Currency: event.Currency,
		ReceiverName: event.ReceiverName, ReceiverPhone: event.ReceiverPhone,
		PackageDesc: event.PackageDesc, PackageWeight: event.PackageWeight,
		PackageType: event.PackageType, PackageSize: event.PackageSize, IsFragile: event.IsFragile,
		ServiceTier: normalizeServiceTier(event.ServiceTier),
	})
	if err != nil || trip == nil {
		return trip, err
	}
	// Publish trip.created (Phase 7M). Consumers should deduplicate on trip_id.
	if s.events != nil {
		_ = s.events.PublishTripCreated(ctx, TripCreatedEvent{
			Event: TopicTripCreated, CorrelationID: event.CorrelationID,
			TripID: trip.ID, BookingID: trip.BookingID,
			ProviderID: trip.ProviderID, CustomerID: trip.CustomerID,
			Status: string(trip.Status), PackageType: trip.PackageType, PackageSize: trip.PackageSize,
			IsFragile: trip.IsFragile, ServiceTier: trip.ServiceTier,
			CreatedAt: trip.CreatedAt, OccurredAt: s.now(),
		})
	}
	return trip, nil
}

// HandleBookingDispatchCancelled cancels eligible trips when the customer cancels their booking (Phase 7L).
// Valid for trips in assigned/en_route_pickup/arrived_pickup state.
// Trips in in_progress/proof_submitted/completed/failed/cancelled state are no-ops.
func (s *Service) HandleBookingDispatchCancelled(ctx context.Context, event BookingDispatchCancelledEvent) error {
	bookingID := strings.TrimSpace(event.BookingID)
	if _, err := uuid.Parse(bookingID); err != nil {
		log.Printf("trip customer cancel: invalid booking_id=%q - skipping", bookingID)
		return nil
	}
	trip, err := s.repository.GetTripByBookingID(ctx, bookingID)
	if err != nil {
		return err
	}
	if trip == nil {
		return nil // no trip - pre-acceptance; Phase 6 request feature handled it
	}
	switch trip.Status {
	case StatusAssigned, StatusEnRoutePickup, StatusArrivedPickup:
		// eligible for customer cancellation
	default:
		return nil // in_progress/proof_submitted/completed/failed/cancelled - no-op
	}
	now := s.now()
	reasonText := strings.TrimSpace(event.Reason)
	updatedTrip, _, err := s.repository.CancelTripByCustomerTx(ctx, CustomerCancelTripInput{
		TripID: trip.ID, FromStatus: trip.Status, ReasonText: reasonText, Now: now,
	})
	if err != nil {
		if appErr := apperrors.From(err); appErr != nil && appErr.Code == apperrors.CodeConflict {
			log.Printf("trip customer cancel: already cancelled trip_id=%s booking_id=%s", trip.ID, bookingID)
			return nil
		}
		return err
	}
	if updatedTrip == nil {
		return nil // race condition - status changed between fetch and update
	}
	cancelledAt := now
	if updatedTrip.CancelledAt != nil {
		cancelledAt = *updatedTrip.CancelledAt
	}
	if s.events != nil {
		_ = s.events.PublishTripCancelled(ctx, TripCancelledEvent{
			Event: TopicTripCancelled, CorrelationID: event.CorrelationID,
			TripID: trip.ID, BookingID: trip.BookingID,
			ProviderID: trip.ProviderID, CustomerID: trip.CustomerID,
			CancelledBy: CancelledByCustomer, ReasonCode: "customer_cancelled", ReasonText: reasonText,
			PenaltyApplied: false, RequiresAdminInvestigation: false,
			CancelledAt: cancelledAt, OccurredAt: now,
		})
	}
	return nil
}

func (s *Service) HandleProviderLocationUpdated(ctx context.Context, event ProviderLocationUpdatedEvent) error {
	if err := validateID(event.ProviderID, "provider_id"); err != nil {
		return err
	}
	assigned, err := s.repository.GetAssignedTripForProvider(ctx, event.ProviderID)
	if err != nil || assigned == nil {
		return err
	}
	if err := ValidateTransition(assigned.Status, StatusEnRoutePickup); err != nil {
		return err
	}
	_, err = s.repository.TransitionTripStatus(ctx, TransitionTripInput{
		TripID: assigned.ID, FromStatus: StatusAssigned, ToStatus: StatusEnRoutePickup,
		ChangedBy: CancelledBySystem, Notes: "auto_started_from_location_update", ChangedAt: s.now(),
	})
	return err
}

func (s *Service) ListProviderTrips(ctx context.Context, providerID string, options ListTripsOptions) (TripListResponse, error) {
	if err := validateID(providerID, "provider_id"); err != nil {
		return TripListResponse{}, err
	}
	if options.Status != "" && !IsValidTripStatus(options.Status) {
		return TripListResponse{}, validationError("status", "Status is invalid.")
	}
	if options.Limit <= 0 || options.Limit > 50 {
		return TripListResponse{}, validationError("limit", "Limit must be between 1 and 50.")
	}
	if options.Offset < 0 {
		return TripListResponse{}, validationError("page", "Page must be 1 or greater.")
	}
	trips, total, err := s.repository.ListProviderTrips(ctx, providerID, options)
	if err != nil {
		return TripListResponse{}, err
	}
	items := make([]TripListItem, 0, len(trips))
	for _, trip := range trips {
		items = append(items, TripListItem{
			ID: trip.ID, BookingID: trip.BookingID, Status: trip.Status,
			PickupAddress: trip.PickupAddress, DropoffAddress: trip.DropoffAddress,
			DistanceKM: trip.DistanceKM, FareAmount: trip.FareAmount, Currency: trip.Currency,
			CompletedAt: trip.CompletedAt, CreatedAt: trip.CreatedAt,
		})
	}
	return TripListResponse{
		Trips: items, Total: total, Page: options.Offset/options.Limit + 1, Limit: options.Limit,
	}, nil
}

func (s *Service) GetProviderActiveTrip(ctx context.Context, providerID string) (*Trip, error) {
	if err := validateID(providerID, "provider_id"); err != nil {
		return nil, err
	}
	trip, err := s.repository.GetProviderActiveTrip(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if trip == nil {
		return nil, apperrors.NotFound("Active trip not found.", nil)
	}
	return trip, nil
}

func (s *Service) GetProviderTrip(ctx context.Context, tripID, providerID string) (*Trip, error) {
	if err := validateID(tripID, "id"); err != nil {
		return nil, err
	}
	if err := validateID(providerID, "provider_id"); err != nil {
		return nil, err
	}
	trip, err := s.repository.GetProviderTripByID(ctx, tripID, providerID)
	if err != nil {
		return nil, err
	}
	if trip == nil {
		return nil, apperrors.NotFound("Trip not found.", nil)
	}
	return trip, nil
}

func (s *Service) GetProviderTripDetail(ctx context.Context, tripID, providerID string) (*TripDetailResponse, error) {
	trip, err := s.GetProviderTrip(ctx, tripID, providerID)
	if err != nil {
		return nil, err
	}
	logs, err := s.repository.ListTripStateLog(ctx, tripID)
	if err != nil {
		return nil, err
	}
	proof, err := s.repository.GetDeliveryProofByTripID(ctx, tripID)
	if err != nil {
		return nil, err
	}
	return &TripDetailResponse{Trip: *trip, StateLog: logs, Proof: proof}, nil
}

func (s *Service) GetProviderProof(ctx context.Context, tripID, providerID string) (*DeliveryProof, error) {
	if _, err := s.GetProviderTrip(ctx, tripID, providerID); err != nil {
		return nil, err
	}
	proof, err := s.repository.GetDeliveryProofByTripID(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if proof == nil {
		return nil, apperrors.NotFound("Delivery proof not found.", nil)
	}
	return proof, nil
}

func (s *Service) FoundationOperation(ctx context.Context, tripID, providerID string) error {
	if _, err := s.GetProviderTrip(ctx, tripID, providerID); err != nil {
		return err
	}
	return apperrors.New(http.StatusNotImplemented, "not_implemented", "This trip action is not available in Phase 7A-7B.", nil)
}

// Phase 7F - Mark Arrived

// Phase 7F - Mark Arrived
func (s *Service) MarkArrived(ctx context.Context, tripID, providerID string) (ArrivedResponse, error) {
	if err := validateID(tripID, "id"); err != nil {
		return ArrivedResponse{}, err
	}
	trip, err := s.repository.GetProviderTripByID(ctx, tripID, providerID)
	if err != nil {
		return ArrivedResponse{}, err
	}
	if trip == nil {
		return ArrivedResponse{}, apperrors.NotFound("Trip not found.", nil)
	}
	if err := ValidateTransition(trip.Status, StatusArrivedPickup); err != nil {
		return ArrivedResponse{}, err
	}
	now := s.now()
	updated, err := s.repository.MarkArrived(ctx, tripID, providerID, trip.Status, now)
	if err != nil {
		return ArrivedResponse{}, err
	}
	if updated == nil {
		return ArrivedResponse{}, apperrors.Conflict("Trip status has already changed.", nil)
	}
	arrivedAt := now
	if updated.ArrivedAt != nil {
		arrivedAt = *updated.ArrivedAt
	}
	if s.events != nil {
		_ = s.events.PublishProviderArrived(ctx, TripProviderArrivedEvent{
			Event: TopicTripProviderArrived, TripID: trip.ID, BookingID: trip.BookingID,
			ProviderID: trip.ProviderID, CustomerID: trip.CustomerID,
			PickupAddress: trip.PickupAddress, PickupLat: trip.PickupLat, PickupLng: trip.PickupLng,
			ArrivedAt: arrivedAt, OccurredAt: now,
		})
	}
	return ArrivedResponse{
		ID: trip.ID, BookingID: trip.BookingID, Status: StatusArrivedPickup,
		ArrivedAt: arrivedAt, Message: "Customer has been notified you have arrived.",
	}, nil
}

// Phase 7G - Start Delivery

// Phase 7G - Start Delivery
func (s *Service) StartTrip(ctx context.Context, tripID, providerID string) (StartTripResponse, error) {
	if err := validateID(tripID, "id"); err != nil {
		return StartTripResponse{}, err
	}
	trip, err := s.repository.GetProviderTripByID(ctx, tripID, providerID)
	if err != nil {
		return StartTripResponse{}, err
	}
	if trip == nil {
		return StartTripResponse{}, apperrors.NotFound("Trip not found.", nil)
	}
	if err := ValidateTransition(trip.Status, StatusInProgress); err != nil {
		return StartTripResponse{}, err
	}
	now := s.now()
	updated, err := s.repository.MarkTripStarted(ctx, tripID, providerID, now)
	if err != nil {
		return StartTripResponse{}, err
	}
	if updated == nil {
		return StartTripResponse{}, apperrors.Conflict("Trip status has already changed.", nil)
	}
	startedAt := now
	if updated.StartedAt != nil {
		startedAt = *updated.StartedAt
	}
	if s.events != nil {
		_ = s.events.PublishTripStarted(ctx, TripStartedEvent{
			Event: TopicTripStarted, TripID: trip.ID, BookingID: trip.BookingID,
			ProviderID: trip.ProviderID, CustomerID: trip.CustomerID,
			PickupLat: trip.PickupLat, PickupLng: trip.PickupLng,
			DropoffAddress: trip.DropoffAddress, DropoffLat: trip.DropoffLat, DropoffLng: trip.DropoffLng,
			StartedAt: startedAt, OccurredAt: now,
		})
	}
	return StartTripResponse{
		ID: trip.ID, BookingID: trip.BookingID, Status: StatusInProgress,
		StartedAt: startedAt, DropoffAddress: trip.DropoffAddress,
		DropoffLat: trip.DropoffLat, DropoffLng: trip.DropoffLng,
	}, nil
}

// Phase 7H - Submit Proof of Delivery

// Phase 7H - Submit Proof of Delivery
func (s *Service) SubmitProof(ctx context.Context, input ProofSubmitInput) (*ProofSubmitResponse, error) {
	if err := validateID(input.TripID, "id"); err != nil {
		return nil, err
	}
	trip, err := s.repository.GetProviderTripByID(ctx, input.TripID, input.ProviderID)
	if err != nil {
		return nil, err
	}
	if trip == nil {
		return nil, apperrors.NotFound("Trip not found.", nil)
	}
	if err := ValidateTransition(trip.Status, StatusProofSubmitted); err != nil {
		return nil, err
	}
	// Idempotency guard: check for existing proof before any file I/O.
	existing, err := s.repository.GetDeliveryProofByTripID(ctx, input.TripID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, apperrors.Conflict("Proof has already been submitted for this trip.", nil)
	}

	// Validate file presence.
	if input.PhotoHeader == nil {
		return nil, validationError("delivery_photo", "Delivery photo is required.")
	}
	if input.SigHeader == nil {
		return nil, validationError("signature", "Signature is required.")
	}
	// Validate file sizes.
	if input.PhotoHeader.Size > MaxDeliveryPhotoSize {
		return nil, validationError("delivery_photo", "Delivery photo exceeds the 5 MB limit.")
	}
	if input.SigHeader.Size > MaxSignatureFileSize {
		return nil, validationError("signature", "Signature exceeds the 3 MB limit.")
	}
	// Validate MIME type from header.
	if err := validateProofMIME(input.PhotoHeader, "delivery_photo"); err != nil {
		return nil, err
	}
	if err := validateProofMIME(input.SigHeader, "signature"); err != nil {
		return nil, err
	}
	// Validate magic bytes (prevents disguised non-image files).
	if err := validateProofMagicBytes(input.PhotoFile, "delivery_photo"); err != nil {
		return nil, err
	}
	if err := validateProofMagicBytes(input.SigFile, "signature"); err != nil {
		return nil, err
	}
	// Validate receiver fields.
	receiverName := strings.TrimSpace(input.ReceiverName)
	if receiverName == "" {
		return nil, validationError("receiver_name", "Receiver name is required.")
	}
	if err := validateE164Phone(input.ReceiverPhone, "receiver_phone"); err != nil {
		return nil, err
	}
	if normalizePhone(input.ReceiverPhone) != normalizePhone(trip.ReceiverPhone) {
		return nil, validationError("receiver_phone", "Phone number does not match the expected receiver.")
	}

	// Storage must be configured.
	if s.storage == nil {
		return nil, apperrors.Unavailable("Proof storage is not configured.", nil)
	}
	photoRef, err := s.storage.SaveProofFile(ctx, input.ProviderID, input.TripID, input.PhotoFile, input.PhotoHeader, "photo")
	if err != nil {
		return nil, apperrors.Internal("Failed to save delivery photo.", err)
	}
	sigRef, err := s.storage.SaveProofFile(ctx, input.ProviderID, input.TripID, input.SigFile, input.SigHeader, "signature")
	if err != nil {
		// TODO: best-effort cleanup of photoRef if storage supports it.
		return nil, apperrors.Internal("Failed to save signature.", err)
	}

	// Atomic DB transaction: insert proof + update trip + insert state log.
	normalizedPhone := normalizePhone(input.ReceiverPhone)
	now := s.now()
	proof, err := s.repository.SubmitProofTx(ctx, SubmitProofDBInput{
		TripID: input.TripID, ProviderID: input.ProviderID,
		PhotoRef: photoRef, SignatureRef: sigRef,
		ReceiverName: receiverName, ReceiverPhone: normalizedPhone,
		Now: now,
	})
	if err != nil {
		return nil, err
	}

	if s.events != nil {
		_ = s.events.PublishProofSubmitted(ctx, TripProofSubmittedEvent{
			Event: TopicTripProofSubmitted, TripID: trip.ID, BookingID: trip.BookingID,
			ProviderID: trip.ProviderID, CustomerID: trip.CustomerID,
			PhotoRef: photoRef, SignatureRef: sigRef,
			ReceiverName: receiverName, ReceiverPhone: normalizedPhone,
			SubmittedAt: proof.SubmittedAt, OccurredAt: now,
		})
	}
	return &ProofSubmitResponse{
		TripID: trip.ID, PhotoRef: photoRef, SignatureRef: sigRef,
		ReceiverName: receiverName, ReceiverPhone: normalizedPhone,
		SubmittedAt: proof.SubmittedAt,
		Message:     "Proof submitted. You can now complete the delivery.",
	}, nil
}

// Phase 7J - Complete Trip

// Phase 7J - Complete Trip
// Returns 400 PROOF_REQUIRED when called from in_progress or when the proof row is missing.
func (s *Service) CompleteTrip(ctx context.Context, tripID, providerID string) (CompleteResponse, error) {
	if err := validateID(tripID, "id"); err != nil {
		return CompleteResponse{}, err
	}
	trip, err := s.repository.GetProviderTripByID(ctx, tripID, providerID)
	if err != nil {
		return CompleteResponse{}, err
	}
	if trip == nil {
		return CompleteResponse{}, apperrors.NotFound("Trip not found.", nil)
	}
	// Special case: in_progress means proof hasn't been submitted yet.
	if trip.Status == StatusInProgress {
		return CompleteResponse{}, apperrors.New(http.StatusBadRequest, "PROOF_REQUIRED",
			"Submit proof of delivery before completing this trip.", nil)
	}
	if err := ValidateTransition(trip.Status, StatusCompleted); err != nil {
		return CompleteResponse{}, err
	}
	// Belt-and-suspenders: verify the proof row exists before completing.
	proof, err := s.repository.GetDeliveryProofByTripID(ctx, tripID)
	if err != nil {
		return CompleteResponse{}, err
	}
	if proof == nil {
		return CompleteResponse{}, apperrors.New(http.StatusBadRequest, "PROOF_REQUIRED",
			"Proof of delivery is missing. Cannot complete trip.", nil)
	}
	now := s.now()
	updatedTrip, _, err := s.repository.CompleteTripTx(ctx, CompleteTripInput{
		TripID: tripID, ProviderID: providerID, Now: now,
	})
	if err != nil {
		return CompleteResponse{}, err
	}
	if updatedTrip == nil {
		return CompleteResponse{}, apperrors.Conflict("Trip status has already changed.", nil)
	}
	completedAt := now
	if updatedTrip.CompletedAt != nil {
		completedAt = *updatedTrip.CompletedAt
	}
	if s.wallet != nil {
		if _, err := s.wallet.CompleteJob(ctx, "dispatch-delivery-service", trip.BookingID); err != nil {
			log.Printf("wallet settlement failed trip_id=%s booking_id=%s provider_id=%s: %v",
				trip.ID, trip.BookingID, trip.ProviderID, err)
			// Trip is already completed in the DB. Do not roll back.
		}
	}
	if s.events != nil {
		_ = s.events.PublishTripCompleted(ctx, TripCompletedEvent{
			Event: TopicTripCompleted, TripID: trip.ID, BookingID: trip.BookingID,
			ProviderID: trip.ProviderID, CustomerID: trip.CustomerID,
			FareAmount: trip.FareAmount, Currency: trip.Currency,
			CompletedAt: completedAt, OccurredAt: now,
		})
	}
	return CompleteResponse{
		ID: trip.ID, BookingID: trip.BookingID, Status: StatusCompleted,
		FareAmount: trip.FareAmount, Currency: trip.Currency,
		CompletedAt: completedAt,
		Message:     "Delivery completed. Your earnings have been updated.",
	}, nil
}

// shouldAutoRefundTrip returns true when a cancelled trip is eligible for an
// automatic customer refund. Only pre-pickup statuses qualify; in_progress
// requires admin review and must never auto-refund.
func shouldAutoRefundTrip(status TripStatus) bool {
	switch status {
	case StatusAssigned, StatusEnRoutePickup, StatusArrivedPickup:
		return true
	default:
		return false
	}
}

// Phase 7K - Cancel Trip

// Phase 7K - Cancel Trip
func (s *Service) CancelTrip(ctx context.Context, tripID, providerID string, req CancelRequest) (CancelResponse, error) {
	// Validate reason_code first (before hitting the DB).
	if strings.TrimSpace(req.ReasonCode) == "" {
		return CancelResponse{}, validationError("reason_code", "Reason code is required.")
	}
	if _, ok := ValidCancelReasons[req.ReasonCode]; !ok {
		return CancelResponse{}, validationError("reason_code", "Invalid reason code.")
	}
	reasonText := strings.TrimSpace(req.ReasonText)
	if len(reasonText) > MaxReasonTextLength {
		return CancelResponse{}, validationError("reason_text",
			fmt.Sprintf("Reason text must not exceed %d characters.", MaxReasonTextLength))
	}
	if err := validateID(tripID, "id"); err != nil {
		return CancelResponse{}, err
	}
	trip, err := s.repository.GetProviderTripByID(ctx, tripID, providerID)
	if err != nil {
		return CancelResponse{}, err
	}
	if trip == nil {
		return CancelResponse{}, apperrors.NotFound("Trip not found.", nil)
	}
	if !CanProviderCancel(trip.Status) {
		return CancelResponse{}, apperrors.Conflict(
			fmt.Sprintf("Cannot cancel a trip in %s state.", trip.Status), nil)
	}

	// Capture the status before any DB mutation (fake repo shares the pointer).
	fromStatus := trip.Status

	// Count existing provider cancellations in the past 30 days.
	prevCount, err := s.repository.CountProviderCancellationsLast30Days(ctx, providerID)
	if err != nil {
		return CancelResponse{}, err
	}
	totalCount := prevCount + 1 // including this one

	// Determine penalty rules.
	penaltyApplied := false
	requiresAdminInvestigation := false
	if fromStatus == StatusInProgress {
		penaltyApplied = true
		requiresAdminInvestigation = true
	} else if totalCount >= 3 {
		penaltyApplied = true
	}

	now := s.now()
	updatedTrip, _, err := s.repository.CancelTripTx(ctx, CancelTripInput{
		TripID: tripID, ProviderID: providerID, FromStatus: fromStatus,
		ReasonCode: req.ReasonCode, ReasonText: reasonText,
		PenaltyApplied: penaltyApplied, Now: now,
	})
	if err != nil {
		return CancelResponse{}, err
	}
	if updatedTrip == nil {
		return CancelResponse{}, apperrors.Conflict("Trip cannot be cancelled in its current state.", nil)
	}
	cancelledAt := now
	if updatedTrip.CancelledAt != nil {
		cancelledAt = *updatedTrip.CancelledAt
	}

	if s.wallet != nil && shouldAutoRefundTrip(fromStatus) {
		tripID := trip.ID
		bookingID := trip.BookingID
		fareAmount := trip.FareAmount
		currency := trip.Currency
		idemKey := "refund-" + trip.ID
		reason := "trip_cancelled_" + req.ReasonCode
		go func() {
			refundCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := s.wallet.RequestRefund(refundCtx, walletclient.RefundRequest{
				PaymentReference: bookingID,
				SourceService:    "dispatch-delivery-service",
				SourceReference:  bookingID,
				AmountKobo:       fareAmount,
				Currency:         currency,
				Reason:           reason,
				IdempotencyKey:   idemKey,
			}); err != nil {
				log.Printf("wallet refund failed trip_id=%s booking_id=%s idempotency_key=%s: %v",
					tripID, bookingID, idemKey, err)
			}
		}()
	}

	if s.events != nil {
		_ = s.events.PublishTripCancelled(ctx, TripCancelledEvent{
			Event: TopicTripCancelled, TripID: trip.ID, BookingID: trip.BookingID,
			ProviderID: trip.ProviderID, CustomerID: trip.CustomerID,
			CancelledBy: CancelledByProvider, ReasonCode: req.ReasonCode, ReasonText: reasonText,
			PenaltyApplied: penaltyApplied, RequiresAdminInvestigation: requiresAdminInvestigation,
			CancelledAt: cancelledAt, OccurredAt: now,
		})
		// Publish suspension flag only for excessive non-in_progress cancellations.
		if !requiresAdminInvestigation && totalCount >= 3 {
			_ = s.events.PublishSuspensionFlag(ctx, SuspensionFlagEvent{
				Event: TopicVerificationSuspension, ProviderID: providerID,
				Reason: "excessive_cancellations", Count30Days: totalCount, OccurredAt: now,
			})
		}
	}

	var warning string
	if !penaltyApplied && !requiresAdminInvestigation && totalCount >= 1 {
		warning = fmt.Sprintf("This is cancellation %d of 3 in the past 30 days.", totalCount)
	}
	return CancelResponse{
		ID: trip.ID, BookingID: trip.BookingID, Status: StatusCancelled,
		ReasonCode: req.ReasonCode, CancelledAt: cancelledAt,
		PenaltyApplied: penaltyApplied, RequiresAdminInvestigation: requiresAdminInvestigation,
		Warning: warning,
	}, nil
}

// File validation helpers

func validateProofMIME(header *multipart.FileHeader, fieldName string) error {
	ct := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if ct != "image/jpeg" && ct != "image/png" {
		return validationError(fieldName, "File must be JPEG or PNG.")
	}
	return nil
}

func validateProofMagicBytes(file multipart.File, fieldName string) error {
	buf := make([]byte, 4)
	n, err := io.ReadFull(file, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return validationError(fieldName, "Cannot read file content.")
	}
	// Seek back so storage can read from the start.
	if seeker, ok := file.(io.Seeker); ok {
		_, _ = seeker.Seek(0, io.SeekStart)
	}
	if n < 3 {
		return validationError(fieldName, "File is too small to be a valid image.")
	}
	// JPEG: FF D8 FF
	if buf[0] == 0xFF && buf[1] == 0xD8 && buf[2] == 0xFF {
		return nil
	}
	// PNG: 89 50 4E 47
	if n >= 4 && buf[0] == 0x89 && buf[1] == 0x50 && buf[2] == 0x4E && buf[3] == 0x47 {
		return nil
	}
	return validationError(fieldName, "File must be a valid JPEG or PNG image.")
}

func validateE164Phone(phone, field string) error {
	normalized := normalizePhone(phone)
	if normalized == "" {
		return validationError(field, "Phone number is required.")
	}
	if !strings.HasPrefix(normalized, "+") {
		return validationError(field, "Phone must be in E.164 format (e.g., +2348011223344).")
	}
	digits := normalized[1:]
	if len(digits) < 7 || len(digits) > 15 {
		return validationError(field, "Phone number length is invalid.")
	}
	for _, ch := range digits {
		if ch < '0' || ch > '9' {
			return validationError(field, "Phone number contains invalid characters.")
		}
	}
	return nil
}

func normalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	return phone
}

func validateAcceptedEvent(event RequestAcceptedEvent) error {
	for field, value := range map[string]string{
		"booking_id": event.BookingID, "provider_id": event.ProviderID,
	} {
		if err := validateID(value, field); err != nil {
			return err
		}
	}
	if strings.TrimSpace(event.CustomerID) != "" {
		if err := validateID(event.CustomerID, "customer_id"); err != nil {
			return err
		}
	}
	for field, value := range map[string]string{
		"pickup_address": event.PickupAddress, "dropoff_address": event.DropoffAddress,
		"receiver_name": event.ReceiverName, "receiver_phone": event.ReceiverPhone,
	} {
		if strings.TrimSpace(value) == "" {
			return validationError(field, "This field is required.")
		}
	}
	if event.FareAmount <= 0 {
		return validationError("fare_amount", "Must be greater than zero.")
	}
	for field, coordinate := range map[string]float64{
		"pickup_lat": event.PickupLat, "pickup_lng": event.PickupLng,
		"dropoff_lat": event.DropoffLat, "dropoff_lng": event.DropoffLng,
	} {
		if coordinate == 0 || math.IsNaN(coordinate) || math.IsInf(coordinate, 0) {
			return validationError(field, "This field is required.")
		}
	}
	if event.PickupLat < -90 || event.PickupLat > 90 || event.DropoffLat < -90 || event.DropoffLat > 90 {
		return validationError("coordinates", "Latitude is invalid.")
	}
	if event.PickupLng < -180 || event.PickupLng > 180 || event.DropoffLng < -180 || event.DropoffLng > 180 {
		return validationError("coordinates", "Longitude is invalid.")
	}
	return nil
}

func validateID(value, field string) error {
	if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
		return validationError(field, "Must be a valid UUID.")
	}
	return nil
}

func validationError(field, message string) error {
	err := apperrors.BadRequest("Check your details.", nil)
	err.Code = apperrors.CodeValidationFailed
	err.Fields = []apperrors.FieldViolation{{Field: field, Message: message}}
	return err
}

func normalizeServiceTier(tier string) string {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "express":
		return "express"
	case "standard":
		return "standard"
	default:
		return "standard"
	}
}

// RateCustomer allows a provider to rate a customer after a completed trip.
// Returns apperrors.NotFound if the trip does not belong to the provider.
// Returns apperrors.Conflict if the trip status is not completed, or if a rating already exists.
func (s *Service) RateCustomer(ctx context.Context, tripID, providerID string, input RateCustomerInput) (CustomerRating, error) {
	if err := validateID(tripID, "id"); err != nil {
		return CustomerRating{}, err
	}
	if err := validateID(providerID, "provider_id"); err != nil {
		return CustomerRating{}, err
	}

	if input.Score < 1 || input.Score > 5 {
		return CustomerRating{}, validationError("score", "Score must be between 1 and 5.")
	}
	comment := input.Comment
	if comment != nil {
		trimmed := strings.TrimSpace(*comment)
		if len(trimmed) > MaxReasonTextLength {
			return CustomerRating{}, validationError("comment", fmt.Sprintf("Comment must not exceed %d characters.", MaxReasonTextLength))
		}
		if trimmed == "" {
			comment = nil
		} else {
			comment = &trimmed
		}
	}

	trip, err := s.repository.GetProviderTripByID(ctx, tripID, providerID)
	if err != nil {
		return CustomerRating{}, err
	}
	if trip == nil {
		return CustomerRating{}, apperrors.NotFound("Trip was not found.", nil)
	}
	if trip.Status != StatusCompleted {
		return CustomerRating{}, apperrors.Conflict("Customer can only be rated after the trip is completed.", nil)
	}

	rating, inserted, err := s.repository.InsertCustomerRating(ctx, tripID, providerID, trip.CustomerID, input.Score, comment)
	if err != nil {
		return CustomerRating{}, err
	}
	if !inserted {
		return CustomerRating{}, apperrors.Conflict("You have already rated this customer.", nil)
	}

	if s.events != nil {
		_ = s.events.PublishCustomerRated(ctx, CustomerRatedEvent{
			Event:      TopicCustomerRated,
			RatingID:   rating.ID,
			TripID:     rating.TripID,
			ProviderID: rating.ProviderID,
			CustomerID: rating.CustomerID,
			Score:      rating.Score,
			Comment:    rating.Comment,
			CreatedAt:  rating.CreatedAt,
			OccurredAt: s.now(),
		})
	}

	return rating, nil
}
