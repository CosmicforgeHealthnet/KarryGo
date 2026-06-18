package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	ChannelPush      = "push"
	ChannelEmail     = "email"
	ChannelWebSocket = "websocket"
	ChannelInApp     = "in_app"

	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"

	RequestStream    = "notification:requests"
	DeliveryStream   = "notification:deliveries"
	DeadLetterStream = "notification:dead_letters"
	ConsumerGroup    = "notification-service"
)

var DefaultChannels = []string{ChannelPush, ChannelWebSocket, ChannelInApp}

type Recipient struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

type Request struct {
	IDempotencyKey string                 `json:"idempotency_key"`
	SourceService  string                 `json:"source_service"`
	EventType      string                 `json:"event_type"`
	Recipient      Recipient              `json:"recipient"`
	Channels       []string               `json:"channels,omitempty"`
	TemplateKey    string                 `json:"template_key,omitempty"`
	Locale         string                 `json:"locale,omitempty"`
	Title          string                 `json:"title,omitempty"`
	Body           string                 `json:"body,omitempty"`
	Data           map[string]interface{} `json:"data,omitempty"`
	TemplateData   map[string]interface{} `json:"template_data,omitempty"`
	Priority       string                 `json:"priority,omitempty"`
}

type SendResponse struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

func (r Request) Validate() error {
	var fields []apperrors.FieldViolation
	if strings.TrimSpace(r.IDempotencyKey) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "idempotency_key", Message: "Idempotency key is required."})
	}
	if strings.TrimSpace(r.SourceService) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "source_service", Message: "Source service is required."})
	}
	if strings.TrimSpace(r.EventType) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "event_type", Message: "Event type is required."})
	}
	if strings.TrimSpace(r.Recipient.Type) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "recipient.type", Message: "Recipient type is required."})
	}
	if strings.TrimSpace(r.Recipient.ID) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "recipient.id", Message: "Recipient id is required."})
	}
	if strings.TrimSpace(r.TemplateKey) == "" && strings.TrimSpace(r.Title) == "" && strings.TrimSpace(r.Body) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "template_key", Message: "Template key or inline title/body is required."})
	}
	for _, channel := range r.Channels {
		if !IsValidChannel(channel) {
			fields = append(fields, apperrors.FieldViolation{Field: "channels", Message: "Unsupported notification channel."})
			break
		}
	}
	if r.Priority != "" && r.Priority != PriorityLow && r.Priority != PriorityNormal && r.Priority != PriorityHigh {
		fields = append(fields, apperrors.FieldViolation{Field: "priority", Message: "Priority must be low, normal, or high."})
	}
	if len(fields) > 0 {
		return apperrors.Validation("Check your notification request.", fields)
	}
	return nil
}

func (r Request) EventName() string {
	if r.EventType == "" {
		return events.NotificationSend
	}
	return r.EventType
}

func IsValidChannel(channel string) bool {
	switch channel {
	case ChannelPush, ChannelEmail, ChannelWebSocket, ChannelInApp:
		return true
	default:
		return false
	}
}

type Client struct {
	BaseURL     string
	HTTPClient  *http.Client
	ServiceName string
	Secret      []byte
}

func (c Client) Send(ctx context.Context, request Request) (SendResponse, error) {
	if err := request.Validate(); err != nil {
		return SendResponse{}, err
	}
	body, err := json.Marshal(request)
	if err != nil {
		return SendResponse{}, err
	}

	url := strings.TrimRight(c.BaseURL, "/") + "/api/v1/notifications/send"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return SendResponse{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(httpRequest, c.ServiceName, c.Secret, body, time.Now()); err != nil {
		return SendResponse{}, err
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		return SendResponse{}, err
	}
	defer response.Body.Close()

	var envelope struct {
		Success bool         `json:"success"`
		Data    SendResponse `json:"data"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return SendResponse{}, err
	}
	if response.StatusCode >= http.StatusBadRequest || !envelope.Success {
		if envelope.Error.Message == "" {
			envelope.Error.Message = "Notification request failed."
		}
		return SendResponse{}, errors.New(envelope.Error.Message)
	}
	return envelope.Data, nil
}

type RedisPublisher struct {
	Client *redis.Client
	Stream string
}

func (p RedisPublisher) Publish(ctx context.Context, request Request) error {
	if err := request.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}

	stream := p.Stream
	if stream == "" {
		stream = RequestStream
	}
	return p.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{
			"event":   events.NotificationSend,
			"payload": string(payload),
		},
	}).Err()
}
