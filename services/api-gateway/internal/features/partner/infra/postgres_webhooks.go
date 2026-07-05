package infra

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/webhooks"
)

// WebhookRepo is the Postgres implementation of webhooks.WebhookRepository.
// The signing_secret column stores AES-GCM ENCRYPTED secret material, never
// plaintext (see webhooks.Cryptor and migration 002).
type WebhookRepo struct {
	pool *pgxpool.Pool
}

// NewWebhookRepo constructs a WebhookRepo.
func NewWebhookRepo(pool *pgxpool.Pool) *WebhookRepo { return &WebhookRepo{pool: pool} }

const webhookColumns = `id, partner_id, url, events, signing_secret, status, created_at`

func (r *WebhookRepo) CreateWebhook(ctx context.Context, w *webhooks.Webhook) (*webhooks.Webhook, error) {
	const q = `INSERT INTO partner_webhooks (id, partner_id, url, events, signing_secret, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.pool.Exec(ctx, q, w.ID, w.PartnerID, w.URL, w.Events, w.EncryptedSigningSecret, w.Status, w.CreatedAt)
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (r *WebhookRepo) ListWebhooks(ctx context.Context, partnerID string) ([]webhooks.Webhook, error) {
	q := `SELECT ` + webhookColumns + ` FROM partner_webhooks
		WHERE partner_id = $1 AND status = 'active' ORDER BY created_at DESC`
	return r.queryWebhooks(ctx, q, partnerID)
}

func (r *WebhookRepo) DeleteWebhook(ctx context.Context, partnerID, webhookID string) error {
	if _, err := uuid.Parse(webhookID); err != nil {
		return nil
	}
	const q = `UPDATE partner_webhooks SET status = 'inactive' WHERE partner_id = $1 AND id = $2`
	_, err := r.pool.Exec(ctx, q, partnerID, webhookID)
	return err
}

func (r *WebhookRepo) FindWebhookByPartner(ctx context.Context, partnerID, webhookID string) (*webhooks.Webhook, error) {
	if _, err := uuid.Parse(webhookID); err != nil {
		return nil, nil
	}
	q := `SELECT ` + webhookColumns + ` FROM partner_webhooks WHERE partner_id = $1 AND id = $2`
	return r.oneWebhook(ctx, q, partnerID, webhookID)
}

func (r *WebhookRepo) FindWebhookByID(ctx context.Context, webhookID string) (*webhooks.Webhook, error) {
	if _, err := uuid.Parse(webhookID); err != nil {
		return nil, nil
	}
	q := `SELECT ` + webhookColumns + ` FROM partner_webhooks WHERE id = $1`
	return r.oneWebhook(ctx, q, webhookID)
}

func (r *WebhookRepo) FindActiveWebhooksForEvent(ctx context.Context, partnerID, eventType string) ([]webhooks.Webhook, error) {
	q := `SELECT ` + webhookColumns + ` FROM partner_webhooks
		WHERE partner_id = $1 AND status = 'active' AND $2 = ANY(events)`
	return r.queryWebhooks(ctx, q, partnerID, eventType)
}

func (r *WebhookRepo) oneWebhook(ctx context.Context, q string, args ...any) (*webhooks.Webhook, error) {
	w, err := scanWebhook(r.pool.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (r *WebhookRepo) queryWebhooks(ctx context.Context, q string, args ...any) ([]webhooks.Webhook, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []webhooks.Webhook
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

func scanWebhook(row rowScanner) (*webhooks.Webhook, error) {
	var w webhooks.Webhook
	if err := row.Scan(&w.ID, &w.PartnerID, &w.URL, &w.Events, &w.EncryptedSigningSecret, &w.Status, &w.CreatedAt); err != nil {
		return nil, err
	}
	return &w, nil
}
