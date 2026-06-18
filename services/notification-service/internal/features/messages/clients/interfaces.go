package messageclients

import "context"

type ProviderResult struct {
	Provider          string
	ProviderMessageID *string
	InvalidTokens     []string
	Retryable         bool
}

type PushMessage struct {
	Tokens   []string
	Title    string
	Body     string
	Data     map[string]string
	Priority string
}

type EmailMessage struct {
	To      string
	Subject string
	Body    string
}

type RealtimeMessage struct {
	RecipientType string
	RecipientID   string
	EventType     string
	Title         string
	Body          string
	Data          map[string]interface{}
}

type PushSender interface {
	SendPush(ctx context.Context, message PushMessage) (ProviderResult, error)
}

type EmailSender interface {
	SendEmail(ctx context.Context, message EmailMessage) (ProviderResult, error)
}

type RealtimeSender interface {
	SendRealtime(ctx context.Context, message RealtimeMessage) error
}

type DeliveryQueue interface {
	EnqueueDelivery(ctx context.Context, deliveryID string) error
	DeadLetterDelivery(ctx context.Context, deliveryID string, reason string) error
	StartDeliveryConsumer(ctx context.Context, handler func(context.Context, string) error)
	StartRequestConsumer(ctx context.Context, handler func(context.Context, []byte) error)
}
