package webhookdelivery

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/hibiken/asynq"
)

const (
	defaultMaxRetries = 5
	defaultTimeout    = 30 * time.Second
)

// Worker signs and delivers webhooks and schedules retries via Asynq.
type Worker struct {
	repo        Repository
	asynqClient Enqueuer
	maxRetries  int
	timeout     time.Duration
	now         func() time.Time
}

// NewWorker builds a Worker. maxRetries<=0 defaults to 5, timeout<=0 to 30s.
func NewWorker(repo Repository, client Enqueuer, maxRetries int, timeout time.Duration) *Worker {
	w := &Worker{repo: repo, asynqClient: client, maxRetries: maxRetries, timeout: timeout, now: time.Now}
	if w.maxRetries <= 0 {
		w.maxRetries = defaultMaxRetries
	}
	if w.timeout <= 0 {
		w.timeout = defaultTimeout
	}
	return w
}

// HandleDeliverWebhook is the Asynq handler for TaskDeliverWebhook. It is
// idempotent (already-delivered deliveries are a no-op) and never panics on nil
// responses or bad input.
func (w *Worker) HandleDeliverWebhook(ctx context.Context, t *asynq.Task) error {
	var p DeliverWebhookPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return nil // bad task payload: drop
	}

	delivery, err := w.repo.GetDelivery(ctx, p.DeliveryID)
	if err != nil || delivery == nil {
		return nil
	}
	webhook, err := w.repo.GetWebhook(ctx, p.WebhookID)
	if err != nil || webhook == nil {
		return nil
	}
	if delivery.Status == StatusDelivered {
		return nil // idempotent: already delivered
	}

	attempts := delivery.Attempts + 1
	if err := w.repo.UpdateDeliveryAttempt(ctx, p.DeliveryID, attempts); err != nil {
		return err
	}

	body := delivery.Payload // already-serialized JSON body
	timestamp := strconv.FormatInt(w.now().Unix(), 10)
	signature := signPayload(body, timestamp, webhook.SigningSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-KarryGo-Signature", "sha256="+signature)
	req.Header.Set("X-KarryGo-Event", p.EventType)
	req.Header.Set("X-KarryGo-Delivery", p.DeliveryID)
	req.Header.Set("X-KarryGo-Timestamp", timestamp)

	client := &http.Client{Timeout: w.timeout}
	resp, err := client.Do(req)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	if err != nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if attempts >= w.maxRetries {
			_ = w.repo.UpdateDeliveryStatus(ctx, p.DeliveryID, StatusFailedPermanent)
			return nil
		}
		delay := retryDelay(attempts)
		retryTask := asynq.NewTask(TaskDeliverWebhook, t.Payload())
		if _, enqueueErr := w.asynqClient.Enqueue(retryTask, asynq.ProcessIn(delay)); enqueueErr != nil {
			return enqueueErr
		}
		return nil
	}

	return w.repo.UpdateDeliveryStatus(ctx, p.DeliveryID, StatusDelivered)
}

// EnqueueForBooking finds active webhooks for the partner+event, creates a
// delivery record per webhook, and enqueues an Asynq delivery task per record.
// The signing secret is never placed in the task payload. No active webhooks =>
// no error (booking status updates must not fail on a missing webhook).
func (w *Worker) EnqueueForBooking(ctx context.Context, bookingID, partnerID, eventType string, payload []byte) error {
	hooks, err := w.repo.ListActiveWebhooksForPartnerEvent(ctx, partnerID, eventType)
	if err != nil {
		return err
	}
	for i := range hooks {
		h := hooks[i]
		deliveryID, err := w.repo.CreateWebhookDelivery(ctx, CreateDeliveryParams{
			PartnerID: partnerID,
			BookingID: bookingID,
			WebhookID: h.ID,
			EventType: eventType,
			Payload:   payload,
		})
		if err != nil {
			log.Printf("webhookdelivery: create delivery for webhook %s failed: %v", h.ID, err)
			continue
		}
		taskPayload, err := json.Marshal(DeliverWebhookPayload{
			DeliveryID: deliveryID,
			WebhookID:  h.ID,
			EventType:  eventType,
		})
		if err != nil {
			continue
		}
		task := asynq.NewTask(TaskDeliverWebhook, taskPayload)
		if _, err := w.asynqClient.Enqueue(task); err != nil {
			log.Printf("webhookdelivery: enqueue for webhook %s failed: %v", h.ID, err)
		}
	}
	return nil
}

// signPayload returns the hex HMAC-SHA256 of "<timestamp>.<body>" keyed by secret.
func signPayload(body []byte, timestamp, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + string(body)))
	return hex.EncodeToString(mac.Sum(nil))
}

// retryDelay maps an attempt number to its backoff before the next attempt.
// attempt 1 -> 30s, 2 -> 5m, 3 -> 30m, 4+ -> 2h.
func retryDelay(attempts int) time.Duration {
	delays := []time.Duration{
		30 * time.Second,
		5 * time.Minute,
		30 * time.Minute,
		2 * time.Hour,
	}
	idx := attempts - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(delays) {
		idx = len(delays) - 1
	}
	return delays[idx]
}
