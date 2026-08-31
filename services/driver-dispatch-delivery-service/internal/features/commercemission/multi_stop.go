package commercemission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercequote"
	requestfeature "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/request"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

type MultiStopMissionStore interface {
	CreateMultiStop(ctx context.Context, input createMultiStopInput) (dispatchcommerce.Mission, error)
	FindMultiStopByOperation(ctx context.Context, caller, operation string) (dispatchcommerce.Mission, string, bool, error)
	FindMultiStopByCanonical(ctx context.Context, caller, canonical string) (dispatchcommerce.Mission, string, bool, error)
}

type QuoteReader interface {
	FindMultiStopByID(ctx context.Context, caller, quoteID string) (commercequote.MultiStopRecord, bool, error)
}

type BroadcastRecoverer interface {
	RecoverBroadcast(ctx context.Context, event requestfeature.BookingDispatchCreatedEvent) (requestfeature.RequestBroadcast, error)
}

// ReceiverCredentialNotifier is the existing Dispatch-owned notification
// policy seam. The implementation submits a typed intent to Notification
// Service; this package never sends an email, push or SMS itself.
type ReceiverCredentialNotifier interface {
	NotifyCustomerDeliveryCodeReady(ctx context.Context, customerID, bookingID string, data map[string]interface{})
}

type MultiStopMissionOptions struct {
	Store     MultiStopMissionStore
	Quotes    QuoteReader
	Broadcast Broadcaster
	Notifier  ReceiverCredentialNotifier
	Now       func() time.Time
}

type MultiStopMissionService struct {
	store          MultiStopMissionStore
	quotes         QuoteReader
	broadcast      Broadcaster
	notifier       ReceiverCredentialNotifier
	now            func() time.Time
	handoverSecret []byte
	media          MediaAttachmentAuthorizer
}

func (s *MultiStopMissionService) WithMediaAttachmentAuthorizer(authorizer MediaAttachmentAuthorizer) *MultiStopMissionService {
	if s != nil {
		s.media = authorizer
	}
	return s
}

func (s *MultiStopMissionService) authorizeEvidence(ctx context.Context, missionRef string, command EvidenceCommand) error {
	if len(command.MediaRefs) == 0 {
		return nil
	}
	if s == nil || s.media == nil {
		return apperrors.New(503, "MEDIA_ATTACHMENT_AUTHORITY_UNAVAILABLE", "Media evidence authorization is unavailable.", nil)
	}
	if err := s.media.AuthorizeEvidence(ctx, missionRef, command.ParcelRefs, command.MediaRefs); err != nil {
		return apperrors.New(409, "MEDIA_ATTACHMENT_SCOPE_MISMATCH", "Media evidence is not authorized for this mission and parcel context.", err)
	}
	return nil
}

func NewMultiStopMission(options MultiStopMissionOptions) *MultiStopMissionService {
	if options.Now == nil {
		options.Now = time.Now
	}
	return &MultiStopMissionService{store: options.Store, quotes: options.Quotes, broadcast: options.Broadcast, notifier: options.Notifier, now: options.Now}
}

func (s *MultiStopMissionService) Create(ctx context.Context, caller string, request dispatchcommerce.MissionCreateRequest) (dispatchcommerce.Mission, error) {
	caller = strings.TrimSpace(caller)
	if caller != dispatchcommerce.SignatureCaller {
		return dispatchcommerce.Mission{}, apperrors.Unauthorized("This caller may not create Commerce missions.", nil)
	}
	if reason, ok := request.Validate(); !ok {
		return dispatchcommerce.Mission{}, apperrors.New(400, "INVALID_MISSION", reason, nil)
	}
	if s == nil || s.store == nil || s.quotes == nil || s.broadcast == nil {
		return dispatchcommerce.Mission{}, apperrors.Unavailable("Commerce mission execution is unavailable.", nil)
	}
	fingerprint, err := missionCreateFingerprint(request)
	if err != nil {
		return dispatchcommerce.Mission{}, apperrors.BadRequest("Mission request is invalid.", err)
	}
	if existing, storedFingerprint, found, findErr := s.store.FindMultiStopByOperation(ctx, caller, request.OperationRef); findErr != nil {
		return dispatchcommerce.Mission{}, findErr
	} else if found {
		if storedFingerprint != fingerprint {
			return dispatchcommerce.Mission{}, apperrors.New(409, "IDEMPOTENCY_CONFLICT", "This operation key was already used for a different mission.", nil)
		}
		existing.Replayed = true
		if err = s.ensureBroadcast(ctx, request, existing, 0, "NGN", true); err != nil {
			return dispatchcommerce.Mission{}, err
		}
		return existing, nil
	}
	quote, found, err := s.quotes.FindMultiStopByID(ctx, caller, request.Quote.QuoteID)
	if err != nil {
		return dispatchcommerce.Mission{}, apperrors.Unavailable("The authoritative quote could not be read.", err)
	}
	if !found {
		return dispatchcommerce.Mission{}, apperrors.New(409, "QUOTE_NOT_FOUND", "The referenced quote does not exist for this caller.", nil)
	}
	if err = verifyMissionQuote(request, quote, s.now().UTC()); err != nil {
		return dispatchcommerce.Mission{}, err
	}
	mission, err := s.store.CreateMultiStop(ctx, createMultiStopInput{Caller: caller, Fingerprint: fingerprint, Request: request, Quote: quote})
	if errors.Is(err, errConcurrentOperation) {
		existing, storedFingerprint, found, readErr := s.store.FindMultiStopByOperation(ctx, caller, request.OperationRef)
		if readErr != nil {
			return dispatchcommerce.Mission{}, readErr
		}
		if !found {
			return dispatchcommerce.Mission{}, err
		}
		if storedFingerprint != fingerprint {
			return dispatchcommerce.Mission{}, apperrors.New(409, "IDEMPOTENCY_CONFLICT", "This operation key was already used for a different mission.", nil)
		}
		existing.Replayed = true
		mission = existing
		err = nil
	}
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if err = s.ensureBroadcast(ctx, request, mission, quote.FeeMinor, quote.Currency, mission.Replayed); err != nil {
		return dispatchcommerce.Mission{}, apperrors.Unavailable("Mission was committed but rider matching is temporarily unavailable; read back this operation before retrying.", err)
	}
	return mission, nil
}

func (s *MultiStopMissionService) ReadOperation(ctx context.Context, caller, operation string) (dispatchcommerce.Mission, error) {
	mission, _, found, err := s.store.FindMultiStopByOperation(ctx, strings.TrimSpace(caller), strings.TrimSpace(operation))
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if !found {
		return dispatchcommerce.Mission{}, apperrors.NotFound("Commerce mission operation not found.", nil)
	}
	return mission, nil
}

func (s *MultiStopMissionService) ReadMission(ctx context.Context, caller, missionRef string) (dispatchcommerce.Mission, error) {
	mission, _, found, err := s.store.FindMultiStopByCanonical(ctx, strings.TrimSpace(caller), strings.TrimSpace(missionRef))
	if err != nil {
		return dispatchcommerce.Mission{}, err
	}
	if !found {
		return dispatchcommerce.Mission{}, apperrors.NotFound("Commerce mission not found.", nil)
	}
	return mission, nil
}

func missionCreateFingerprint(request dispatchcommerce.MissionCreateRequest) (string, error) {
	request.CorrelationID = ""
	return dispatchcommerce.Fingerprint(request)
}

func verifyMissionQuote(request dispatchcommerce.MissionCreateRequest, quote commercequote.MultiStopRecord, now time.Time) error {
	if !quote.Serviceable {
		return apperrors.New(409, "QUOTE_NOT_SERVICEABLE", "The referenced quote is not serviceable.", nil)
	}
	if !quote.ExpiresAt.After(now) {
		return apperrors.New(409, "QUOTE_EXPIRED", "The referenced quote has expired.", nil)
	}
	if request.Quote.QuoteReference != quote.QuoteReference || request.Quote.QuoteRevision != quote.Revision || request.Quote.QuoteFingerprint != quote.RequestFingerprint {
		return apperrors.New(409, "QUOTE_MISMATCH", "The mission does not match the authoritative quote revision.", nil)
	}
	var quoted dispatchcommerce.QuoteRequest
	if err := json.Unmarshal(quote.RequestSnapshot, &quoted); err != nil {
		return apperrors.Unavailable("The authoritative quote evidence is unreadable.", err)
	}
	if quoted.Plan == nil || !reflect.DeepEqual(*quoted.Plan, request.Plan) {
		return apperrors.New(409, "STALE_PLAN", "The immutable plan binding differs from the quoted plan.", nil)
	}
	quotedSemantics := struct {
		Mode         dispatchcommerce.Mode
		ServiceClass string
		PickupStops  []dispatchcommerce.PickupStop
		Receiver     dispatchcommerce.Point
		Parcels      []dispatchcommerce.Parcel
		Vehicle      dispatchcommerce.VehicleRequirement
		PromisedBy   time.Time
	}{quoted.Mode, quoted.ServiceClass, quoted.PickupStops, quoted.Receiver, quoted.Parcels, quoted.Vehicle, quoted.PromisedBy}
	missionSemantics := struct {
		Mode         dispatchcommerce.Mode
		ServiceClass string
		PickupStops  []dispatchcommerce.PickupStop
		Receiver     dispatchcommerce.Point
		Parcels      []dispatchcommerce.Parcel
		Vehicle      dispatchcommerce.VehicleRequirement
		PromisedBy   time.Time
	}{request.Mode, request.ServiceClass, request.PickupStops, request.Receiver, request.Parcels, request.Vehicle, request.PromisedBy}
	if !reflect.DeepEqual(quotedSemantics, missionSemantics) {
		return apperrors.New(409, "QUOTE_MISMATCH", "The mission route, parcels or requirements differ from the quoted request.", nil)
	}
	return nil
}

func (s *MultiStopMissionService) ensureBroadcast(ctx context.Context, request dispatchcommerce.MissionCreateRequest, mission dispatchcommerce.Mission, fee int64, currency string, recover bool) error {
	if mission.BookingRef == "" || mission.State != "SEARCHING" {
		return nil
	}
	if fee == 0 {
		if quote, found, err := s.quotes.FindMultiStopByID(ctx, dispatchcommerce.SignatureCaller, mission.Quote.QuoteID); err == nil && found {
			fee = quote.FeeMinor
			currency = quote.Currency
		}
	}
	ordered := request.PickupStops
	if len(ordered) == 0 {
		// A read-back retry reconstructs the provider-facing event from the
		// immutable mission view rather than trusting a new caller payload.
		return fmt.Errorf("mission broadcast recovery requires immutable request snapshot")
	}
	first := ordered[0]
	event := requestfeature.BookingDispatchCreatedEvent{
		BookingID: mission.BookingRef, CustomerID: request.CustomerRef,
		PickupLat: first.Location.Latitude, PickupLng: first.Location.Longitude,
		DropoffLat: request.Receiver.Latitude, DropoffLng: request.Receiver.Longitude,
		PickupAddress: first.Location.Address, DropoffAddress: request.Receiver.Address,
		FareAmount: fee, ServiceType: "dispatch", PaymentMethod: "wallet", Currency: currency,
		ReceiverName: receiverDisplay(request.Receiver), ReceiverPhone: request.Receiver.Phone,
		PackageDesc:   fmt.Sprintf("Commerce multi-stop mission: %d pickups, %d parcels", len(request.PickupStops), len(request.Parcels)),
		PackageWeight: float64(totalParcelWeight(request.Parcels)) / 1000, PackageType: "commerce",
		PackageSize: "large", IsFragile: hasFragile(request.Parcels), ServiceTier: serviceTier(request.Mode),
		CorrelationID: request.CorrelationID, OccurredAt: s.now().UTC(),
	}
	if recover {
		if recovery, ok := s.broadcast.(BroadcastRecoverer); ok {
			_, err := recovery.RecoverBroadcast(ctx, event)
			return err
		}
	}
	_, err := s.broadcast.StartBroadcast(ctx, event)
	return err
}

func totalParcelWeight(parcels []dispatchcommerce.Parcel) int64 {
	var total int64
	for _, p := range parcels {
		total += p.WeightGrams
	}
	return total
}
