package webhookdelivery

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

func taskFor(t *testing.T, p DeliverWebhookPayload) *asynq.Task {
	t.Helper()
	b, _ := json.Marshal(p)
	return asynq.NewTask(TaskDeliverWebhook, b)
}

func TestHandleDeliverWebhookSuccess(t *testing.T) {
	const secret = "sign-secret"
	var gotSig, gotEvent, gotDelivery, gotTS, gotCT string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get("X-KarryGo-Signature")
		gotEvent = r.Header.Get("X-KarryGo-Event")
		gotDelivery = r.Header.Get("X-KarryGo-Delivery")
		gotTS = r.Header.Get("X-KarryGo-Timestamp")
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := newFakeRepo()
	repo.delivery = &Delivery{ID: "d1", WebhookID: "w1", PartnerID: "p1", EventType: "delivery.completed",
		Payload: []byte(`{"booking_ref":"KGP-AAAA1111"}`), Status: StatusPending, Attempts: 0}
	repo.webhook = &Webhook{ID: "w1", PartnerID: "p1", URL: srv.URL, SigningSecret: secret, Status: "active"}
	w := NewWorker(repo, &fakeEnqueuer{}, 5, 0)

	if err := w.HandleDeliverWebhook(context.Background(), taskFor(t, DeliverWebhookPayload{DeliveryID: "d1", WebhookID: "w1", EventType: "delivery.completed"})); err != nil {
		t.Fatalf("HandleDeliverWebhook error = %v", err)
	}
	if repo.deliveryStatus["d1"] != StatusDelivered {
		t.Fatalf("status = %q, want delivered", repo.deliveryStatus["d1"])
	}
	if repo.attemptUpdates["d1"] != 1 {
		t.Fatalf("attempts = %d, want 1", repo.attemptUpdates["d1"])
	}
	if gotCT != "application/json" || gotEvent != "delivery.completed" || gotDelivery != "d1" || gotTS == "" {
		t.Fatalf("headers wrong: ct=%q event=%q delivery=%q ts=%q", gotCT, gotEvent, gotDelivery, gotTS)
	}
	// signature = sha256=HMAC(timestamp + "." + body, secret)
	want := "sha256=" + signPayload(gotBody, gotTS, secret)
	if gotSig != want {
		t.Fatalf("signature = %q, want %q", gotSig, want)
	}
}

func TestHandleDeliverWebhookNon2xxSchedulesRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	repo := newFakeRepo()
	repo.delivery = &Delivery{ID: "d1", WebhookID: "w1", Payload: []byte(`{}`), Status: StatusPending, Attempts: 0}
	repo.webhook = &Webhook{ID: "w1", URL: srv.URL, SigningSecret: "s", Status: "active"}
	enq := &fakeEnqueuer{}
	w := NewWorker(repo, enq, 5, 0)

	if err := w.HandleDeliverWebhook(context.Background(), taskFor(t, DeliverWebhookPayload{DeliveryID: "d1", WebhookID: "w1"})); err != nil {
		t.Fatalf("error = %v", err)
	}
	if repo.attemptUpdates["d1"] != 1 {
		t.Fatalf("attempts = %d, want 1", repo.attemptUpdates["d1"])
	}
	if repo.deliveryStatus["d1"] == StatusDelivered {
		t.Fatal("must not be delivered on 500")
	}
	if len(enq.tasks) != 1 {
		t.Fatalf("retry tasks = %d, want 1", len(enq.tasks))
	}
}

func TestHandleDeliverWebhookNetworkErrorSchedulesRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing listening now

	repo := newFakeRepo()
	repo.delivery = &Delivery{ID: "d1", WebhookID: "w1", Payload: []byte(`{}`), Status: StatusPending, Attempts: 0}
	repo.webhook = &Webhook{ID: "w1", URL: url, SigningSecret: "s", Status: "active"}
	enq := &fakeEnqueuer{}
	w := NewWorker(repo, enq, 5, time.Second)

	if err := w.HandleDeliverWebhook(context.Background(), taskFor(t, DeliverWebhookPayload{DeliveryID: "d1", WebhookID: "w1"})); err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(enq.tasks) != 1 {
		t.Fatalf("retry tasks = %d, want 1 on network error", len(enq.tasks))
	}
}

func TestHandleDeliverWebhookMaxRetriesFailedPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	repo := newFakeRepo()
	// Attempts=4 -> this attempt is the 5th -> no more retries.
	repo.delivery = &Delivery{ID: "d1", WebhookID: "w1", Payload: []byte(`{}`), Status: StatusPending, Attempts: 4}
	repo.webhook = &Webhook{ID: "w1", URL: srv.URL, SigningSecret: "s", Status: "active"}
	enq := &fakeEnqueuer{}
	w := NewWorker(repo, enq, 5, 0)

	if err := w.HandleDeliverWebhook(context.Background(), taskFor(t, DeliverWebhookPayload{DeliveryID: "d1", WebhookID: "w1"})); err != nil {
		t.Fatalf("error = %v", err)
	}
	if repo.deliveryStatus["d1"] != StatusFailedPermanent {
		t.Fatalf("status = %q, want failed_permanent", repo.deliveryStatus["d1"])
	}
	if len(enq.tasks) != 0 {
		t.Fatalf("retry tasks = %d, want 0 after max retries", len(enq.tasks))
	}
}

func TestHandleDeliverWebhookIdempotent(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := newFakeRepo()
	repo.delivery = &Delivery{ID: "d1", WebhookID: "w1", Payload: []byte(`{}`), Status: StatusDelivered, Attempts: 1}
	repo.webhook = &Webhook{ID: "w1", URL: srv.URL, SigningSecret: "s", Status: "active"}
	w := NewWorker(repo, &fakeEnqueuer{}, 5, 0)

	if err := w.HandleDeliverWebhook(context.Background(), taskFor(t, DeliverWebhookPayload{DeliveryID: "d1", WebhookID: "w1"})); err != nil {
		t.Fatalf("error = %v", err)
	}
	if hits != 0 {
		t.Fatal("already-delivered delivery must not send an HTTP request")
	}
	if len(repo.attemptUpdates) != 0 {
		t.Fatal("already-delivered delivery must not update attempts")
	}
}

func TestHandleDeliverWebhookBadPayload(t *testing.T) {
	w := NewWorker(newFakeRepo(), &fakeEnqueuer{}, 5, 0)
	task := asynq.NewTask(TaskDeliverWebhook, []byte(`{bad`))
	if err := w.HandleDeliverWebhook(context.Background(), task); err != nil {
		t.Fatalf("bad payload should return nil, got %v", err)
	}
}

func TestHandleDeliverWebhookMissingDeliveryOrWebhook(t *testing.T) {
	// delivery missing
	repo := newFakeRepo() // delivery nil
	w := NewWorker(repo, &fakeEnqueuer{}, 5, 0)
	if err := w.HandleDeliverWebhook(context.Background(), taskFor(t, DeliverWebhookPayload{DeliveryID: "d1", WebhookID: "w1"})); err != nil {
		t.Fatalf("missing delivery should return nil, got %v", err)
	}
	// webhook missing
	repo2 := newFakeRepo()
	repo2.delivery = &Delivery{ID: "d1", Status: StatusPending}
	w2 := NewWorker(repo2, &fakeEnqueuer{}, 5, 0)
	if err := w2.HandleDeliverWebhook(context.Background(), taskFor(t, DeliverWebhookPayload{DeliveryID: "d1", WebhookID: "w1"})); err != nil {
		t.Fatalf("missing webhook should return nil, got %v", err)
	}
}

func TestRetryDelaySchedule(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{1, 30 * time.Second},
		{2, 5 * time.Minute},
		{3, 30 * time.Minute},
		{4, 2 * time.Hour},
		{5, 2 * time.Hour}, // clamps to last
		{0, 30 * time.Second},
	}
	for _, tc := range cases {
		if got := retryDelay(tc.attempts); got != tc.want {
			t.Fatalf("retryDelay(%d) = %v, want %v", tc.attempts, got, tc.want)
		}
	}
}

func TestEnqueueForBooking(t *testing.T) {
	repo := newFakeRepo()
	repo.activeWebhooks = []Webhook{{ID: "w1"}, {ID: "w2"}}
	enq := &fakeEnqueuer{}
	w := NewWorker(repo, enq, 5, 0)

	if err := w.EnqueueForBooking(context.Background(), "bk1", "p1", "delivery.completed", []byte(`{"x":1}`)); err != nil {
		t.Fatalf("EnqueueForBooking error = %v", err)
	}
	if len(repo.createdDeliveries) != 2 {
		t.Fatalf("created deliveries = %d, want 2", len(repo.createdDeliveries))
	}
	if len(enq.tasks) != 2 {
		t.Fatalf("enqueued tasks = %d, want 2", len(enq.tasks))
	}
	// Signing secret must never be in the task payload.
	for _, task := range enq.tasks {
		var p DeliverWebhookPayload
		if err := json.Unmarshal(task.Payload(), &p); err != nil {
			t.Fatalf("bad task payload: %v", err)
		}
		if p.DeliveryID == "" || p.WebhookID == "" || p.EventType != "delivery.completed" {
			t.Fatalf("task payload incomplete: %+v", p)
		}
	}
}

func TestEnqueueForBookingNoWebhooks(t *testing.T) {
	repo := newFakeRepo() // no active webhooks
	enq := &fakeEnqueuer{}
	w := NewWorker(repo, enq, 5, 0)
	if err := w.EnqueueForBooking(context.Background(), "bk1", "p1", "delivery.completed", []byte(`{}`)); err != nil {
		t.Fatalf("EnqueueForBooking error = %v", err)
	}
	if len(repo.createdDeliveries) != 0 || len(enq.tasks) != 0 {
		t.Fatal("no active webhooks must create no deliveries/tasks")
	}
}
