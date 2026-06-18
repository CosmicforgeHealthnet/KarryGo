package messagehttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	messageclients "cosmicforge/logistics/services/notification-service/internal/features/messages/clients"
	messagemodels "cosmicforge/logistics/services/notification-service/internal/features/messages/models"
	messagerepositories "cosmicforge/logistics/services/notification-service/internal/features/messages/repositories"
	messageusecases "cosmicforge/logistics/services/notification-service/internal/features/messages/usecases"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestSendRequiresServiceAuth(t *testing.T) {
	router := newHTTPTestRouter()
	body := notificationBody(t)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/send", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestSendReturnsAcceptedForSignedRequest(t *testing.T) {
	router := newHTTPTestRouter()
	body := notificationBody(t)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/send", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(request, "customer-service", []byte("secret"), body, time.Now()); err != nil {
		t.Fatalf("SignRequest() error = %v", err)
	}
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func newHTTPTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	repo := &httpFakeRepository{}
	queue := &httpFakeQueue{}
	hub := messageclients.NewWebSocketHub()
	service := messageusecases.NewNotificationService(messageusecases.Options{
		Repository:          repo,
		PushSender:          &httpFakePushSender{},
		EmailSender:         &httpFakeEmailSender{},
		RealtimeSender:      hub,
		Queue:               queue,
		RealtimeTokenSecret: []byte("realtime-secret"),
		MaxAttempts:         5,
	})
	router := gin.New()
	router.Use(httpx.RequestID())
	router.Use(httpx.ErrorHandler())
	RegisterRoutes(router.Group("/api/v1/notifications"), service, hub, serviceauth.Secrets{"customer-service": []byte("secret")})
	return router
}

func notificationBody(t *testing.T) []byte {
	t.Helper()
	payload, err := json.Marshal(notifications.Request{
		IDempotencyKey: "customer-service:test:1",
		SourceService:  "customer-service",
		EventType:      "test.created",
		Recipient: notifications.Recipient{
			Type: "customer",
			ID:   "customer-1",
		},
		Channels: []string{notifications.ChannelInApp},
		Title:    "Hello",
		Body:     "World",
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return payload
}

type httpFakeRepository struct{}

func (r *httpFakeRepository) CreateMessageWithDeliveries(ctx context.Context, input messagerepositories.CreateMessageInput) (messagemodels.Message, []messagemodels.Delivery, bool, error) {
	message := messagemodels.Message{
		ID:             "message-1",
		IdempotencyKey: input.Request.IDempotencyKey,
		SourceService:  input.Request.SourceService,
		EventType:      input.Request.EventType,
		RecipientType:  input.Request.Recipient.Type,
		RecipientID:    input.Request.Recipient.ID,
		Channels:       input.Channels,
		Title:          input.Title,
		Body:           input.Body,
		Priority:       input.Priority,
		Status:         messagemodels.StatusQueued,
	}
	delivery := messagemodels.Delivery{
		ID:        "delivery-1",
		MessageID: message.ID,
		Channel:   notifications.ChannelInApp,
		Status:    messagemodels.StatusQueued,
	}
	return message, []messagemodels.Delivery{delivery}, false, nil
}

func (r *httpFakeRepository) GetMessage(ctx context.Context, id string) (messagemodels.Message, error) {
	return messagemodels.Message{ID: id}, nil
}

func (r *httpFakeRepository) ListMessages(ctx context.Context, recipientType string, recipientID string, limit int) ([]messagemodels.Message, error) {
	return nil, nil
}

func (r *httpFakeRepository) GetDelivery(ctx context.Context, id string) (messagemodels.Delivery, error) {
	return messagemodels.Delivery{}, nil
}

func (r *httpFakeRepository) ListDueRetryDeliveries(ctx context.Context, limit int) ([]messagemodels.Delivery, error) {
	return nil, nil
}

func (r *httpFakeRepository) MarkDelivery(ctx context.Context, id string, status string, nextAttemptAt *time.Time, providerMessageID *string, lastError *string) error {
	return nil
}

func (r *httpFakeRepository) RecordAttempt(ctx context.Context, deliveryID string, provider string, providerMessageID *string, status string, errorMessage *string) error {
	return nil
}

func (r *httpFakeRepository) GetTemplate(ctx context.Context, key string, locale string) (messagemodels.Template, bool, error) {
	return messagemodels.Template{}, false, nil
}

func (r *httpFakeRepository) IsChannelEnabled(ctx context.Context, recipientType string, recipientID string, channel string) (bool, error) {
	return true, nil
}

func (r *httpFakeRepository) UpsertDevice(ctx context.Context, device messagemodels.Device) (messagemodels.Device, error) {
	return device, nil
}

func (r *httpFakeRepository) ListActiveDevices(ctx context.Context, recipientType string, recipientID string) ([]messagemodels.Device, error) {
	return nil, nil
}

func (r *httpFakeRepository) DeactivateDeviceToken(ctx context.Context, token string) error {
	return nil
}

type httpFakeQueue struct{}

func (q *httpFakeQueue) EnqueueDelivery(ctx context.Context, deliveryID string) error {
	return nil
}

func (q *httpFakeQueue) DeadLetterDelivery(ctx context.Context, deliveryID string, reason string) error {
	return nil
}

func (q *httpFakeQueue) StartDeliveryConsumer(ctx context.Context, handler func(context.Context, string) error) {
}

func (q *httpFakeQueue) StartRequestConsumer(ctx context.Context, handler func(context.Context, []byte) error) {
}

type httpFakePushSender struct{}

func (s *httpFakePushSender) SendPush(ctx context.Context, message messageclients.PushMessage) (messageclients.ProviderResult, error) {
	return messageclients.ProviderResult{Provider: "fake_push"}, nil
}

type httpFakeEmailSender struct{}

func (s *httpFakeEmailSender) SendEmail(ctx context.Context, message messageclients.EmailMessage) (messageclients.ProviderResult, error) {
	return messageclients.ProviderResult{Provider: "fake_email"}, nil
}
