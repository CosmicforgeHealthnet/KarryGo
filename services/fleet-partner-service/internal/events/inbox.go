package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

// Inbox statuses.
const (
	InboxStatusProcessing = "PROCESSING"
	InboxStatusProcessed  = "PROCESSED"
	InboxStatusFailed     = "FAILED"
)

// ErrDuplicateEvent means this consumer already fully processed the event;
// the delivery should be acknowledged without reprocessing.
var ErrDuplicateEvent = errors.New("inbox: event already processed")

// ErrPayloadMismatch means the same event ID arrived with a different
// payload — a contract or security error that must never be processed.
var ErrPayloadMismatch = errors.New("inbox: payload hash mismatch for existing event id")

// InboxRepository deduplicates consumed events per consumer.
type InboxRepository struct {
	db database.Querier
}

// NewInboxRepository wires the repository over a pool or transaction.
func NewInboxRepository(db database.Querier) *InboxRepository {
	return &InboxRepository{db: db}
}

// HashPayload produces the canonical payload hash stored with each entry.
func HashPayload(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// Begin registers a delivery attempt inside the caller's transaction.
//   - First delivery inserts a PROCESSING row and returns nil.
//   - A redelivery of a PROCESSED event returns ErrDuplicateEvent.
//   - A redelivery of a FAILED/PROCESSING event (crash recovery, retry)
//     re-claims the row and returns nil.
//   - The same event ID with a different payload returns ErrPayloadMismatch.
//
// Concurrent duplicate deliveries serialize on the primary-key row lock, so
// exactly one transaction proceeds at a time.
func (r *InboxRepository) Begin(ctx context.Context, q database.Querier, consumerName string, eventID string, eventType string, schemaVersion int, payload []byte) error {
	payloadHash := HashPayload(payload)

	var status, storedHash string
	err := q.QueryRow(ctx, `
		SELECT status, payload_hash FROM fleet_partner_inbox
		WHERE consumer_name = $1 AND event_id = $2
		FOR UPDATE`,
		consumerName, eventID,
	).Scan(&status, &storedHash)

	switch {
	case err == nil:
		if storedHash != payloadHash {
			return ErrPayloadMismatch
		}
		if status == InboxStatusProcessed {
			return ErrDuplicateEvent
		}
		_, err = q.Exec(ctx, `
			UPDATE fleet_partner_inbox SET status = $1, error_code = NULL
			WHERE consumer_name = $2 AND event_id = $3`,
			InboxStatusProcessing, consumerName, eventID,
		)
		if err != nil {
			return fmt.Errorf("inbox: reclaim entry: %w", err)
		}
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		_, err = q.Exec(ctx, `
			INSERT INTO fleet_partner_inbox
				(consumer_name, event_id, event_type, schema_version, payload_hash, received_at, status)
			VALUES ($1, $2, $3, $4, $5, now(), $6)`,
			consumerName, eventID, eventType, schemaVersion, payloadHash, InboxStatusProcessing,
		)
		if err != nil {
			return fmt.Errorf("inbox: insert entry: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("inbox: lookup entry: %w", err)
	}
}

// MarkProcessed completes the entry inside the same transaction as the
// business effect, so an event is never marked successful before its local
// transaction commits.
func (r *InboxRepository) MarkProcessed(ctx context.Context, q database.Querier, consumerName string, eventID string) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_inbox SET status = $1, processed_at = now(), error_code = NULL
		WHERE consumer_name = $2 AND event_id = $3`,
		InboxStatusProcessed, consumerName, eventID,
	)
	if err != nil {
		return fmt.Errorf("inbox: mark processed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("inbox: entry %s/%s not found", consumerName, eventID)
	}
	return nil
}

// MarkFailed records a recoverable failure with a stable error code. Runs
// outside the failed business transaction (which rolled back).
func (r *InboxRepository) MarkFailed(ctx context.Context, consumerName string, eventID string, errorCode string) error {
	if len(errorCode) > 100 {
		errorCode = errorCode[:100]
	}
	_, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_inbox SET status = $1, error_code = $2
		WHERE consumer_name = $3 AND event_id = $4`,
		InboxStatusFailed, errorCode, consumerName, eventID,
	)
	if err != nil {
		return fmt.Errorf("inbox: mark failed: %w", err)
	}
	return nil
}
