package delivery

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	quoteTTL        = 5 * time.Minute
	idempotencyTTL  = 24 * time.Hour
	defaultPage     = 1
	defaultLimit    = 20
	maxLimit        = 100
	bookingRefChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// Deps are the collaborators a Service needs. Now/NewID/NewBookingRef are
// optional and default to production implementations; tests may override them.
type Deps struct {
	Bookings      BookingRepository
	Idempotency   IdempotencyStore
	Wallet        WalletClient
	Publisher     Publisher
	Now           func() time.Time
	NewID         func() string
	NewBookingRef func() string
}

// Service implements the Partner delivery/quote workflow.
type Service struct {
	bookings      BookingRepository
	idem          IdempotencyStore
	wallet        WalletClient
	publisher     Publisher
	now           func() time.Time
	newID         func() string
	newBookingRef func() string
}

// NewService builds a Service, filling in default clock/id/ref generators.
func NewService(d Deps) *Service {
	s := &Service{
		bookings:      d.Bookings,
		idem:          d.Idempotency,
		wallet:        d.Wallet,
		publisher:     d.Publisher,
		now:           d.Now,
		newID:         d.NewID,
		newBookingRef: d.NewBookingRef,
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.newID == nil {
		s.newID = uuid.NewString
	}
	if s.newBookingRef == nil {
		s.newBookingRef = generateBookingRef
	}
	return s
}

// GetQuote computes a deterministic fare/ETA. It performs no DB write, no
// payment-wallet call, and no event publish.
func (s *Service) GetQuote(_ context.Context, req QuoteRequest) (QuoteResponse, error) {
	if errs := validateQuote(req); len(errs) > 0 {
		return QuoteResponse{}, newValidationError(errs)
	}
	dist := haversineKM(*req.PickupLat, *req.PickupLng, *req.DestLat, *req.DestLng)
	tier := normalizeTier(req.ServiceTier)
	fare := calculateFareKobo(dist, req.PackageSize, tier)
	return QuoteResponse{
		DistanceKM:   roundKM(dist),
		EstimatedETA: estimateETA(dist),
		Fare:         Money{Kobo: fare, Display: formatNaira(fare)},
		ServiceTier:  tier,
		ExpiresAt:    s.now().Add(quoteTTL).UTC().Format(rfc3339),
	}, nil
}

// CreateDelivery runs the full create flow and returns the HTTP status and raw
// JSON body to send. On a cached idempotent replay it returns the stored body.
func (s *Service) CreateDelivery(ctx context.Context, partnerID string, req DeliveryRequest) (int, []byte, error) {
	if errs := validateDelivery(req); len(errs) > 0 {
		return 0, nil, newValidationError(errs)
	}
	tier := normalizeTier(req.ServiceTier)

	// Idempotency check.
	key := partnerID + ":" + strings.TrimSpace(req.IdempotencyKey)
	hash := requestHashHex(req)
	if existing, err := s.idem.Get(ctx, key); err == nil && existing != nil && existing.ExpiresAt.After(s.now()) {
		if existing.RequestHash == hash {
			return existing.StatusCode, existing.Response, nil
		}
		return 0, nil, errIdempotencyConflict()
	}

	// Fare + dispatch id (generated before the payment intent, per Option A).
	dist := haversineKM(*req.PickupLat, *req.PickupLng, *req.DestLat, *req.DestLng)
	fare := calculateFareKobo(dist, req.PackageSize, tier)
	dispatchID := s.newID()

	booking, err := s.insertPending(ctx, partnerID, req, tier, fare, dispatchID)
	if err != nil {
		return 0, nil, ErrInternal()
	}

	// Payment intent bound to the DISPATCH source so dispatch settlement/refund
	// (CompleteJob / RequestRefund on "dispatch-delivery-service") keeps working.
	intent, err := s.wallet.CreatePaymentIntent(ctx, PaymentIntentInput{
		SourceService:   dispatchSourceService,
		SourceReference: dispatchID,
		CustomerID:      req.CustomerID,
		CustomerEmail:   req.CustomerEmail,
		AmountKobo:      fare,
		Currency:        currencyNGN,
		PaymentMethod:   req.PaymentMethod,
		IdempotencyKey:  "intent-" + booking.ID,
	})
	if err != nil {
		_ = s.bookings.UpdateStatus(ctx, booking.ID, StatusFailed)
		return 0, nil, errPaymentFailed()
	}

	if req.PaymentMethod == PaymentMethodWallet {
		if err := s.wallet.PayFromWallet(ctx, intent.ID, "pay-"+booking.ID); err != nil {
			_ = s.bookings.UpdateStatus(ctx, booking.ID, StatusFailed)
			return 0, nil, errPaymentFailed()
		}
	}

	// Publish dispatch event only after payment is secured.
	payload, err := buildDispatchCreatedPayload(booking, dispatchID, req.CustomerID, req.PaymentMethod, s.now())
	if err != nil {
		_ = s.bookings.UpdateStatus(ctx, booking.ID, StatusFailed)
		return 0, nil, ErrInternal()
	}
	if err := s.publisher.Publish(ctx, TopicBookingDispatchCreated, payload); err != nil {
		_ = s.bookings.UpdateStatus(ctx, booking.ID, StatusFailed)
		return 0, nil, errDispatchUnavailable() // do NOT store idempotency success
	}

	// Promote to dispatching and record payment reference + publish time.
	now := s.now()
	paymentRef := intent.Reference
	if err := s.bookings.UpdatePaymentAndDispatching(ctx, booking.ID, paymentRef, dispatchID, now); err != nil {
		return 0, nil, ErrInternal()
	}
	booking.Status = StatusDispatching
	booking.PaymentIntentID = &paymentRef
	booking.EventPublishedAt = &now

	body, err := json.Marshal(envelope{Success: true, Data: summaryOf(booking)})
	if err != nil {
		return 0, nil, ErrInternal()
	}

	// Store the successful 201 for idempotent replay (best-effort).
	_ = s.idem.Put(ctx, IdempotencyRecord{
		Key:         key,
		PartnerID:   partnerID,
		RequestHash: hash,
		Response:    body,
		StatusCode:  201,
		ExpiresAt:   s.now().Add(idempotencyTTL),
	})

	return 201, body, nil
}

// GetDelivery returns a delivery by UUID or booking_ref, scoped to the partner.
func (s *Service) GetDelivery(ctx context.Context, partnerID, idOrRef string) (DeliveryDetail, error) {
	b, err := s.bookings.FindByPartnerAndIDOrRef(ctx, partnerID, strings.TrimSpace(idOrRef))
	if err != nil {
		return DeliveryDetail{}, ErrInternal()
	}
	if b == nil {
		return DeliveryDetail{}, errBookingNotFound()
	}
	return detailOf(b), nil
}

// ListDeliveries returns a partner's deliveries with pagination and optional status filter.
func (s *Service) ListDeliveries(ctx context.Context, partnerID string, page, limit int, status string) (ListResponse, error) {
	if s := strings.TrimSpace(status); s != "" && !isValidStatus(s) {
		return ListResponse{}, newValidationError([]FieldError{{Field: "status", Message: "Unknown delivery status filter."}})
	}
	if page < 1 {
		page = defaultPage
	}
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	filter := ListFilter{
		Status: strings.ToLower(strings.TrimSpace(status)),
		Page:   page,
		Limit:  limit,
		Offset: (page - 1) * limit,
	}
	items, total, err := s.bookings.ListByPartner(ctx, partnerID, filter)
	if err != nil {
		return ListResponse{}, ErrInternal()
	}
	summaries := make([]DeliverySummary, 0, len(items))
	for i := range items {
		summaries = append(summaries, summaryOf(&items[i]))
	}
	return ListResponse{
		Deliveries: summaries,
		Meta:       ListMeta{Page: page, Limit: limit, Total: total},
	}, nil
}

// CancelDelivery cancels a delivery when its status allows, refunding held
// payment and publishing booking.dispatch.cancelled.
func (s *Service) CancelDelivery(ctx context.Context, partnerID, idOrRef string) (CancelResponse, error) {
	b, err := s.bookings.FindByPartnerAndIDOrRef(ctx, partnerID, strings.TrimSpace(idOrRef))
	if err != nil {
		return CancelResponse{}, ErrInternal()
	}
	if b == nil {
		return CancelResponse{}, errBookingNotFound()
	}
	if !canCancel(b.Status) {
		return CancelResponse{}, errCannotCancel()
	}

	// Refund first: if a refund is required and fails, do NOT publish/cancel.
	if b.PaymentIntentID != nil && strings.TrimSpace(*b.PaymentIntentID) != "" {
		if err := s.wallet.RequestRefund(ctx, RefundInput{
			PaymentReference: *b.PaymentIntentID,
			SourceService:    dispatchSourceService,
			SourceReference:  deref(b.DispatchBookingID),
			AmountKobo:       b.FareAmountKobo,
			Currency:         currencyNGN,
			Reason:           "partner_cancelled",
			IdempotencyKey:   "refund-" + b.ID,
		}); err != nil {
			return CancelResponse{}, errPaymentFailed()
		}
	}

	payload, err := buildDispatchCancelledPayload(deref(b.DispatchBookingID), "partner_cancelled", s.now())
	if err != nil {
		return CancelResponse{}, ErrInternal()
	}
	if err := s.publisher.Publish(ctx, TopicBookingDispatchCancelled, payload); err != nil {
		return CancelResponse{}, errDispatchUnavailable() // do not mark cancelled
	}

	if err := s.bookings.UpdateStatus(ctx, b.ID, StatusCancelled); err != nil {
		return CancelResponse{}, ErrInternal()
	}
	return CancelResponse{ID: b.ID, BookingRef: b.BookingRef, Status: StatusCancelled, Cancelled: true}, nil
}

// dispatchSourceService is the payment-wallet source_service used for partner
// deliveries (Option A) so dispatch trip completion/cancellation settles the
// same payment intent it created for its own booking id.
const dispatchSourceService = "dispatch-delivery-service"

type envelope struct {
	Success bool `json:"success"`
	Data    any  `json:"data"`
}

func (s *Service) insertPending(ctx context.Context, partnerID string, req DeliveryRequest, tier string, fare int64, dispatchID string) (*PartnerBooking, error) {
	now := s.now()
	b := &PartnerBooking{
		ID:                s.newID(),
		PartnerID:         partnerID,
		ExternalRef:       optionalStr(req.ExternalRef),
		PickupAddress:     strings.TrimSpace(req.PickupAddress),
		PickupLat:         *req.PickupLat,
		PickupLng:         *req.PickupLng,
		DestAddress:       strings.TrimSpace(req.DestAddress),
		DestLat:           *req.DestLat,
		DestLng:           *req.DestLng,
		ReceiverName:      strings.TrimSpace(req.ReceiverName),
		ReceiverPhone:     strings.TrimSpace(req.ReceiverPhone),
		PackageType:       optionalStr(req.PackageType),
		PackageSize:       optionalStr(req.PackageSize),
		IsFragile:         req.IsFragile,
		ServiceTier:       tier,
		FareAmountKobo:    fare,
		Currency:          currencyNGN,
		Status:            StatusPending,
		DispatchBookingID: &dispatchID,
		IdempotencyKey:    strings.TrimSpace(req.IdempotencyKey),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	for attempt := 0; attempt < 5; attempt++ {
		b.BookingRef = s.newBookingRef()
		err := s.bookings.CreatePendingBooking(ctx, b)
		if err == nil {
			return b, nil
		}
		if errors.Is(err, ErrDuplicateBookingRef) {
			continue
		}
		return nil, err
	}
	return nil, errors.New("could not generate a unique booking ref")
}

func optionalStr(s string) *string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// generateBookingRef returns a reference like "KGP-A1B2C3D4".
func generateBookingRef() string {
	const n = 8
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		// rand.Read failure is effectively impossible; fall back to uuid chars.
		return "KGP-" + strings.ToUpper(uuid.NewString()[:n])
	}
	out := make([]byte, n)
	for i := range buf {
		out[i] = bookingRefChars[int(buf[i])%len(bookingRefChars)]
	}
	return "KGP-" + string(out)
}
