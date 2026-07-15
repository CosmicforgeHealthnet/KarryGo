package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
)

const rfc3339 = time.RFC3339

const (
	defaultDeliveryListLimit = 20
	maxDeliveryListLimit     = 100
)

// Deps are the webhook service collaborators.
type Deps struct {
	Webhooks   WebhookRepository
	Deliveries WebhookDeliveryRepository
	Bookings   BookingLookup
	Cryptor    *Cryptor
	Sender     Sender
	Now        func() time.Time
	NewID      func() string
}

// Service implements partner webhook registration, delivery, and retry.
type Service struct {
	repo       WebhookRepository
	deliveries WebhookDeliveryRepository
	bookings   BookingLookup
	cryptor    *Cryptor
	sender     Sender
	now        func() time.Time
	newID      func() string
}

// NewService builds a webhook Service with default clock/id generators.
func NewService(d Deps) *Service {
	s := &Service{
		repo:       d.Webhooks,
		deliveries: d.Deliveries,
		bookings:   d.Bookings,
		cryptor:    d.Cryptor,
		sender:     d.Sender,
		now:        d.Now,
		newID:      d.NewID,
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.newID == nil {
		s.newID = uuid.NewString
	}
	return s
}

// --- request/response DTOs --------------------------------------------------

// CreateWebhookInput is the POST /webhooks body.
type CreateWebhookInput struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

// CreateWebhookResult is returned once at creation; it includes the plaintext secret.
type CreateWebhookResult struct {
	ID            string   `json:"id"`
	URL           string   `json:"url"`
	Events        []string `json:"events"`
	SigningSecret string   `json:"signing_secret"`
	Status        string   `json:"status"`
}

// WebhookView is the GET /webhooks item shape (never includes the secret).
type WebhookView struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Status    string   `json:"status"`
	CreatedAt string   `json:"created_at"`
}

// DeleteResult is the DELETE /webhooks/:id data.
type DeleteResult struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// WebhookDeliveryView is one row of the GET /webhooks/:id/deliveries response -
// the delivery log/history screen. Note: only the HTTP response STATUS CODE is
// stored (partner_webhook_deliveries has no response body column), not the full
// response body - if the dashboard needs to display a full response body, that
// requires a new migration adding a response body column, which has not been
// done here.
type WebhookDeliveryView struct {
	ID             string          `json:"id"`
	EventType      string          `json:"event_type"`
	Status         string          `json:"status"`
	ResponseStatus *int            `json:"response_status"`
	Attempts       int             `json:"attempts"`
	Payload        json.RawMessage `json:"payload"`
	LastAttemptAt  *string         `json:"last_attempt_at"`
	NextAttemptAt  *string         `json:"next_attempt_at"`
	CreatedAt      string          `json:"created_at"`
}

// WebhookDeliveryDetailView is the detailed view for a single delivery.
type WebhookDeliveryDetailView struct {
	ID             string          `json:"id"`
	WebhookID      string          `json:"webhook_id"`
	EventType      string          `json:"event_type"`
	Status         string          `json:"status"`
	ResponseStatus *int            `json:"response_status"`
	Attempts       int             `json:"attempts"`
	Payload        json.RawMessage `json:"payload"`
	LastAttemptAt  *string         `json:"last_attempt_at"`
	NextAttemptAt  *string         `json:"next_attempt_at"`
	CreatedAt      string          `json:"created_at"`
}

// WebhookDeliveryListResponse is the GET /webhooks/:id/deliveries response data.
type WebhookDeliveryListResponse struct {
	Deliveries []WebhookDeliveryView `json:"deliveries"`
	Meta       delivery.ListMeta     `json:"meta"`
}

// WebhookDeliveryListAllResponse is the GET /webhook-logs response data.
type WebhookDeliveryListAllResponse struct {
	Deliveries []WebhookDeliveryView `json:"deliveries"`
	Meta       delivery.ListMeta     `json:"meta"`
}

// --- registration -----------------------------------------------------------

// CreateWebhook validates and stores a webhook, returning the plaintext signing
// secret exactly once. The stored secret is AES-GCM encrypted, never plaintext.
func (s *Service) CreateWebhook(ctx context.Context, partnerID string, in CreateWebhookInput) (*CreateWebhookResult, error) {
	var fields []delivery.FieldError
	if fe := validateWebhookURL(in.URL); fe != nil {
		fields = append(fields, *fe)
	}
	events, fe := validateAndNormalizeEvents(in.Events)
	if fe != nil {
		fields = append(fields, *fe)
	}
	if len(fields) > 0 {
		return nil, delivery.ValidationError(fields)
	}

	secret, err := generateSigningSecret()
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	encrypted, err := s.cryptor.EncryptSecret(secret)
	if err != nil {
		return nil, delivery.ErrInternal()
	}

	created, err := s.repo.CreateWebhook(ctx, &Webhook{
		ID:                     s.newID(),
		PartnerID:              partnerID,
		URL:                    strings.TrimSpace(in.URL),
		Events:                 events,
		EncryptedSigningSecret: encrypted,
		Status:                 StatusActive,
		CreatedAt:              s.now(),
	})
	if err != nil {
		return nil, delivery.ErrInternal()
	}

	return &CreateWebhookResult{
		ID:            created.ID,
		URL:           created.URL,
		Events:        created.Events,
		SigningSecret: secret, // shown once, never stored/logged in plaintext
		Status:        created.Status,
	}, nil
}

// ListWebhooks returns a partner's webhooks without signing secrets.
func (s *Service) ListWebhooks(ctx context.Context, partnerID string) ([]WebhookView, error) {
	hooks, err := s.repo.ListWebhooks(ctx, partnerID)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	views := make([]WebhookView, 0, len(hooks))
	for _, w := range hooks {
		views = append(views, WebhookView{
			ID:        w.ID,
			URL:       w.URL,
			Events:    w.Events,
			Status:    w.Status,
			CreatedAt: w.CreatedAt.UTC().Format(rfc3339),
		})
	}
	return views, nil
}

// DeleteWebhook soft-deletes a partner's webhook (partner-scoped).
func (s *Service) DeleteWebhook(ctx context.Context, partnerID, webhookID string) (*DeleteResult, error) {
	existing, err := s.repo.FindWebhookByPartner(ctx, partnerID, webhookID)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if existing == nil {
		return nil, delivery.WebhookNotFound()
	}
	if err := s.repo.DeleteWebhook(ctx, partnerID, webhookID); err != nil {
		return nil, delivery.ErrInternal()
	}
	return &DeleteResult{ID: webhookID, Deleted: true}, nil
}

// TestWebhook sends a real signed delivery.test event to one partner webhook and
// returns success only on a 2xx response. It does not persist a delivery row
// (partner_webhook_deliveries requires a booking FK) and does not schedule retries.
func (s *Service) TestWebhook(ctx context.Context, partnerID, webhookID string) error {
	w, err := s.repo.FindWebhookByPartner(ctx, partnerID, webhookID)
	if err != nil {
		return delivery.ErrInternal()
	}
	if w == nil {
		return delivery.WebhookNotFound()
	}
	secret, err := s.cryptor.DecryptSecret(w.EncryptedSigningSecret)
	if err != nil {
		return delivery.ErrInternal()
	}

	now := s.now()
	payload := WebhookPayload{
		Event:      EventDeliveryTest,
		DeliveryID: s.newID(),
		BookingRef: "KGP-TESTONLY",
		Status:     "test",
		OccurredAt: now.UTC().Format(rfc3339),
		Data:       map[string]any{"test": true},
	}
	status, sendErr := s.sender.Send(ctx, *w, WebhookDelivery{ID: payload.DeliveryID, WebhookID: w.ID, EventType: EventDeliveryTest}, secret, payload, now)
	if sendErr != nil {
		return delivery.WebhookDeliveryFailed("The webhook endpoint responded with status " + strconv.Itoa(status) + ".")
	}
	return nil
}

// ListDeliveries returns a page of delivery attempt history (the "webhook
// logs" screen) for one of the partner's own webhooks. Ownership is checked
// first via FindWebhookByPartner - the same guard DeleteWebhook and TestWebhook
// already use - so a partner can never read another partner's delivery log by
// guessing a webhook id.
//
// This data was already being written by ProcessDispatchEvent via
// s.deliveries.CreateDelivery/MarkDelivered/etc.; it was just never exposed for
// reading before this method + the new ListByWebhook repo method existed.
func (s *Service) ListDeliveries(ctx context.Context, partnerID, webhookID string, page, limit int) (*WebhookDeliveryListResponse, error) {
	w, err := s.repo.FindWebhookByPartner(ctx, partnerID, webhookID)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if w == nil {
		return nil, delivery.WebhookNotFound()
	}

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = defaultDeliveryListLimit
	}
	if limit > maxDeliveryListLimit {
		limit = maxDeliveryListLimit
	}
	offset := (page - 1) * limit

	items, total, err := s.deliveries.ListByWebhook(ctx, webhookID, limit, offset)
	if err != nil {
		return nil, delivery.ErrInternal()
	}

	views := make([]WebhookDeliveryView, 0, len(items))
	for i := range items {
		views = append(views, deliveryViewOf(&items[i]))
	}
	return &WebhookDeliveryListResponse{
		Deliveries: views,
		Meta:       delivery.ListMeta{Page: page, Limit: limit, Total: total},
	}, nil
}


// ListAllDeliveries returns a page of delivery attempts for all webhooks
// belonging to a partner. This is used by the /webhook-logs endpoint.
func (s *Service) ListAllDeliveries(ctx context.Context, partnerID string, page, limit int) (*WebhookDeliveryListAllResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = defaultDeliveryListLimit
	}
	if limit > maxDeliveryListLimit {
		limit = maxDeliveryListLimit
	}
	offset := (page - 1) * limit

	items, total, err := s.deliveries.ListByPartner(ctx, partnerID, limit, offset)
	if err != nil {
		// Log the actual error so we can see what's wrong
		log.Printf("ListAllDeliveries error for partner %s: %v", partnerID, err)
		return nil, delivery.ErrInternal()
	}

	views := make([]WebhookDeliveryView, 0, len(items))
	for i := range items {
		views = append(views, deliveryViewOf(&items[i]))
	}
	return &WebhookDeliveryListAllResponse{
		Deliveries: views,
		Meta:       delivery.ListMeta{Page: page, Limit: limit, Total: total},
	}, nil
}


// GetDeliveryByID returns a single delivery attempt by ID, with ownership check.
func (s *Service) GetDeliveryByID(ctx context.Context, partnerID, deliveryID string) (*WebhookDeliveryDetailView, error) {
	item, err := s.deliveries.FindByID(ctx, partnerID, deliveryID)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if item == nil {
		return nil, delivery.WebhookNotFound()
	}

	return &WebhookDeliveryDetailView{
		ID:             item.ID,
		WebhookID:      item.WebhookID,
		EventType:      item.EventType,
		Status:         item.Status,
		ResponseStatus: item.ResponseStatus,
		Attempts:       item.Attempts,
		Payload:        json.RawMessage(item.Payload),
		LastAttemptAt:  formatTimePtr(item.LastAttemptAt),
		NextAttemptAt:  formatTimePtr(item.NextAttemptAt),
		CreatedAt:      item.CreatedAt.UTC().Format(rfc3339),
	}, nil
}

func deliveryViewOf(d *WebhookDelivery) WebhookDeliveryView {
	return WebhookDeliveryView{
		ID:             d.ID,
		EventType:      d.EventType,
		Status:         d.Status,
		ResponseStatus: d.ResponseStatus,
		Attempts:       d.Attempts,
		Payload:        json.RawMessage(d.Payload),
		LastAttemptAt:  formatTimePtr(d.LastAttemptAt),
		NextAttemptAt:  formatTimePtr(d.NextAttemptAt),
		CreatedAt:      d.CreatedAt.UTC().Format(rfc3339),
	}
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(rfc3339)
	return &s
}

// --- dispatch event processing ---------------------------------------------

// dispatchLifecycleEvent is the minimal shape decoded from dispatch topics. Every
// dispatch lifecycle event carries booking_id (= partner dispatch_booking_id).
// Mirrors driver-dispatch-delivery-service request/model.go and trip/model.go.
// trip.started additionally carries the rider/vehicle snapshot fields.
type dispatchLifecycleEvent struct {
	Event       string    `json:"event"`
	BookingID   string    `json:"booking_id"`
	OccurredAt  time.Time `json:"occurred_at"`
	CompletedAt time.Time `json:"completed_at"`
	CancelledAt time.Time `json:"cancelled_at"`
	CancelledBy string    `json:"cancelled_by"`
	Reason      string    `json:"reason"`

	// Rider/vehicle snapshot (trip.started).
	ProviderID   string   `json:"provider_id"`
	RiderName    string   `json:"rider_name"`
	RiderPhone   string   `json:"rider_phone"`
	RiderRating  *float64 `json:"rider_rating"`
	VehicleColor string   `json:"vehicle_color"`
	VehicleModel string   `json:"vehicle_model"`
	PlateNumber  string   `json:"plate_number"`
}

func (ev dispatchLifecycleEvent) riderSnapshot() delivery.RiderSnapshot {
	return delivery.RiderSnapshot{
		RiderID:      strings.TrimSpace(ev.ProviderID),
		RiderName:    strings.TrimSpace(ev.RiderName),
		RiderPhone:   strings.TrimSpace(ev.RiderPhone),
		RiderRating:  ev.RiderRating,
		VehicleColor: strings.TrimSpace(ev.VehicleColor),
		VehicleModel: strings.TrimSpace(ev.VehicleModel),
		PlateNumber:  strings.TrimSpace(ev.PlateNumber),
	}
}

// ProcessDispatchEvent maps a dispatch event to a partner event, updates the
// partner booking status, and fans out webhook deliveries. Malformed or
// unmatched events are ignored (never crash the subscriber).
func (s *Service) ProcessDispatchEvent(ctx context.Context, topic string, raw []byte) error {
	_, err := s.ProcessDispatchEventResult(ctx, topic, raw)
	return err
}

// ProcessDispatchEventResult is ProcessDispatchEvent, but also reports whether a
// partner booking matched (used by the internal HTTP callbacks to answer
// skipped=true for non-partner bookings).
func (s *Service) ProcessDispatchEventResult(ctx context.Context, topic string, raw []byte) (bool, error) {
	mapping, ok := MappingForTopic(topic)
	if !ok {
		return false, nil
	}
	var ev dispatchLifecycleEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		log.Printf("webhooks: dropping malformed %s payload: %v", topic, err)
		return false, nil
	}
	dispatchID := strings.TrimSpace(ev.BookingID)
	if dispatchID == "" {
		log.Printf("webhooks: %s missing booking_id - skipping", topic)
		return false, nil
	}

	booking, err := s.bookings.FindByDispatchBookingID(ctx, dispatchID)
	if err != nil {
		return false, err
	}
	if booking == nil {
		return false, nil // not a partner booking; ignore
	}

	// trip.started carries the rider/vehicle snapshot: store it together with the
	// status transition so GET /deliveries/:id shows the rider immediately.
	if snap := ev.riderSnapshot(); topic == DispatchTopicTripStarted && snap.HasData() {
		if err := s.bookings.UpdateRiderSnapshotAndStatus(ctx, booking.ID, snap, mapping.BookingStatus); err != nil {
			log.Printf("webhooks: failed to store rider snapshot for booking %s: %v", booking.ID, err)
		}
	} else if err := s.bookings.UpdateStatus(ctx, booking.ID, mapping.BookingStatus); err != nil {
		log.Printf("webhooks: failed to update booking %s status: %v", booking.ID, err)
	}

	if mapping.PartnerEvent == "" {
		// Provider-arrived is not a public partner webhook event yet; keep it as
		// a status-only transition until the partner webhook contract adds it.
		return true, nil
	}

	hooks, err := s.repo.FindActiveWebhooksForEvent(ctx, booking.PartnerID, mapping.PartnerEvent)
	if err != nil {
		return true, err
	}

	occurred := ev.OccurredAt
	if occurred.IsZero() {
		occurred = s.now()
	}
	for i := range hooks {
		w := hooks[i]
		payload := buildPayload(booking, mapping, ev, occurred)
		body, err := json.Marshal(payload)
		if err != nil {
			continue
		}
		created, err := s.deliveries.CreateDelivery(ctx, &WebhookDelivery{
			ID:        s.newID(),
			WebhookID: w.ID,
			BookingID: booking.ID,
			EventType: mapping.PartnerEvent,
			Payload:   body,
			Attempts:  0,
			Status:    DeliveryStatusPending,
			CreatedAt: s.now(),
		})
		if err != nil {
			log.Printf("webhooks: failed to record delivery for webhook %s: %v", w.ID, err)
			continue
		}
		_, _ = s.deliverOnce(ctx, w, *created, payload) // immediate attempt; failures scheduled
	}
	return true, nil
}

func buildPayload(b *delivery.PartnerBooking, mapping EventMapping, ev dispatchLifecycleEvent, occurred time.Time) WebhookPayload {
	data := map[string]any{}
	if !ev.CompletedAt.IsZero() {
		data["completed_at"] = ev.CompletedAt.UTC().Format(rfc3339)
	}
	if !ev.CancelledAt.IsZero() {
		data["cancelled_at"] = ev.CancelledAt.UTC().Format(rfc3339)
	}
	if ev.CancelledBy != "" {
		data["cancelled_by"] = ev.CancelledBy
	}
	if ev.Reason != "" {
		data["reason"] = ev.Reason
	}
	if snap := ev.riderSnapshot(); snap.HasData() {
		rider := map[string]any{}
		if snap.RiderID != "" {
			rider["id"] = snap.RiderID
		}
		if snap.RiderName != "" {
			rider["name"] = snap.RiderName
		}
		if snap.RiderPhone != "" {
			rider["phone"] = snap.RiderPhone
		}
		if snap.RiderRating != nil {
			rider["rating"] = *snap.RiderRating
		}
		if snap.VehicleColor != "" {
			rider["vehicle_color"] = snap.VehicleColor
		}
		if snap.VehicleModel != "" {
			rider["vehicle_model"] = snap.VehicleModel
		}
		if snap.PlateNumber != "" {
			rider["plate_number"] = snap.PlateNumber
		}
		data["rider"] = rider
	}
	return WebhookPayload{
		Event:       mapping.PartnerEvent,
		DeliveryID:  b.ID,
		BookingRef:  b.BookingRef,
		ExternalRef: b.ExternalRef,
		Status:      mapping.BookingStatus,
		OccurredAt:  occurred.UTC().Format(rfc3339),
		Data:        data,
	}
}

// --- retry worker -----------------------------------------------------------

// ProcessDueDeliveries attempts all pending deliveries whose next_attempt_at is due.
func (s *Service) ProcessDueDeliveries(ctx context.Context, limit int) error {
	due, err := s.deliveries.FindPendingDueDeliveries(ctx, limit)
	if err != nil {
		return err
	}
	for i := range due {
		d := due[i]
		w, err := s.repo.FindWebhookByID(ctx, d.WebhookID)
		if err != nil || w == nil {
			// Orphaned delivery: stop retrying it.
			_ = s.deliveries.MarkFailedPermanent(ctx, d.ID, nil)
			continue
		}
		var payload WebhookPayload
		if err := json.Unmarshal(d.Payload, &payload); err != nil {
			_ = s.deliveries.MarkFailedPermanent(ctx, d.ID, nil)
			continue
		}
		_, _ = s.deliverOnce(ctx, *w, d, payload)
	}
	return nil
}

// RunRetryWorker polls due deliveries on an interval until ctx is cancelled.
func (s *Service) RunRetryWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.ProcessDueDeliveries(ctx, 50); err != nil {
				log.Printf("webhooks: retry worker error: %v", err)
			}
		}
	}
}

// deliverOnce performs a single delivery attempt and records the outcome:
// delivered on 2xx, otherwise scheduled for retry or marked failed_permanent
// once attempts are exhausted.
func (s *Service) deliverOnce(ctx context.Context, w Webhook, d WebhookDelivery, payload WebhookPayload) (int, error) {
	newAttempts := d.Attempts + 1

	secret, err := s.cryptor.DecryptSecret(w.EncryptedSigningSecret)
	if err != nil {
		// Cannot sign without the secret; do not retry indefinitely.
		_ = s.deliveries.MarkFailedPermanent(ctx, d.ID, nil)
		return 0, err
	}

	status, sendErr := s.sender.Send(ctx, w, d, secret, payload, s.now())
	if sendErr == nil && status >= 200 && status < 300 {
		_ = s.deliveries.MarkDelivered(ctx, d.ID, status)
		return status, nil
	}

	var respStatus *int
	if status > 0 {
		st := status
		respStatus = &st
	}
	if delay, ok := NextRetryDelay(newAttempts); ok {
		next := s.now().Add(delay)
		_ = s.deliveries.MarkFailedAttempt(ctx, d.ID, newAttempts, respStatus, &next)
	} else {
		_ = s.deliveries.MarkFailedPermanent(ctx, d.ID, respStatus)
	}
	if sendErr == nil {
		sendErr = errors.New("webhook endpoint returned non-2xx status")
	}
	return status, sendErr
}