package messageusecases

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/services/notification-service/internal/config"
	messageclients "cosmicforge/logistics/services/notification-service/internal/features/messages/clients"
	messagemodels "cosmicforge/logistics/services/notification-service/internal/features/messages/models"
	messagerepositories "cosmicforge/logistics/services/notification-service/internal/features/messages/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/notifications"
)

type NotificationService struct {
	repository Repository
	push       messageclients.PushSender
	email      messageclients.EmailSender
	realtime   messageclients.RealtimeSender
	queue      messageclients.DeliveryQueue
	tokens     *RealtimeTokenManager

	maxAttempts int
	now         func() time.Time
}

type Repository interface {
	messagerepositories.Repository
}

type Options struct {
	Repository          Repository
	PushSender          messageclients.PushSender
	EmailSender         messageclients.EmailSender
	RealtimeSender      messageclients.RealtimeSender
	Queue               messageclients.DeliveryQueue
	RealtimeTokenSecret []byte
	MaxAttempts         int
}

func NewNotificationService(opts Options) *NotificationService {
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	return &NotificationService{
		repository:  opts.Repository,
		push:        opts.PushSender,
		email:       opts.EmailSender,
		realtime:    opts.RealtimeSender,
		queue:       opts.Queue,
		tokens:      NewRealtimeTokenManager(opts.RealtimeTokenSecret),
		maxAttempts: maxAttempts,
		now:         time.Now,
	}
}

type SendResult struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

func (s *NotificationService) Send(ctx context.Context, request notifications.Request) (SendResult, error) {
	if err := request.Validate(); err != nil {
		return SendResult{}, err
	}

	resolved, err := s.resolveRequest(ctx, request)
	if err != nil {
		return SendResult{}, err
	}
	message, deliveries, duplicate, err := s.repository.CreateMessageWithDeliveries(ctx, messagerepositories.CreateMessageInput{
		Request:     request,
		Channels:    resolved.channels,
		TemplateKey: resolved.templateKey,
		Locale:      resolved.locale,
		Title:       resolved.title,
		Body:        resolved.body,
		Priority:    resolved.priority,
	})
	if err != nil {
		return SendResult{}, err
	}
	if duplicate {
		return SendResult{MessageID: message.ID, Status: message.Status}, nil
	}

	for _, delivery := range deliveries {
		if s.queue == nil {
			continue
		}
		if err := s.queue.EnqueueDelivery(ctx, delivery.ID); err != nil {
			return SendResult{}, apperrors.Unavailable("Notification delivery could not be queued.", err)
		}
	}

	return SendResult{MessageID: message.ID, Status: messagemodels.StatusQueued}, nil
}

func (s *NotificationService) HandleStreamRequest(ctx context.Context, payload []byte) error {
	var request notifications.Request
	if err := json.Unmarshal(payload, &request); err != nil {
		return err
	}
	_, err := s.Send(ctx, request)
	return err
}

func (s *NotificationService) StartConsumers(ctx context.Context, workerCount int) {
	if s.queue == nil {
		return
	}
	if workerCount <= 0 {
		workerCount = 1
	}

	go s.queue.StartRequestConsumer(ctx, s.HandleStreamRequest)
	for i := 0; i < workerCount; i++ {
		go s.queue.StartDeliveryConsumer(ctx, s.ProcessDelivery)
	}
	go s.retryDueDeliveries(ctx)
}

func (s *NotificationService) ProcessDelivery(ctx context.Context, deliveryID string) error {
	delivery, err := s.repository.GetDelivery(ctx, deliveryID)
	if err != nil {
		return err
	}
	if isFinalStatus(delivery.Status) {
		return nil
	}

	message, err := s.repository.GetMessage(ctx, delivery.MessageID)
	if err != nil {
		return err
	}

	enabled, err := s.repository.IsChannelEnabled(ctx, message.RecipientType, message.RecipientID, delivery.Channel)
	if err != nil {
		return err
	}
	if !enabled {
		return s.finishDelivery(ctx, delivery, "preferences", nil, messagemodels.StatusSuppressed, nil)
	}

	switch delivery.Channel {
	case notifications.ChannelPush:
		return s.deliverPush(ctx, delivery, message)
	case notifications.ChannelEmail:
		return s.deliverEmail(ctx, delivery, message)
	case notifications.ChannelWebSocket:
		return s.deliverRealtime(ctx, delivery, message)
	case notifications.ChannelInApp:
		return s.finishDelivery(ctx, delivery, "in_app", nil, messagemodels.StatusSent, nil)
	default:
		errMessage := "unsupported notification channel"
		return s.finishDelivery(ctx, delivery, "notification-service", nil, messagemodels.StatusSkipped, &errMessage)
	}
}

type RegisterDeviceInput struct {
	RecipientType string `json:"recipient_type"`
	RecipientID   string `json:"recipient_id"`
	Token         string `json:"token"`
	Platform      string `json:"platform"`
	App           string `json:"app"`
}

func (s *NotificationService) RegisterDevice(ctx context.Context, input RegisterDeviceInput) (messagemodels.Device, error) {
	var fields []apperrors.FieldViolation
	if input.RecipientType == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "recipient_type", Message: "Recipient type is required."})
	}
	if input.RecipientID == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "recipient_id", Message: "Recipient id is required."})
	}
	if input.Token == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "token", Message: "Device token is required."})
	}
	if len(fields) > 0 {
		return messagemodels.Device{}, apperrors.Validation("Check your device details.", fields)
	}
	if input.Platform == "" {
		input.Platform = "unknown"
	}
	if input.App == "" {
		input.App = "unknown"
	}

	return s.repository.UpsertDevice(ctx, messagemodels.Device{
		RecipientType: input.RecipientType,
		RecipientID:   input.RecipientID,
		Token:         input.Token,
		Platform:      input.Platform,
		App:           input.App,
	})
}

type RealtimeTokenInput struct {
	RecipientType string `json:"recipient_type"`
	RecipientID   string `json:"recipient_id"`
}

type RealtimeTokenResult struct {
	Token     string `json:"token"`
	ExpiresIn int64  `json:"expires_in"`
}

func (s *NotificationService) RealtimeToken(input RealtimeTokenInput) (RealtimeTokenResult, error) {
	ttl := 15 * time.Minute
	token, err := s.tokens.Sign(input.RecipientType, input.RecipientID, ttl)
	if err != nil {
		return RealtimeTokenResult{}, err
	}
	return RealtimeTokenResult{Token: token, ExpiresIn: int64(ttl.Seconds())}, nil
}

func (s *NotificationService) VerifyRealtimeToken(token string) (string, string, error) {
	return s.tokens.Verify(token)
}

func (s *NotificationService) GetMessage(ctx context.Context, id string) (messagemodels.Message, error) {
	return s.repository.GetMessage(ctx, id)
}

func (s *NotificationService) ListMessages(ctx context.Context, recipientType string, recipientID string, limit int) ([]messagemodels.Message, error) {
	if recipientType == "" || recipientID == "" {
		return nil, apperrors.Validation("Check your query.", []apperrors.FieldViolation{
			{Field: "recipient", Message: "Recipient type and id are required."},
		})
	}
	return s.repository.ListMessages(ctx, recipientType, recipientID, limit)
}

func (s *NotificationService) retryDueDeliveries(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deliveries, err := s.repository.ListDueRetryDeliveries(ctx, 50)
			if err != nil {
				continue
			}
			for _, delivery := range deliveries {
				_ = s.queue.EnqueueDelivery(ctx, delivery.ID)
			}
		}
	}
}

type resolvedRequest struct {
	channels    []string
	templateKey *string
	locale      string
	title       string
	body        string
	priority    string
}

func (s *NotificationService) resolveRequest(ctx context.Context, request notifications.Request) (resolvedRequest, error) {
	resolved := resolvedRequest{
		channels: append([]string{}, request.Channels...),
		locale:   request.Locale,
		title:    request.Title,
		body:     request.Body,
		priority: request.Priority,
	}
	if resolved.locale == "" {
		resolved.locale = "en-NG"
	}
	if resolved.priority == "" {
		resolved.priority = notifications.PriorityNormal
	}
	if request.TemplateKey != "" {
		resolved.templateKey = &request.TemplateKey
		template, ok, err := s.repository.GetTemplate(ctx, request.TemplateKey, resolved.locale)
		if err != nil {
			return resolvedRequest{}, err
		}
		if ok {
			if resolved.title == "" {
				resolved.title = renderTemplate(template.Title, request.TemplateData)
			}
			if resolved.body == "" {
				resolved.body = renderTemplate(template.Body, request.TemplateData)
			}
			if len(resolved.channels) == 0 {
				resolved.channels = append([]string{}, template.DefaultChannels...)
			}
		} else if resolved.title == "" && resolved.body == "" {
			return resolvedRequest{}, apperrors.NotFound("Notification template could not be found.", nil)
		}
	}
	if len(resolved.channels) == 0 {
		resolved.channels = append([]string{}, notifications.DefaultChannels...)
	}
	if resolved.title == "" && resolved.body == "" {
		return resolvedRequest{}, apperrors.Validation("Check your notification request.", []apperrors.FieldViolation{
			{Field: "title", Message: "Title or body is required."},
		})
	}
	return resolved, nil
}

func (s *NotificationService) deliverPush(ctx context.Context, delivery messagemodels.Delivery, message messagemodels.Message) error {
	devices, err := s.repository.ListActiveDevices(ctx, message.RecipientType, message.RecipientID)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		errMessage := "recipient has no active push devices"
		return s.finishDelivery(ctx, delivery, "firebase", nil, messagemodels.StatusSkipped, &errMessage)
	}
	tokens := make([]string, 0, len(devices))
	for _, device := range devices {
		tokens = append(tokens, device.Token)
	}
	result, err := s.push.SendPush(ctx, messageclients.PushMessage{
		Tokens:   tokens,
		Title:    message.Title,
		Body:     message.Body,
		Data:     stringifyData(message.Data),
		Priority: message.Priority,
	})
	for _, invalidToken := range result.InvalidTokens {
		_ = s.repository.DeactivateDeviceToken(ctx, invalidToken)
	}
	return s.handleProviderResult(ctx, delivery, result, err)
}

func (s *NotificationService) deliverEmail(ctx context.Context, delivery messagemodels.Delivery, message messagemodels.Message) error {
	if message.RecipientEmail == nil || *message.RecipientEmail == "" {
		errMessage := "recipient email is missing"
		return s.finishDelivery(ctx, delivery, "smtp", nil, messagemodels.StatusSkipped, &errMessage)
	}
	result, err := s.email.SendEmail(ctx, messageclients.EmailMessage{
		To:      *message.RecipientEmail,
		Subject: message.Title,
		Body:    message.Body,
	})
	return s.handleProviderResult(ctx, delivery, result, err)
}

func (s *NotificationService) deliverRealtime(ctx context.Context, delivery messagemodels.Delivery, message messagemodels.Message) error {
	err := s.realtime.SendRealtime(ctx, messageclients.RealtimeMessage{
		RecipientType: message.RecipientType,
		RecipientID:   message.RecipientID,
		EventType:     message.EventType,
		Title:         message.Title,
		Body:          message.Body,
		Data:          message.Data,
	})
	if err != nil {
		result := messageclients.ProviderResult{Provider: "websocket", Retryable: true}
		return s.handleProviderResult(ctx, delivery, result, err)
	}
	return s.finishDelivery(ctx, delivery, "websocket", nil, messagemodels.StatusSent, nil)
}

func (s *NotificationService) handleProviderResult(ctx context.Context, delivery messagemodels.Delivery, result messageclients.ProviderResult, err error) error {
	provider := result.Provider
	if provider == "" {
		provider = delivery.Channel
	}
	if err == nil {
		return s.finishDelivery(ctx, delivery, provider, result.ProviderMessageID, messagemodels.StatusSent, nil)
	}

	errMessage := err.Error()
	status := messagemodels.StatusFailed
	nextAttemptAt := (*time.Time)(nil)
	if result.Retryable && delivery.Attempts+1 < s.maxAttempts {
		status = messagemodels.StatusRetrying
		next := s.now().Add(config.RetryBackoff(delivery.Attempts + 1))
		nextAttemptAt = &next
	} else if result.Retryable {
		status = messagemodels.StatusDeadLettered
		if s.queue != nil {
			_ = s.queue.DeadLetterDelivery(ctx, delivery.ID, errMessage)
		}
	}

	if recordErr := s.repository.RecordAttempt(ctx, delivery.ID, provider, result.ProviderMessageID, status, &errMessage); recordErr != nil {
		return recordErr
	}
	if err := s.repository.MarkDelivery(ctx, delivery.ID, status, nextAttemptAt, result.ProviderMessageID, &errMessage); err != nil {
		return err
	}
	if status == messagemodels.StatusRetrying && s.queue != nil && nextAttemptAt != nil {
		delay := time.Until(*nextAttemptAt)
		time.AfterFunc(delay, func() {
			_ = s.queue.EnqueueDelivery(context.Background(), delivery.ID)
		})
	}
	return nil
}

func (s *NotificationService) finishDelivery(ctx context.Context, delivery messagemodels.Delivery, provider string, providerMessageID *string, status string, errMessage *string) error {
	if err := s.repository.RecordAttempt(ctx, delivery.ID, provider, providerMessageID, status, errMessage); err != nil {
		return err
	}
	return s.repository.MarkDelivery(ctx, delivery.ID, status, nil, providerMessageID, errMessage)
}

func renderTemplate(template string, values map[string]interface{}) string {
	rendered := template
	for key, value := range values {
		rendered = strings.ReplaceAll(rendered, "{{"+key+"}}", fmt.Sprint(value))
	}
	return rendered
}

func stringifyData(data map[string]interface{}) map[string]string {
	result := map[string]string{}
	for key, value := range data {
		result[key] = fmt.Sprint(value)
	}
	return result
}

func isFinalStatus(status string) bool {
	switch status {
	case messagemodels.StatusSent, messagemodels.StatusSkipped, messagemodels.StatusSuppressed, messagemodels.StatusFailed, messagemodels.StatusDeadLettered:
		return true
	default:
		return false
	}
}
