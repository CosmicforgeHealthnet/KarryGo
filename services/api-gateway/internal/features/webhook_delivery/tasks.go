// Package webhookdelivery implements Asynq-based partner webhook delivery:
// dispatch-event subscribers that enqueue deliveries, and a worker that signs
// and POSTs them with retry scheduling.
package webhookdelivery

// TaskDeliverWebhook is the Asynq task type for a single webhook delivery attempt.
const TaskDeliverWebhook = "webhook:deliver"

// DeliverWebhookPayload is the Asynq task payload. It intentionally never
// contains the signing secret — the worker loads it from the DB.
type DeliverWebhookPayload struct {
	DeliveryID string `json:"delivery_id"`
	WebhookID  string `json:"webhook_id"`
	EventType  string `json:"event_type"`
}

// Delivery status values.
const (
	StatusPending         = "pending"
	StatusDelivered       = "delivered"
	StatusFailedPermanent = "failed_permanent"
)
