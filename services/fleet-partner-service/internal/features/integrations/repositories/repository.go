// Package repositories persists organization API clients, webhook
// subscriptions, deliveries, fanout claims and integration command
// idempotency. Methods taking a database.Querier run inside the caller's
// transaction; nil means the pool.
package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/integrations/models"
)

// Repository is the PostgreSQL integrations store.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository builds the store.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.pool
	}
	return q
}

// WithinTx runs f in one transaction.
func (r *Repository) WithinTx(ctx context.Context, f func(pgx.Tx) error) error {
	return database.WithinTx(ctx, r.pool, f)
}

// Configured reports whether the store has a database.
func (r *Repository) Configured() bool { return r != nil && r.pool != nil }

// ClaimCommand records an integration command once per idempotency key.
// It returns the resource the key is bound to and whether this is a replay;
// a key reused with different input is reported through fingerprintMismatch.
func (r *Repository) ClaimCommand(ctx context.Context, q database.Querier, org, command, key, fingerprint, resourceID string) (resource string, replay bool, fingerprintMismatch bool, err error) {
	err = r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_integration_commands(organization_id,command_type,idempotency_key,request_fingerprint,resource_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING RETURNING resource_id`, org, command, key, fingerprint, resourceID).Scan(&resource)
	if err == nil {
		return resource, false, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, false, err
	}
	var storedFingerprint string
	if err = r.q(q).QueryRow(ctx, `SELECT request_fingerprint,resource_id FROM fleet_partner_integration_commands WHERE organization_id=$1 AND command_type=$2 AND idempotency_key=$3`, org, command, key).Scan(&storedFingerprint, &resource); err != nil {
		return "", false, false, err
	}
	if storedFingerprint != fingerprint {
		return "", false, true, nil
	}
	return resource, true, false, nil
}

type scanner interface{ Scan(...any) error }

// ---------------------------------------------------------------------------
// API clients
// ---------------------------------------------------------------------------

const apiClientSelect = `SELECT id,organization_id,name,client_id,scopes,ip_allowlist::text[],status,expires_at,last_used_at,rotated_at,revoked_at,created_by,secret_version,version,created_at,updated_at FROM fleet_partner_organization_api_clients`

func scanAPIClient(row scanner) (models.APIClient, error) {
	var c models.APIClient
	err := row.Scan(&c.ID, &c.OrganizationID, &c.Name, &c.ClientID, &c.Scopes, &c.IPAllowlist, &c.Status, &c.ExpiresAt, &c.LastUsedAt, &c.RotatedAt, &c.RevokedAt, &c.CreatedBy, &c.SecretVersion, &c.Version, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// GetAPIClient reads one API client.
func (r *Repository) GetAPIClient(ctx context.Context, q database.Querier, org, id string) (models.APIClient, error) {
	return scanAPIClient(r.q(q).QueryRow(ctx, apiClientSelect+` WHERE organization_id=$1 AND id=$2`, org, id))
}

// InsertAPIClient creates an active API client with its hashed secret.
func (r *Repository) InsertAPIClient(ctx context.Context, q database.Querier, c models.APIClient, secretHash []byte) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_organization_api_clients
			(id,organization_id,name,client_id,secret_hash,secret_version,scopes,ip_allowlist,status,expires_at,created_by)
			VALUES($1,$2,$3,$4,$5,1,$6,$7,'ACTIVE',$8,$9)`, c.ID, c.OrganizationID, c.Name,
		c.ClientID, secretHash, c.Scopes, c.IPAllowlist, c.ExpiresAt, c.CreatedBy)
	return err
}

// ListAPIClients lists the organization's API clients, newest first.
func (r *Repository) ListAPIClients(ctx context.Context, org string) ([]models.APIClient, error) {
	rows, err := r.pool.Query(ctx, apiClientSelect+` WHERE organization_id=$1 ORDER BY created_at DESC`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.APIClient
	for rows.Next() {
		item, scanErr := scanAPIClient(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// LockActiveAPIClientSecret reads an active client's secret hash and version
// at the expected version FOR UPDATE.
func (r *Repository) LockActiveAPIClientSecret(ctx context.Context, q database.Querier, org, id string, expected int64) ([]byte, int, error) {
	var previous []byte
	var previousVersion int
	err := r.q(q).QueryRow(ctx, `SELECT secret_hash,secret_version FROM fleet_partner_organization_api_clients
			WHERE organization_id=$1 AND id=$2 AND status='ACTIVE' AND version=$3 FOR UPDATE`, org, id, expected).Scan(&previous, &previousVersion)
	return previous, previousVersion, err
}

// RotateAPIClientSecret replaces the secret, keeping the previous one valid
// until prevUntil when given, and reports whether a row changed.
func (r *Repository) RotateAPIClientSecret(ctx context.Context, q database.Querier, org, id string, expected int64, secretHash []byte, prevHash, prevVersion, prevUntil any) (bool, error) {
	tag, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_organization_api_clients SET
			secret_hash=$1,secret_version=secret_version+1,previous_secret_hash=$2,previous_secret_version=$3,
			previous_secret_valid_until=$4,rotated_at=now(),updated_at=now(),version=version+1
			WHERE organization_id=$5 AND id=$6 AND status='ACTIVE' AND version=$7`, secretHash,
		prevHash, prevVersion, prevUntil, org, id, expected)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// RevokeAPIClient revokes an active client at the expected version and
// reports whether a row changed.
func (r *Repository) RevokeAPIClient(ctx context.Context, q database.Querier, org, id string, expected int64, revokedHash []byte) (bool, error) {
	tag, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_organization_api_clients SET status='REVOKED',revoked_at=now(),
			secret_hash=$1,previous_secret_hash=NULL,
			previous_secret_version=NULL,previous_secret_valid_until=NULL,updated_at=now(),version=version+1
			WHERE organization_id=$2 AND id=$3 AND status='ACTIVE' AND version=$4`, revokedHash, org, id, expected)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ClientCredential reads what authentication needs for one client ID.
func (r *Repository) ClientCredential(ctx context.Context, clientID string) (models.ClientCredential, error) {
	var c models.ClientCredential
	err := r.pool.QueryRow(ctx, `SELECT id,client_id,organization_id,status,expires_at,scopes,ip_allowlist::text[],
		secret_hash,secret_version,previous_secret_hash,previous_secret_version,previous_secret_valid_until
		FROM fleet_partner_organization_api_clients WHERE client_id=$1`, clientID).Scan(
		&c.APIClientID, &c.ClientID, &c.OrganizationID, &c.Status, &c.ExpiresAt, &c.Scopes,
		&c.IPAllowlist, &c.SecretHash, &c.SecretVersion, &c.PreviousHash, &c.PreviousVersion, &c.PreviousValidUntil)
	return c, err
}

// TouchAPIClient records use, at most every thirty seconds.
func (r *Repository) TouchAPIClient(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE fleet_partner_organization_api_clients SET last_used_at=now()
		WHERE id=$1 AND status='ACTIVE' AND (last_used_at IS NULL OR last_used_at < now()-interval '30 seconds')`, id)
	return err
}

// ---------------------------------------------------------------------------
// Webhook subscriptions
// ---------------------------------------------------------------------------

const webhookSelect = `SELECT id,organization_id,endpoint_url,event_types,status,failure_count,disabled_at,created_by,secret_version,version,created_at,updated_at FROM fleet_partner_webhook_subscriptions`

func scanWebhook(row scanner) (models.WebhookSubscription, error) {
	var s models.WebhookSubscription
	err := row.Scan(&s.ID, &s.OrganizationID, &s.EndpointURL, &s.EventTypes, &s.Status, &s.FailureCount, &s.DisabledAt, &s.CreatedBy, &s.SecretVersion, &s.Version, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

// GetWebhook reads one subscription.
func (r *Repository) GetWebhook(ctx context.Context, q database.Querier, org, id string) (models.WebhookSubscription, error) {
	return scanWebhook(r.q(q).QueryRow(ctx, webhookSelect+` WHERE organization_id=$1 AND id=$2`, org, id))
}

// InsertWebhook creates an active subscription with its encrypted secret.
func (r *Repository) InsertWebhook(ctx context.Context, q database.Querier, s models.WebhookSubscription, ciphertext []byte) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_webhook_subscriptions
			(id,organization_id,endpoint_url,event_types,secret_ciphertext,secret_version,status,created_by)
			VALUES($1,$2,$3,$4,$5,1,'ACTIVE',$6)`, s.ID, s.OrganizationID, s.EndpointURL, s.EventTypes, ciphertext, s.CreatedBy)
	return err
}

// UpdateWebhook rewrites a subscription at the expected version and reports
// whether a row changed.
func (r *Repository) UpdateWebhook(ctx context.Context, q database.Querier, org, id, endpoint string, eventTypes []string, status string, expected int64) (bool, error) {
	tag, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_webhook_subscriptions SET endpoint_url=$1,event_types=$2,status=$3,
			disabled_at=CASE WHEN $3='DISABLED' THEN COALESCE(disabled_at,now()) ELSE NULL END,updated_at=now(),version=version+1
			WHERE organization_id=$4 AND id=$5 AND version=$6`, endpoint, eventTypes, status, org, id, expected)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// LockActiveWebhookSecret reads an active subscription's secret at the
// expected version FOR UPDATE.
func (r *Repository) LockActiveWebhookSecret(ctx context.Context, q database.Querier, org, id string, expected int64) ([]byte, int, error) {
	var previous []byte
	var previousVersion int
	err := r.q(q).QueryRow(ctx, `SELECT secret_ciphertext,secret_version FROM fleet_partner_webhook_subscriptions WHERE organization_id=$1 AND id=$2 AND status='ACTIVE' AND version=$3 FOR UPDATE`, org, id, expected).Scan(&previous, &previousVersion)
	return previous, previousVersion, err
}

// RotateWebhookSecret replaces the secret, keeping the previous one valid
// until prevUntil when given, and reports whether a row changed.
func (r *Repository) RotateWebhookSecret(ctx context.Context, q database.Querier, org, id string, expected int64, ciphertext []byte, prevCipher, prevVersion, prevUntil any) (bool, error) {
	tag, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_webhook_subscriptions SET secret_ciphertext=$1,secret_version=secret_version+1,
			previous_secret_ciphertext=$2,previous_secret_version=$3,previous_secret_valid_until=$4,updated_at=now(),version=version+1
			WHERE organization_id=$5 AND id=$6 AND status='ACTIVE' AND version=$7`, ciphertext, prevCipher, prevVersion, prevUntil, org, id, expected)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ListWebhooks lists subscriptions, newest first.
func (r *Repository) ListWebhooks(ctx context.Context, org string) ([]models.WebhookSubscription, error) {
	rows, err := r.pool.Query(ctx, webhookSelect+` WHERE organization_id=$1 ORDER BY created_at DESC`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.WebhookSubscription
	for rows.Next() {
		item, scanErr := scanWebhook(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Deliveries and fanout
// ---------------------------------------------------------------------------

const deliverySelect = `SELECT id,organization_id,subscription_id,source_event_id,event_type,attempt_no,status,COALESCE(response_status,0),COALESCE(response_body_redacted,''),scheduled_at,delivered_at,next_attempt_at,COALESCE(error_class,''),COALESCE(error_code,''),correlation_id,COALESCE(job_id,''),COALESCE(replay_of_delivery_id,''),version,created_at,updated_at FROM fleet_partner_webhook_deliveries`

func scanDelivery(row scanner) (models.WebhookDelivery, error) {
	var d models.WebhookDelivery
	err := row.Scan(&d.ID, &d.OrganizationID, &d.SubscriptionID, &d.SourceEventID, &d.EventType, &d.AttemptNo, &d.Status, &d.ResponseStatus, &d.ResponseExcerpt, &d.ScheduledAt, &d.DeliveredAt, &d.NextAttemptAt, &d.ErrorClass, &d.ErrorCode, &d.CorrelationID, &d.JobID, &d.ReplayOfDeliveryID, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

// GetDelivery reads one delivery.
func (r *Repository) GetDelivery(ctx context.Context, q database.Querier, org, id string) (models.WebhookDelivery, error) {
	return scanDelivery(r.q(q).QueryRow(ctx, deliverySelect+` WHERE organization_id=$1 AND id=$2`, org, id))
}

// ListDeliveries lists a subscription's latest 200 deliveries.
func (r *Repository) ListDeliveries(ctx context.Context, org, subscriptionID string) ([]models.WebhookDelivery, error) {
	rows, err := r.pool.Query(ctx, deliverySelect+` WHERE organization_id=$1 AND subscription_id=$2 ORDER BY created_at DESC LIMIT 200`, org, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.WebhookDelivery
	for rows.Next() {
		item, scanErr := scanDelivery(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// LockFailedDelivery reads a failed or dead-lettered delivery FOR UPDATE.
func (r *Repository) LockFailedDelivery(ctx context.Context, q database.Querier, org, id string) (models.WebhookDelivery, error) {
	var d models.WebhookDelivery
	err := r.q(q).QueryRow(ctx, `SELECT subscription_id,source_event_id,event_type,correlation_id,attempt_no FROM fleet_partner_webhook_deliveries WHERE organization_id=$1 AND id=$2 AND status IN('FAILED','DEAD_LETTERED') FOR UPDATE`, org, id).Scan(&d.SubscriptionID, &d.SourceEventID, &d.EventType, &d.CorrelationID, &d.AttemptNo)
	return d, err
}

// NextAttempt is the next attempt number for an event's delivery.
func (r *Repository) NextAttempt(ctx context.Context, q database.Querier, subscriptionID, eventID string) (int, error) {
	var attempt int
	err := r.q(q).QueryRow(ctx, `SELECT COALESCE(max(attempt_no),0)+1 FROM fleet_partner_webhook_deliveries WHERE subscription_id=$1 AND source_event_id=$2`, subscriptionID, eventID).Scan(&attempt)
	return attempt, err
}

// InsertReplayDelivery creates a pending replay of a failed delivery.
func (r *Repository) InsertReplayDelivery(ctx context.Context, q database.Querier, id, org string, original models.WebhookDelivery, attempt int, replayOf string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_webhook_deliveries(id,organization_id,subscription_id,source_event_id,event_type,attempt_no,status,scheduled_at,correlation_id,replay_of_delivery_id) VALUES($1,$2,$3,$4,$5,$6,'PENDING',now(),$7,$8)`, id, org, original.SubscriptionID, original.SourceEventID, original.EventType, attempt, original.CorrelationID, replayOf)
	return err
}

// SetDeliveryJob links a delivery to its durable job.
func (r *Repository) SetDeliveryJob(ctx context.Context, q database.Querier, id, jobID string) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_webhook_deliveries SET job_id=$1 WHERE id=$2`, jobID, id)
	return err
}

// PendingPublicEvents lists committed public events not yet fanned out.
func (r *Repository) PendingPublicEvents(ctx context.Context, eventTypes []string, limit int) ([]models.OutboxEvent, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,payload FROM fleet_partner_outbox WHERE event_type=ANY($1)
		AND NOT EXISTS(SELECT 1 FROM fleet_partner_webhook_fanout_events f WHERE f.source_event_id=fleet_partner_outbox.id)
		ORDER BY created_at LIMIT $2`, eventTypes, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.OutboxEvent
	for rows.Next() {
		var item models.OutboxEvent
		if err := rows.Scan(&item.ID, &item.Raw); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ClaimFanout claims an event for fanout once and reports whether this call
// claimed it.
func (r *Repository) ClaimFanout(ctx context.Context, q database.Querier, eventID, org, eventType string) (bool, error) {
	var claimed string
	err := r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_webhook_fanout_events(source_event_id,organization_id,event_type) VALUES($1,$2,$3) ON CONFLICT DO NOTHING RETURNING source_event_id`, eventID, org, eventType).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// ActiveSubscriptionsFor lists active subscriptions to an event type. The
// result set is fully consumed before returning so the caller's transaction
// connection is free for inserts.
func (r *Repository) ActiveSubscriptionsFor(ctx context.Context, q database.Querier, org, eventType string) ([]string, error) {
	rows, err := r.q(q).Query(ctx, `SELECT id FROM fleet_partner_webhook_subscriptions WHERE organization_id=$1 AND status='ACTIVE' AND event_types @> ARRAY[$2]::text[]`, org, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// InsertFirstDelivery creates the first delivery of an event to a
// subscription and reports whether it was created.
func (r *Repository) InsertFirstDelivery(ctx context.Context, q database.Querier, id, org, subscriptionID, eventID, eventType, correlationID string) (bool, error) {
	var inserted string
	err := r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_webhook_deliveries(id,organization_id,subscription_id,source_event_id,event_type,attempt_no,status,scheduled_at,correlation_id) VALUES($1,$2,$3,$4,$5,1,'PENDING',now(),$6) ON CONFLICT(subscription_id,source_event_id,attempt_no) DO NOTHING RETURNING id`, id, org, subscriptionID, eventID, eventType, correlationID).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// DeliveryContext reads everything one delivery attempt needs.
func (r *Repository) DeliveryContext(ctx context.Context, org, id string) (models.DeliveryContext, error) {
	var d models.DeliveryContext
	err := r.pool.QueryRow(ctx, `SELECT d.subscription_id,d.source_event_id,d.event_type,d.correlation_id,d.status,
		s.endpoint_url,s.secret_ciphertext,s.secret_version,s.previous_secret_ciphertext,s.previous_secret_version,s.previous_secret_valid_until,o.payload
		FROM fleet_partner_webhook_deliveries d JOIN fleet_partner_webhook_subscriptions s ON s.id=d.subscription_id AND s.organization_id=d.organization_id
		JOIN fleet_partner_outbox o ON o.id=d.source_event_id WHERE d.organization_id=$1 AND d.id=$2 AND s.status='ACTIVE'`, org, id).Scan(&d.SubscriptionID, &d.EventID, &d.EventType, &d.CorrelationID, &d.Status, &d.Endpoint, &d.CurrentCipher, &d.CurrentVersion, &d.PreviousCipher, &d.PreviousVersion, &d.PreviousUntil, &d.RawEvent)
	return d, err
}

// MarkDelivering records that an attempt is in flight.
func (r *Repository) MarkDelivering(ctx context.Context, org, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE fleet_partner_webhook_deliveries SET status='DELIVERING',updated_at=now(),version=version+1 WHERE organization_id=$1 AND id=$2`, org, id)
	return err
}

// MarkDeliverySucceeded records a successful attempt and reports whether a
// row changed.
func (r *Repository) MarkDeliverySucceeded(ctx context.Context, q database.Querier, org, id string, status int, excerpt string) (bool, error) {
	tag, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_webhook_deliveries SET status='SUCCEEDED',response_status=$1,response_body_redacted=$2,delivered_at=now(),next_attempt_at=NULL,error_class=NULL,error_code=NULL,updated_at=now(),version=version+1 WHERE organization_id=$3 AND id=$4`, status, excerpt, org, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ResetFailureCount clears a delivery's subscription failure count.
func (r *Repository) ResetFailureCount(ctx context.Context, q database.Querier, org, deliveryID string) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_webhook_subscriptions SET failure_count=0,updated_at=now() WHERE organization_id=$1 AND id=(SELECT subscription_id FROM fleet_partner_webhook_deliveries WHERE id=$2)`, org, deliveryID)
	return err
}

// MarkDeliveryFailed records a failed attempt; next is nil for a terminal one.
func (r *Repository) MarkDeliveryFailed(ctx context.Context, q database.Querier, org, id, status string, responseStatus int, next any, class, code string) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_webhook_deliveries SET status=$1,response_status=NULLIF($2,0),response_body_redacted='response body omitted',next_attempt_at=$3,error_class=$4,error_code=$5,updated_at=now(),version=version+1 WHERE organization_id=$6 AND id=$7`, status, responseStatus, next, class, code, org, id)
	return err
}

// RecordSubscriptionFailure counts a terminal failure against a delivery's
// subscription, disabling it at the threshold. It returns the disabled
// subscription ID ("" when still active) and the subscription version.
func (r *Repository) RecordSubscriptionFailure(ctx context.Context, q database.Querier, threshold int, org, deliveryID string) (string, int64, error) {
	var disabled string
	var version int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_webhook_subscriptions SET failure_count=failure_count+1,status=CASE WHEN failure_count+1 >= $1 THEN 'DISABLED' ELSE status END,disabled_at=CASE WHEN failure_count+1 >= $1 THEN COALESCE(disabled_at,now()) ELSE disabled_at END,updated_at=now(),version=version+1 WHERE organization_id=$2 AND id=(SELECT subscription_id FROM fleet_partner_webhook_deliveries WHERE id=$3) RETURNING CASE WHEN status='DISABLED' THEN id ELSE '' END,version`, threshold, org, deliveryID).Scan(&disabled, &version)
	return disabled, version, err
}

// WebhookAggregates reads the fleet-wide delivery backlog, oldest scheduled
// pending delivery, dead letters and disabled subscriptions.
func (r *Repository) WebhookAggregates(ctx context.Context) (backlog int64, oldest *time.Time, deadLetters, disabled int64, err error) {
	if err = r.pool.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE status IN ('PENDING','DELIVERING','RETRY_SCHEDULED')),
		min(scheduled_at) FILTER (WHERE status IN ('PENDING','DELIVERING','RETRY_SCHEDULED')),
		count(*) FILTER (WHERE status='DEAD_LETTERED')
		FROM fleet_partner_webhook_deliveries`).Scan(&backlog, &oldest, &deadLetters); err != nil {
		return
	}
	err = r.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_webhook_subscriptions WHERE status='DISABLED'`).Scan(&disabled)
	return
}
