package messageusecases

import (
	"context"
	"testing"
	"time"

	messageclients "cosmicforge/logistics/services/notification-service/internal/features/messages/clients"
	messagemodels "cosmicforge/logistics/services/notification-service/internal/features/messages/models"
	messagerepositories "cosmicforge/logistics/services/notification-service/internal/features/messages/repositories"
	"cosmicforge/logistics/shared/go/notifications"
)

func TestSendUsesTemplateDefaultChannelsWhenChannelsAreOmitted(t *testing.T) {
	repo := newFakeRepository()
	queue := &fakeQueue{}
	service := newTestService(repo, queue)

	result, err := service.Send(context.Background(), notifications.Request{
		IDempotencyKey: "customer-service:booking.created:1",
		SourceService:  "customer-service",
		EventType:      "booking.created",
		Recipient: notifications.Recipient{
			Type: "customer",
			ID:   "customer-1",
		},
		TemplateKey:  "booking.created",
		TemplateData: map[string]interface{}{"name": "Ada"},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result.MessageID == "" || result.Status != messagemodels.StatusQueued {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(repo.deliveries) != 2 {
		t.Fatalf("expected two template deliveries, got %d", len(repo.deliveries))
	}
	if len(queue.enqueued) != 2 {
		t.Fatalf("expected two queued deliveries, got %d", len(queue.enqueued))
	}
	if repo.messages[result.MessageID].Title != "Hello Ada" {
		t.Fatalf("expected rendered title, got %q", repo.messages[result.MessageID].Title)
	}
}

func TestProcessDeliverySkipsEmailWithoutRecipientEmail(t *testing.T) {
	repo := newFakeRepository()
	queue := &fakeQueue{}
	service := newTestService(repo, queue)

	result, err := service.Send(context.Background(), notifications.Request{
		IDempotencyKey: "customer-service:email:1",
		SourceService:  "customer-service",
		EventType:      "email.test",
		Recipient: notifications.Recipient{
			Type: "customer",
			ID:   "customer-1",
		},
		Channels: []string{notifications.ChannelEmail},
		Title:    "Email",
		Body:     "Missing address",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	deliveryID := queue.enqueued[0]
	if err := service.ProcessDelivery(context.Background(), deliveryID); err != nil {
		t.Fatalf("ProcessDelivery() error = %v", err)
	}

	delivery := repo.deliveries[deliveryID]
	if delivery.Status != messagemodels.StatusSkipped {
		t.Fatalf("delivery status = %s", delivery.Status)
	}
	if repo.messages[result.MessageID].Status != messagemodels.StatusQueued {
		t.Fatalf("message should remain queued in v1 aggregate status, got %s", repo.messages[result.MessageID].Status)
	}
}

func newTestService(repo *fakeRepository, queue *fakeQueue) *NotificationService {
	service := NewNotificationService(Options{
		Repository:          repo,
		PushSender:          &fakePushSender{},
		EmailSender:         &fakeEmailSender{},
		RealtimeSender:      &fakeRealtimeSender{},
		Queue:               queue,
		RealtimeTokenSecret: []byte("realtime-secret"),
		MaxAttempts:         5,
	})
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	service.tokens.now = func() time.Time { return now }
	return service
}

type fakeRepository struct {
	messages   map[string]messagemodels.Message
	deliveries map[string]messagemodels.Delivery
	devices    []messagemodels.Device
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		messages:   map[string]messagemodels.Message{},
		deliveries: map[string]messagemodels.Delivery{},
	}
}

func (r *fakeRepository) CreateMessageWithDeliveries(ctx context.Context, input messagerepositories.CreateMessageInput) (messagemodels.Message, []messagemodels.Delivery, bool, error) {
	for _, message := range r.messages {
		if message.IdempotencyKey == input.Request.IDempotencyKey {
			return message, nil, true, nil
		}
	}
	message := messagemodels.Message{
		ID:             "message-" + input.Request.IDempotencyKey,
		IdempotencyKey: input.Request.IDempotencyKey,
		SourceService:  input.Request.SourceService,
		EventType:      input.Request.EventType,
		RecipientType:  input.Request.Recipient.Type,
		RecipientID:    input.Request.Recipient.ID,
		Channels:       input.Channels,
		TemplateKey:    input.TemplateKey,
		Locale:         input.Locale,
		Title:          input.Title,
		Body:           input.Body,
		Data:           input.Request.Data,
		TemplateData:   input.Request.TemplateData,
		Priority:       input.Priority,
		Status:         messagemodels.StatusQueued,
	}
	if input.Request.Recipient.Email != "" {
		message.RecipientEmail = &input.Request.Recipient.Email
	}
	r.messages[message.ID] = message

	var deliveries []messagemodels.Delivery
	for i, channel := range input.Channels {
		delivery := messagemodels.Delivery{
			ID:        message.ID + "-delivery-" + channel,
			MessageID: message.ID,
			Channel:   channel,
			Status:    messagemodels.StatusQueued,
			Attempts:  i * 0,
		}
		r.deliveries[delivery.ID] = delivery
		deliveries = append(deliveries, delivery)
	}
	return message, deliveries, false, nil
}

func (r *fakeRepository) GetMessage(ctx context.Context, id string) (messagemodels.Message, error) {
	return r.messages[id], nil
}

func (r *fakeRepository) ListMessages(ctx context.Context, recipientType string, recipientID string, limit int) ([]messagemodels.Message, error) {
	var messages []messagemodels.Message
	for _, message := range r.messages {
		if message.RecipientType == recipientType && message.RecipientID == recipientID {
			messages = append(messages, message)
		}
	}
	return messages, nil
}

func (r *fakeRepository) GetDelivery(ctx context.Context, id string) (messagemodels.Delivery, error) {
	return r.deliveries[id], nil
}

func (r *fakeRepository) ListDueRetryDeliveries(ctx context.Context, limit int) ([]messagemodels.Delivery, error) {
	return nil, nil
}

func (r *fakeRepository) MarkDelivery(ctx context.Context, id string, status string, nextAttemptAt *time.Time, providerMessageID *string, lastError *string) error {
	delivery := r.deliveries[id]
	delivery.Status = status
	delivery.Attempts++
	delivery.NextAttemptAt = nextAttemptAt
	delivery.ProviderMessageID = providerMessageID
	delivery.LastError = lastError
	r.deliveries[id] = delivery
	return nil
}

func (r *fakeRepository) RecordAttempt(ctx context.Context, deliveryID string, provider string, providerMessageID *string, status string, errorMessage *string) error {
	return nil
}

func (r *fakeRepository) GetTemplate(ctx context.Context, key string, locale string) (messagemodels.Template, bool, error) {
	if key != "booking.created" {
		return messagemodels.Template{}, false, nil
	}
	return messagemodels.Template{
		Key:             key,
		Locale:          locale,
		Title:           "Hello {{name}}",
		Body:            "Your booking is ready.",
		DefaultChannels: []string{notifications.ChannelPush, notifications.ChannelInApp},
		Active:          true,
	}, true, nil
}

func (r *fakeRepository) IsChannelEnabled(ctx context.Context, recipientType string, recipientID string, channel string) (bool, error) {
	return true, nil
}

func (r *fakeRepository) UpsertDevice(ctx context.Context, device messagemodels.Device) (messagemodels.Device, error) {
	device.ID = "device-1"
	r.devices = append(r.devices, device)
	return device, nil
}

func (r *fakeRepository) ListActiveDevices(ctx context.Context, recipientType string, recipientID string) ([]messagemodels.Device, error) {
	return r.devices, nil
}

func (r *fakeRepository) DeactivateDeviceToken(ctx context.Context, token string) error {
	return nil
}

type fakeQueue struct {
	enqueued []string
}

func (q *fakeQueue) EnqueueDelivery(ctx context.Context, deliveryID string) error {
	q.enqueued = append(q.enqueued, deliveryID)
	return nil
}

func (q *fakeQueue) DeadLetterDelivery(ctx context.Context, deliveryID string, reason string) error {
	return nil
}

func (q *fakeQueue) StartDeliveryConsumer(ctx context.Context, handler func(context.Context, string) error) {
}

func (q *fakeQueue) StartRequestConsumer(ctx context.Context, handler func(context.Context, []byte) error) {
}

type fakePushSender struct{}

func (s *fakePushSender) SendPush(ctx context.Context, message messageclients.PushMessage) (messageclients.ProviderResult, error) {
	providerID := "push-1"
	return messageclients.ProviderResult{Provider: "fake_push", ProviderMessageID: &providerID}, nil
}

type fakeEmailSender struct{}

func (s *fakeEmailSender) SendEmail(ctx context.Context, message messageclients.EmailMessage) (messageclients.ProviderResult, error) {
	providerID := "email-1"
	return messageclients.ProviderResult{Provider: "fake_email", ProviderMessageID: &providerID}, nil
}

type fakeRealtimeSender struct{}

func (s *fakeRealtimeSender) SendRealtime(ctx context.Context, message messageclients.RealtimeMessage) error {
	return nil
}
