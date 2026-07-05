package webhookdelivery

import (
	"context"

	"github.com/hibiken/asynq"
)

// Booking is the minimal partner booking view the subscriber needs.
type Booking struct {
	ID                string
	PartnerID         string
	DispatchBookingID string
	Status            string
}

// Delivery is a partner_webhook_deliveries row view.
type Delivery struct {
	ID        string
	WebhookID string
	PartnerID string
	EventType string
	Payload   []byte // raw JSON body to POST
	Status    string
	Attempts  int
}

// Webhook is a partner_webhooks row view. SigningSecret is the PLAINTEXT secret
// (the repository decrypts it), used for HMAC signing.
type Webhook struct {
	ID            string
	PartnerID     string
	URL           string
	SigningSecret string
	Status        string
}

// CreateDeliveryParams is the input for a new webhook delivery record.
type CreateDeliveryParams struct {
	PartnerID string
	BookingID string
	WebhookID string
	EventType string
	Payload   []byte
}

// Repository is the DB surface for dispatch-event mapping and webhook delivery.
// It maps onto the existing partner_bookings / partner_webhooks /
// partner_webhook_deliveries tables — no new tables.
type Repository interface {
	// Partner booking (dispatch subscriber).
	FindByDispatchBookingID(ctx context.Context, dispatchBookingID string) (*Booking, error)
	UpdateBookingStatus(ctx context.Context, bookingID, status string) error

	// Webhook delivery (worker + enqueue).
	GetDelivery(ctx context.Context, deliveryID string) (*Delivery, error)
	GetWebhook(ctx context.Context, webhookID string) (*Webhook, error)
	UpdateDeliveryAttempt(ctx context.Context, deliveryID string, attempts int) error
	UpdateDeliveryStatus(ctx context.Context, deliveryID, status string) error
	CreateWebhookDelivery(ctx context.Context, params CreateDeliveryParams) (deliveryID string, err error)
	ListActiveWebhooksForPartnerEvent(ctx context.Context, partnerID, eventType string) ([]Webhook, error)
}

// WebhookWorkerInterface is the subset of the worker the subscriber depends on.
type WebhookWorkerInterface interface {
	EnqueueForBooking(ctx context.Context, bookingID, partnerID, eventType string, payload []byte) error
}

// Enqueuer abstracts *asynq.Client so the worker is testable without Redis.
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}
