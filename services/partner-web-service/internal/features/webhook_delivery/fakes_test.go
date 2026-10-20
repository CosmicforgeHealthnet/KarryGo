package webhookdelivery

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/hibiken/asynq"
)

// fakeRepo implements Repository for tests.
type fakeRepo struct {
	mu sync.Mutex

	booking         *Booking
	findErr         error
	updateStatusErr error
	statusUpdates   map[string]string

	delivery       *Delivery
	webhook        *Webhook
	getDeliveryErr error
	getWebhookErr  error
	attemptUpdates map[string]int
	deliveryStatus map[string]string

	activeWebhooks    []Webhook
	createdDeliveries []CreateDeliveryParams
	createErr         error
	listErr           error
	seq               int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		statusUpdates:  map[string]string{},
		attemptUpdates: map[string]int{},
		deliveryStatus: map[string]string{},
	}
}

func (r *fakeRepo) FindByDispatchBookingID(_ context.Context, _ string) (*Booking, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	return r.booking, nil
}

func (r *fakeRepo) UpdateBookingStatus(_ context.Context, bookingID, status string) error {
	if r.updateStatusErr != nil {
		return r.updateStatusErr
	}
	r.mu.Lock()
	r.statusUpdates[bookingID] = status
	r.mu.Unlock()
	return nil
}

func (r *fakeRepo) GetDelivery(_ context.Context, _ string) (*Delivery, error) {
	if r.getDeliveryErr != nil {
		return nil, r.getDeliveryErr
	}
	return r.delivery, nil
}

func (r *fakeRepo) GetWebhook(_ context.Context, _ string) (*Webhook, error) {
	if r.getWebhookErr != nil {
		return nil, r.getWebhookErr
	}
	return r.webhook, nil
}

func (r *fakeRepo) UpdateDeliveryAttempt(_ context.Context, deliveryID string, attempts int) error {
	r.mu.Lock()
	r.attemptUpdates[deliveryID] = attempts
	r.mu.Unlock()
	return nil
}

func (r *fakeRepo) UpdateDeliveryStatus(_ context.Context, deliveryID, status string) error {
	r.mu.Lock()
	r.deliveryStatus[deliveryID] = status
	r.mu.Unlock()
	return nil
}

func (r *fakeRepo) CreateWebhookDelivery(_ context.Context, p CreateDeliveryParams) (string, error) {
	if r.createErr != nil {
		return "", r.createErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	r.createdDeliveries = append(r.createdDeliveries, p)
	return "delivery-" + strconv.Itoa(r.seq), nil
}

func (r *fakeRepo) ListActiveWebhooksForPartnerEvent(_ context.Context, _, _ string) ([]Webhook, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.activeWebhooks, nil
}

// fakeWorker implements WebhookWorkerInterface, recording enqueue calls.
type fakeWorker struct {
	calls []enqueueCall
	err   error
}

type enqueueCall struct {
	bookingID string
	partnerID string
	eventType string
	payload   []byte
}

func (w *fakeWorker) EnqueueForBooking(_ context.Context, bookingID, partnerID, eventType string, payload []byte) error {
	w.calls = append(w.calls, enqueueCall{bookingID, partnerID, eventType, payload})
	return w.err
}

// fakeEnqueuer implements Enqueuer, recording tasks + options.
type fakeEnqueuer struct {
	tasks []*asynq.Task
	opts  [][]asynq.Option
	err   error
}

func (e *fakeEnqueuer) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	e.tasks = append(e.tasks, task)
	e.opts = append(e.opts, opts)
	if e.err != nil {
		return nil, e.err
	}
	return &asynq.TaskInfo{ID: "t"}, nil
}

var errBoom = errors.New("boom")
