package webhooks

import (
	"context"
	"time"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
)

// WebhookRepository persists partner webhooks. All partner-facing queries MUST
// filter by partner_id.
type WebhookRepository interface {
	CreateWebhook(ctx context.Context, w *Webhook) (*Webhook, error)
	ListWebhooks(ctx context.Context, partnerID string) ([]Webhook, error)
	// DeleteWebhook soft-deletes (status=inactive) a partner's webhook.
	DeleteWebhook(ctx context.Context, partnerID, webhookID string) error
	FindWebhookByPartner(ctx context.Context, partnerID, webhookID string) (*Webhook, error)
	FindActiveWebhooksForEvent(ctx context.Context, partnerID, eventType string) ([]Webhook, error)
	// FindWebhookByID loads a webhook without a partner filter. It is used only by
	// the internal retry worker.
	FindWebhookByID(ctx context.Context, webhookID string) (*Webhook, error)
}

// WebhookDeliveryRepository persists webhook delivery attempts.
type WebhookDeliveryRepository interface {
	CreateDelivery(ctx context.Context, d *WebhookDelivery) (*WebhookDelivery, error)
	MarkDelivered(ctx context.Context, deliveryID string, responseStatus int) error
	MarkFailedAttempt(ctx context.Context, deliveryID string, attempts int, responseStatus *int, nextAttemptAt *time.Time) error
	MarkFailedPermanent(ctx context.Context, deliveryID string, responseStatus *int) error
	FindPendingDueDeliveries(ctx context.Context, limit int) ([]WebhookDelivery, error)
	// ListByWebhook returns a page of delivery attempts for one webhook, most
	// recent first, plus the total matching count for pagination.
	ListByWebhook(ctx context.Context, webhookID string, limit, offset int) ([]WebhookDelivery, int, error)
}

// BookingLookup resolves partner bookings for dispatch event mapping and updates
// booking status, plus the rider/vehicle snapshot on trip.started.
type BookingLookup interface {
	FindByDispatchBookingID(ctx context.Context, dispatchBookingID string) (*delivery.PartnerBooking, error)
	UpdateStatus(ctx context.Context, bookingID, status string) error
	UpdateRiderSnapshotAndStatus(ctx context.Context, bookingID string, snap delivery.RiderSnapshot, status string) error
}
