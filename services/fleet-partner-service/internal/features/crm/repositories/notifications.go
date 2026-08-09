package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
)

const notificationColumns = `id, organization_id, subject_type, subject_id, intent_type,
	recipient_reference, channel, payload, idempotency_key, status, attempts, next_attempt_at,
	COALESCE(lease_owner, ''), lease_expires_at, delivery_reference, safe_error_code,
	dead_letter_reason, correlation_id, causation_id, version, created_at, updated_at`

func scanNotification(row pgx.Row) (models.NotificationOperation, error) {
	var operation models.NotificationOperation
	var payload []byte
	err := row.Scan(&operation.ID, &operation.OrganizationID, &operation.SubjectType,
		&operation.SubjectID, &operation.IntentType, &operation.RecipientReference,
		&operation.Channel, &payload, &operation.IdempotencyKey, &operation.Status,
		&operation.Attempts, &operation.NextAttemptAt, &operation.LeaseOwner,
		&operation.LeaseExpiresAt, &operation.DeliveryReference, &operation.SafeErrorCode,
		&operation.DeadLetterReason, &operation.CorrelationID, &operation.CausationID,
		&operation.Version, &operation.CreatedAt, &operation.UpdatedAt)
	if err != nil {
		return operation, scanErr(err)
	}
	operation.Payload = decodeJSONMap(payload)
	return operation, nil
}

// EnqueueNotification records a durable intent to notify somebody.
//
// It is written inside the same transaction as the domain change it announces.
// The delivery itself is not: an outage at notification-service must not roll
// back an issued quotation or an activated contract, so the intent survives and
// the worker keeps trying.
//
// The unique (organization_id, idempotency_key) index means the same domain
// event can never produce two deliveries; a replay returns the stored row.
func (r *Repository) EnqueueNotification(ctx context.Context, q database.Querier,
	operation models.NotificationOperation) (models.NotificationOperation, error) {
	payload, err := encodeJSON(operation.Payload)
	if err != nil {
		return models.NotificationOperation{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_crm_notification_operations
			(id, organization_id, subject_type, subject_id, intent_type, recipient_reference,
			 channel, payload, idempotency_key, status, correlation_id, causation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'PENDING', $10, $11)
		ON CONFLICT (organization_id, idempotency_key) DO UPDATE
			SET updated_at = fleet_partner_crm_notification_operations.updated_at
		RETURNING `+notificationColumns,
		operation.ID, operation.OrganizationID, operation.SubjectType, operation.SubjectID,
		operation.IntentType, operation.RecipientReference, operation.Channel, payload,
		operation.IdempotencyKey, operation.CorrelationID, operation.CausationID)
	created, err := scanNotification(row)
	if err != nil {
		return created, classify(err, "enqueue notification")
	}
	return created, nil
}

// ClaimNotifications leases due notification operations for one worker.
//
// The lease is taken with SKIP LOCKED and a compare on the lease expiry, so a
// second worker never picks up work in flight, and a worker that crashed after
// claiming has its rows returned to the pool once the lease lapses rather than
// leaving them stuck forever.
func (r *Repository) ClaimNotifications(ctx context.Context, owner string,
	leaseFor time.Duration, limit int) ([]models.NotificationOperation, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE fleet_partner_crm_notification_operations SET
			status = 'PROCESSING', lease_owner = $1,
			lease_expires_at = now() + $2::interval,
			attempts = attempts + 1, version = version + 1, updated_at = now()
		WHERE id IN (
			SELECT id FROM fleet_partner_crm_notification_operations
			WHERE status IN ('PENDING', 'FAILED_RETRYABLE', 'UNKNOWN')
			  AND next_attempt_at <= now()
			  AND (lease_expires_at IS NULL OR lease_expires_at < now())
			ORDER BY next_attempt_at ASC, id ASC
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+notificationColumns,
		owner, fmt.Sprintf("%d seconds", int64(leaseFor.Seconds())), cursor.ClampLimit(limit))
	if err != nil {
		return nil, classify(err, "claim notifications")
	}
	defer rows.Close()
	out := make([]models.NotificationOperation, 0)
	for rows.Next() {
		value, scanError := scanNotification(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// RecoverExpiredNotificationLeases returns abandoned work to the pool.
func (r *Repository) RecoverExpiredNotificationLeases(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_crm_notification_operations SET
			status = 'FAILED_RETRYABLE', lease_owner = NULL, lease_expires_at = NULL,
			version = version + 1, updated_at = now()
		WHERE status = 'PROCESSING' AND lease_expires_at IS NOT NULL
		  AND lease_expires_at < now()`)
	if err != nil {
		return 0, classify(err, "recover notification leases")
	}
	return tag.RowsAffected(), nil
}

// HeartbeatNotification extends a lease held by this worker.
func (r *Repository) HeartbeatNotification(ctx context.Context, owner, operationID string,
	leaseFor time.Duration) error {
	_, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_crm_notification_operations
		SET lease_expires_at = now() + $3::interval, updated_at = now()
		WHERE id = $2 AND lease_owner = $1`,
		owner, operationID, fmt.Sprintf("%d seconds", int64(leaseFor.Seconds())))
	return classify(err, "heartbeat notification")
}

// MarkNotificationDelivered records a confirmed delivery.
func (r *Repository) MarkNotificationDelivered(ctx context.Context, owner, operationID,
	deliveryReference string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_crm_notification_operations SET
			status = 'DELIVERED', delivery_reference = $3, safe_error_code = '',
			lease_owner = NULL, lease_expires_at = NULL,
			version = version + 1, updated_at = now()
		WHERE id = $2 AND lease_owner = $1`, owner, operationID, deliveryReference)
	return classify(err, "mark notification delivered")
}

// MarkNotificationUnknown records an ambiguous outcome.
//
// An unknown outcome is not a failure and is emphatically not a success: the
// send may have landed. It stays retryable so the next pass can read back
// rather than being written off either way.
func (r *Repository) MarkNotificationUnknown(ctx context.Context, owner, operationID,
	safeErrorCode string, backoff time.Duration) error {
	_, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_crm_notification_operations SET
			status = 'UNKNOWN', safe_error_code = $3,
			next_attempt_at = now() + $4::interval,
			lease_owner = NULL, lease_expires_at = NULL,
			version = version + 1, updated_at = now()
		WHERE id = $2 AND lease_owner = $1`,
		owner, operationID, safeErrorCode,
		fmt.Sprintf("%d seconds", int64(backoff.Seconds())))
	return classify(err, "mark notification unknown")
}

// MarkNotificationRetryable schedules another attempt, or dead-letters when the
// attempt budget is exhausted.
func (r *Repository) MarkNotificationRetryable(ctx context.Context, owner, operationID,
	safeErrorCode string, backoff time.Duration, maxAttempts int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_crm_notification_operations SET
			status = CASE WHEN attempts >= $5 THEN 'DEAD_LETTER' ELSE 'FAILED_RETRYABLE' END,
			dead_letter_reason = CASE WHEN attempts >= $5 THEN $3 ELSE dead_letter_reason END,
			safe_error_code = $3,
			next_attempt_at = now() + $4::interval,
			lease_owner = NULL, lease_expires_at = NULL,
			version = version + 1, updated_at = now()
		WHERE id = $2 AND lease_owner = $1`,
		owner, operationID, safeErrorCode,
		fmt.Sprintf("%d seconds", int64(backoff.Seconds())), maxAttempts)
	return classify(err, "mark notification retryable")
}

// MarkNotificationPermanent dead-letters an operation that can never succeed.
func (r *Repository) MarkNotificationPermanent(ctx context.Context, owner, operationID,
	safeErrorCode string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_crm_notification_operations SET
			status = 'DEAD_LETTER', safe_error_code = $3, dead_letter_reason = $3,
			lease_owner = NULL, lease_expires_at = NULL,
			version = version + 1, updated_at = now()
		WHERE id = $2 AND lease_owner = $1`, owner, operationID, safeErrorCode)
	return classify(err, "mark notification permanent")
}

// ReplayNotification returns a dead-lettered operation to the queue.
//
// The identity is preserved: the same row, the same idempotency key, the same
// payload. An operator repairing a downstream outage replays the original
// intent rather than minting a new one that could deliver twice.
func (r *Repository) ReplayNotification(ctx context.Context, q database.Querier,
	organizationID, operationID string) (models.NotificationOperation, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_crm_notification_operations SET
			status = 'PENDING', attempts = 0, next_attempt_at = now(),
			lease_owner = NULL, lease_expires_at = NULL, safe_error_code = '',
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2
		  AND status IN ('DEAD_LETTER', 'FAILED_PERMANENT')
		RETURNING `+notificationColumns, organizationID, operationID)
	replayed, err := scanNotification(row)
	if err != nil {
		return replayed, classify(err, "replay notification")
	}
	return replayed, nil
}

// GetNotification reads one notification operation.
func (r *Repository) GetNotification(ctx context.Context, q database.Querier,
	organizationID, operationID string) (models.NotificationOperation, error) {
	return scanNotification(r.q(q).QueryRow(ctx, `SELECT `+notificationColumns+`
		FROM fleet_partner_crm_notification_operations
		WHERE organization_id = $1 AND id = $2`, organizationID, operationID))
}

// ListNotifications returns a tenant-scoped page of notification operations.
func (r *Repository) ListNotifications(ctx context.Context, organizationID, status,
	subjectID string, page cursor.Cursor, hasCursor bool,
	limit int) ([]models.NotificationOperation, error) {
	args := []any{organizationID, status, subjectID}
	where := `organization_id = $1 AND ($2 = '' OR status = $2) AND ($3 = '' OR subject_id = $3)`
	where, args = pageWhere(where, args, page, hasCursor, "created_at")
	args = append(args, cursor.ClampLimit(limit))

	rows, err := r.db.Query(ctx, `SELECT `+notificationColumns+`
		FROM fleet_partner_crm_notification_operations WHERE `+where+`
		ORDER BY created_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list notifications")
	}
	defer rows.Close()
	out := make([]models.NotificationOperation, 0)
	for rows.Next() {
		value, scanError := scanNotification(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// NotificationBacklog reports the retry backlog and dead-letter depth.
func (r *Repository) NotificationBacklog(ctx context.Context) (int64, int64, error) {
	var backlog, deadLetters int64
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status IN ('PENDING', 'FAILED_RETRYABLE', 'UNKNOWN')),
		       count(*) FILTER (WHERE status = 'DEAD_LETTER')
		FROM fleet_partner_crm_notification_operations`).Scan(&backlog, &deadLetters)
	if err != nil {
		return 0, 0, classify(err, "notification backlog")
	}
	return backlog, deadLetters, nil
}
