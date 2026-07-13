package infra

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/webhooks"
)

// WebhookDeliveryRepo is the Postgres implementation of
// webhooks.WebhookDeliveryRepository.
type WebhookDeliveryRepo struct {
	pool *pgxpool.Pool
}

// NewWebhookDeliveryRepo constructs a WebhookDeliveryRepo.
func NewWebhookDeliveryRepo(pool *pgxpool.Pool) *WebhookDeliveryRepo {
	return &WebhookDeliveryRepo{pool: pool}
}

const webhookDeliveryColumns = `id, webhook_id, booking_id, event_type, payload, attempts,
	last_attempt_at, next_attempt_at, status, response_status, created_at`

func (r *WebhookDeliveryRepo) CreateDelivery(ctx context.Context, d *webhooks.WebhookDelivery) (*webhooks.WebhookDelivery, error) {
	const q = `INSERT INTO partner_webhook_deliveries
		(id, webhook_id, booking_id, event_type, payload, attempts, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := r.pool.Exec(ctx, q, d.ID, d.WebhookID, d.BookingID, d.EventType, d.Payload, d.Attempts, d.Status, d.CreatedAt)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (r *WebhookDeliveryRepo) MarkDelivered(ctx context.Context, deliveryID string, responseStatus int) error {
	const q = `UPDATE partner_webhook_deliveries
		SET status = 'delivered', response_status = $2, last_attempt_at = now(), attempts = attempts + 1
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, deliveryID, responseStatus)
	return err
}

func (r *WebhookDeliveryRepo) MarkFailedAttempt(ctx context.Context, deliveryID string, attempts int, responseStatus *int, nextAttemptAt *time.Time) error {
	const q = `UPDATE partner_webhook_deliveries
		SET status = 'pending', attempts = $2, response_status = $3, next_attempt_at = $4, last_attempt_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, deliveryID, attempts, responseStatus, nextAttemptAt)
	return err
}

func (r *WebhookDeliveryRepo) MarkFailedPermanent(ctx context.Context, deliveryID string, responseStatus *int) error {
	const q = `UPDATE partner_webhook_deliveries
		SET status = 'failed_permanent', response_status = $2, next_attempt_at = NULL, last_attempt_at = now(),
		    attempts = attempts + 1
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, deliveryID, responseStatus)
	return err
}

func (r *WebhookDeliveryRepo) FindPendingDueDeliveries(ctx context.Context, limit int) ([]webhooks.WebhookDelivery, error) {
	const q = `SELECT ` + webhookDeliveryColumns + ` FROM partner_webhook_deliveries
		WHERE status = 'pending' AND (next_attempt_at IS NULL OR next_attempt_at <= now())
		ORDER BY created_at ASC LIMIT $1`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []webhooks.WebhookDelivery
	for rows.Next() {
		var d webhooks.WebhookDelivery
		if err := rows.Scan(
			&d.ID, &d.WebhookID, &d.BookingID, &d.EventType, &d.Payload, &d.Attempts,
			&d.LastAttemptAt, &d.NextAttemptAt, &d.Status, &d.ResponseStatus, &d.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListByWebhook returns a page of delivery attempts for one webhook, most
// recent first, plus the total matching count for pagination. This is new: it's
// what actually powers the dashboard's webhook delivery log/history screen -
// every other method on this repo only writes rows, this is the first one that
// reads them back for display.
func (r *WebhookDeliveryRepo) ListByWebhook(ctx context.Context, webhookID string, limit, offset int) ([]webhooks.WebhookDelivery, int, error) {
	var total int
	const countQ = `SELECT count(*) FROM partner_webhook_deliveries WHERE webhook_id = $1`
	if err := r.pool.QueryRow(ctx, countQ, webhookID).Scan(&total); err != nil {
		return nil, 0, err
	}

	const q = `SELECT ` + webhookDeliveryColumns + ` FROM partner_webhook_deliveries
		WHERE webhook_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, webhookID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []webhooks.WebhookDelivery
	for rows.Next() {
		var d webhooks.WebhookDelivery
		if err := rows.Scan(
			&d.ID, &d.WebhookID, &d.BookingID, &d.EventType, &d.Payload, &d.Attempts,
			&d.LastAttemptAt, &d.NextAttemptAt, &d.Status, &d.ResponseStatus, &d.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}
