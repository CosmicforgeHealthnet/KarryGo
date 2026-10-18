package webhooks

import (
	"context"
	"sync"
	"time"

	"cosmicforge/logistics/services/partner-web-service/internal/features/partner/delivery"
)

// fakeWebhookRepo is an in-memory WebhookRepository.
type fakeWebhookRepo struct {
	mu    sync.Mutex
	hooks map[string]*Webhook // by id
}

func newFakeWebhookRepo() *fakeWebhookRepo { return &fakeWebhookRepo{hooks: map[string]*Webhook{}} }

func (r *fakeWebhookRepo) CreateWebhook(_ context.Context, w *Webhook) (*Webhook, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *w
	r.hooks[w.ID] = &cp
	return w, nil
}

func (r *fakeWebhookRepo) ListWebhooks(_ context.Context, partnerID string) ([]Webhook, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Webhook
	for _, w := range r.hooks {
		if w.PartnerID == partnerID && w.Status == StatusActive {
			out = append(out, *w)
		}
	}
	return out, nil
}

func (r *fakeWebhookRepo) DeleteWebhook(_ context.Context, partnerID, webhookID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w, ok := r.hooks[webhookID]; ok && w.PartnerID == partnerID {
		w.Status = StatusInactive
	}
	return nil
}

func (r *fakeWebhookRepo) FindWebhookByPartner(_ context.Context, partnerID, webhookID string) (*Webhook, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w, ok := r.hooks[webhookID]; ok && w.PartnerID == partnerID {
		cp := *w
		return &cp, nil
	}
	return nil, nil
}

func (r *fakeWebhookRepo) FindWebhookByID(_ context.Context, webhookID string) (*Webhook, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if w, ok := r.hooks[webhookID]; ok {
		cp := *w
		return &cp, nil
	}
	return nil, nil
}

func (r *fakeWebhookRepo) FindActiveWebhooksForEvent(_ context.Context, partnerID, eventType string) ([]Webhook, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Webhook
	for _, w := range r.hooks {
		if w.PartnerID != partnerID || w.Status != StatusActive {
			continue
		}
		for _, e := range w.Events {
			if e == eventType {
				out = append(out, *w)
				break
			}
		}
	}
	return out, nil
}

// fakeDeliveryRepo is an in-memory WebhookDeliveryRepository.
type fakeDeliveryRepo struct {
	mu              sync.Mutex
	created         []*WebhookDelivery
	delivered       map[string]int
	failedAttempt   map[string]int
	failedAttemptTo map[string]*time.Time
	failedPermanent map[string]bool
	due             []WebhookDelivery
}

func newFakeDeliveryRepo() *fakeDeliveryRepo {
	return &fakeDeliveryRepo{
		delivered:       map[string]int{},
		failedAttempt:   map[string]int{},
		failedAttemptTo: map[string]*time.Time{},
		failedPermanent: map[string]bool{},
	}
}

func (r *fakeDeliveryRepo) CreateDelivery(_ context.Context, d *WebhookDelivery) (*WebhookDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *d
	r.created = append(r.created, &cp)
	return d, nil
}

func (r *fakeDeliveryRepo) MarkDelivered(_ context.Context, deliveryID string, responseStatus int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.delivered[deliveryID] = responseStatus
	return nil
}

func (r *fakeDeliveryRepo) MarkFailedAttempt(_ context.Context, deliveryID string, attempts int, responseStatus *int, nextAttemptAt *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failedAttempt[deliveryID] = attempts
	r.failedAttemptTo[deliveryID] = nextAttemptAt
	return nil
}

func (r *fakeDeliveryRepo) MarkFailedPermanent(_ context.Context, deliveryID string, responseStatus *int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failedPermanent[deliveryID] = true
	return nil
}

func (r *fakeDeliveryRepo) FindPendingDueDeliveries(_ context.Context, limit int) ([]WebhookDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.due, nil
}

func (r *fakeDeliveryRepo) ListByWebhook(_ context.Context, webhookID string, limit, offset int) ([]WebhookDelivery, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []WebhookDelivery
	for _, d := range r.created {
		if d.WebhookID == webhookID {
			all = append(all, *d)
		}
	}
	total := len(all)
	if offset > total {
		return []WebhookDelivery{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (r *fakeDeliveryRepo) ListByPartner(_ context.Context, _ string, limit, offset int) ([]WebhookDelivery, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	total := len(r.created)

	if offset >= total {
		return []WebhookDelivery{}, total, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	out := make([]WebhookDelivery, 0, end-offset)
	for _, delivery := range r.created[offset:end] {
		out = append(out, *delivery)
	}

	return out, total, nil
}
func (r *fakeDeliveryRepo) FindByID(_ context.Context, _ string, deliveryID string) (*WebhookDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, delivery := range r.created {
		if delivery.ID == deliveryID {
			copy := *delivery
			return &copy, nil
		}
	}

	return nil, nil
}

// fakeBookingLookup is an in-memory BookingLookup.
type fakeBookingLookup struct {
	mu            sync.Mutex
	byDispatchID  map[string]*delivery.PartnerBooking
	statusUpdates map[string]string
	riderUpdates  map[string]delivery.RiderSnapshot
}

func newFakeBookingLookup() *fakeBookingLookup {
	return &fakeBookingLookup{
		byDispatchID:  map[string]*delivery.PartnerBooking{},
		statusUpdates: map[string]string{},
		riderUpdates:  map[string]delivery.RiderSnapshot{},
	}
}

func (l *fakeBookingLookup) FindByDispatchBookingID(_ context.Context, dispatchBookingID string) (*delivery.PartnerBooking, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if b, ok := l.byDispatchID[dispatchBookingID]; ok {
		cp := *b
		return &cp, nil
	}
	return nil, nil
}

func (l *fakeBookingLookup) UpdateStatus(_ context.Context, bookingID, status string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.statusUpdates[bookingID] = status
	return nil
}

func (l *fakeBookingLookup) UpdateRiderSnapshotAndStatus(_ context.Context, bookingID string, snap delivery.RiderSnapshot, status string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.statusUpdates[bookingID] = status
	l.riderUpdates[bookingID] = snap
	return nil
}

// fakeSender is an in-memory Sender.
type fakeSender struct {
	mu          sync.Mutex
	status      int
	err         error
	calls       int
	lastSecret  string
	lastPayload WebhookPayload
}

func (s *fakeSender) Send(_ context.Context, _ Webhook, _ WebhookDelivery, plaintextSecret string, payload WebhookPayload, _ time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.lastSecret = plaintextSecret
	s.lastPayload = payload
	return s.status, s.err
}

func testCryptor(t interface{ Fatalf(string, ...any) }) *Cryptor {
	c, err := NewCryptor("unit-test-key-material")
	if err != nil {
		t.Fatalf("NewCryptor() error = %v", err)
	}
	return c
}
