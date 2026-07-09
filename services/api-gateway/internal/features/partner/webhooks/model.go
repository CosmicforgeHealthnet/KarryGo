// Package webhooks implements partner webhook registration, HMAC-signed
// delivery, dispatch-event fan-out, and a DB-backed retry worker.
package webhooks

import (
	"time"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
)

// Partner-facing webhook event types.
const (
	EventDeliveryAccepted   = "delivery.accepted"
	EventDeliveryInProgress = "delivery.in_progress"
	EventDeliveryCompleted  = "delivery.completed"
	EventDeliveryCancelled  = "delivery.cancelled"
	EventDeliveryNoProvider = "delivery.no_provider"
	EventDeliveryFailed     = "delivery.failed"

	// EventDeliveryTest is only used by POST /webhooks/:id/test and never requires
	// a subscription.
	EventDeliveryTest = "delivery.test"
)

// allowedPartnerEvents is the set a partner may subscribe to.
var allowedPartnerEvents = map[string]struct{}{
	EventDeliveryAccepted:   {},
	EventDeliveryInProgress: {},
	EventDeliveryCompleted:  {},
	EventDeliveryCancelled:  {},
	EventDeliveryNoProvider: {},
	EventDeliveryFailed:     {},
}

// Webhook statuses.
const (
	StatusActive   = "active"
	StatusInactive = "inactive"
)

// Webhook delivery statuses.
const (
	DeliveryStatusPending         = "pending"
	DeliveryStatusDelivered       = "delivered"
	DeliveryStatusFailedPermanent = "failed_permanent"
)

// Dispatch event topics consumed by api-gateway. These strings mirror
// driver-dispatch-delivery-service (request/model.go, trip/model.go and
// trip/subscribers.go); they are duplicated here because cross-service internal
// packages must not be imported.
const (
	DispatchTopicRequestAccepted = "request.accepted"
	DispatchTopicNoProviderFound = "request.no_provider_found"
	DispatchTopicTripStarted     = "trip.started"
	DispatchTopicTripCompleted   = "trip.completed"
	DispatchTopicTripCancelled   = "trip.cancelled"
	DispatchTopicTripFailed      = "trip.failed"
)

// EventMapping maps a dispatch topic to a partner event and partner booking status.
type EventMapping struct {
	PartnerEvent  string
	BookingStatus string
}

// dispatchTopicMap maps dispatch topics to partner events + booking statuses.
var dispatchTopicMap = map[string]EventMapping{
	DispatchTopicRequestAccepted: {EventDeliveryAccepted, delivery.StatusAccepted},
	DispatchTopicNoProviderFound: {EventDeliveryNoProvider, delivery.StatusNoProvider},
	DispatchTopicTripStarted:     {EventDeliveryInProgress, delivery.StatusInProgress},
	DispatchTopicTripCompleted:   {EventDeliveryCompleted, delivery.StatusCompleted},
	DispatchTopicTripCancelled:   {EventDeliveryCancelled, delivery.StatusCancelled},
	DispatchTopicTripFailed:      {EventDeliveryFailed, delivery.StatusFailed},
}

// DispatchTopics returns the dispatch topics api-gateway subscribes to.
func DispatchTopics() []string {
	return []string{
		DispatchTopicRequestAccepted,
		DispatchTopicNoProviderFound,
		DispatchTopicTripStarted,
		DispatchTopicTripCompleted,
		DispatchTopicTripCancelled,
		DispatchTopicTripFailed,
	}
}

// MappingForTopic returns the partner event mapping for a dispatch topic.
func MappingForTopic(topic string) (EventMapping, bool) {
	m, ok := dispatchTopicMap[topic]
	return m, ok
}

// Webhook is a partner-registered webhook endpoint.
type Webhook struct {
	ID        string
	PartnerID string
	URL       string
	Events    []string
	// EncryptedSigningSecret holds the AES-GCM encrypted signing secret. The
	// plaintext is NEVER stored or logged; it is shown once at creation and, for
	// delivery, decrypted in memory just long enough to compute the HMAC.
	EncryptedSigningSecret string
	Status                 string
	CreatedAt              time.Time
}

// WebhookDelivery is one delivery attempt record (partner_webhook_deliveries).
type WebhookDelivery struct {
	ID             string
	WebhookID      string
	BookingID      string
	EventType      string
	Payload        []byte
	Attempts       int
	LastAttemptAt  *time.Time
	NextAttemptAt  *time.Time
	Status         string
	ResponseStatus *int
	CreatedAt      time.Time
}

// WebhookPayload is the JSON body delivered to partner endpoints.
type WebhookPayload struct {
	Event       string         `json:"event"`
	DeliveryID  string         `json:"delivery_id"`
	BookingRef  string         `json:"booking_ref"`
	ExternalRef *string        `json:"external_ref"`
	Status      string         `json:"status"`
	OccurredAt  string         `json:"occurred_at"`
	Data        map[string]any `json:"data"`
}
