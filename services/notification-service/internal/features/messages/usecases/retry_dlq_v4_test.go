package messageusecases

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	messageclients "cosmicforge/logistics/services/notification-service/internal/features/messages/clients"
	messagemodels "cosmicforge/logistics/services/notification-service/internal/features/messages/models"
	"cosmicforge/logistics/shared/go/notifications"
)

type retryingEmail struct{ messages []messageclients.EmailMessage }

func (s *retryingEmail) SendEmail(_ context.Context, message messageclients.EmailMessage) (messageclients.ProviderResult, error) {
	s.messages = append(s.messages, message)
	if len(s.messages) == 1 {
		return messageclients.ProviderResult{Provider: "smtp", Retryable: true}, errors.New("smtp transport failed")
	}
	return messageclients.ProviderResult{Provider: "smtp"}, nil
}

func TestSMTPRetryUsesPersistedDeliveryIdentityAndMessageTime(t *testing.T) {
	repo := newFakeRepository()
	queue := &fakeQueue{}
	service := newTestService(repo, queue)
	sender := &retryingEmail{}
	service.email = sender
	result, err := service.Send(context.Background(), notifications.Request{
		IDempotencyKey: "email-retry", SourceService: "customer-service", EventType: "customer.auth.otp",
		Recipient: notifications.Recipient{Type: "customer", ID: "test-account", Email: "test@example.com"},
		Channels:  []string{notifications.ChannelEmail}, Title: "Test", Body: "Test notification",
	})
	if err != nil {
		t.Fatal(err)
	}
	message := repo.messages[result.MessageID]
	message.CreatedAt = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	repo.messages[result.MessageID] = message
	id := queue.enqueued[0]
	for i := 0; i < 3; i++ {
		if err := service.ProcessDelivery(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if len(sender.messages) != 2 || sender.messages[0].DeliveryID != id || sender.messages[1].DeliveryID != id ||
		!sender.messages[0].CreatedAt.Equal(message.CreatedAt) || !sender.messages[1].CreatedAt.Equal(message.CreatedAt) {
		t.Fatal("retry identity/time changed or final delivery was sent again")
	}
}

type failingRealtime struct{}

func (failingRealtime) SendRealtime(context.Context, messageclients.RealtimeMessage) error {
	return errors.New("realtime provider unreachable")
}

type recordingQueue struct {
	fakeQueue
	mu     sync.Mutex
	letter []string
}

func (q *recordingQueue) EnqueueDelivery(context.Context, string) error { return nil }
func (q *recordingQueue) DeadLetterDelivery(_ context.Context, id, _ string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.letter = append(q.letter, id)
	return nil
}

// Customer V4 §26.5: a provider outage never rolls back the accepted
// notification; delivery retries with bounded backoff and dead-letters once,
// after which further processing changes nothing.
func TestDeliveryRetriesWithBackoffThenDeadLettersOnce(t *testing.T) {
	repo := newFakeRepository()
	queue := &recordingQueue{}
	service := newTestService(repo, &queue.fakeQueue)
	service.realtime = failingRealtime{}
	service.queue = queue

	result, err := service.Send(context.Background(), notifications.Request{
		IDempotencyKey: "taxi:ride.assigned:1", SourceService: "driver-taxi-service", EventType: "ride.assigned",
		Recipient: notifications.Recipient{Type: "customer", ID: "customer-1"},
		Channels:  []string{notifications.ChannelWebSocket}, Title: "Driver assigned", Body: "Open your ride.",
	})
	if err != nil || result.MessageID == "" {
		t.Fatalf("Send: %v %+v", err, result)
	}
	var deliveryID string
	for id := range repo.deliveries {
		deliveryID = id
	}
	var previousGap int64
	for attempt := 1; attempt <= 4; attempt++ {
		if err := service.ProcessDelivery(context.Background(), deliveryID); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		d := repo.deliveries[deliveryID]
		if d.Status != messagemodels.StatusRetrying || d.NextAttemptAt == nil {
			t.Fatalf("attempt %d: status=%s next=%v, want a scheduled retry", attempt, d.Status, d.NextAttemptAt)
		}
		gap := int64(d.NextAttemptAt.Sub(service.now()))
		if gap <= previousGap {
			t.Fatalf("attempt %d: backoff %d did not grow beyond %d", attempt, gap, previousGap)
		}
		previousGap = gap
	}
	if err := service.ProcessDelivery(context.Background(), deliveryID); err != nil {
		t.Fatal(err)
	}
	if d := repo.deliveries[deliveryID]; d.Status != messagemodels.StatusDeadLettered || len(queue.letter) != 1 {
		t.Fatalf("final status=%s dead letters=%d", d.Status, len(queue.letter))
	}
	attempts := repo.deliveries[deliveryID].Attempts
	_ = service.ProcessDelivery(context.Background(), deliveryID)
	if repo.deliveries[deliveryID].Attempts != attempts || len(queue.letter) != 1 {
		t.Fatal("a dead-lettered delivery was processed again")
	}
	if _, ok := repo.messages[result.MessageID]; !ok {
		t.Fatal("the accepted notification was rolled back by a provider outage")
	}
}
