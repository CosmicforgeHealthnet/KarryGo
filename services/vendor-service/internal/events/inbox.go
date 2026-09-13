package events

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrPayloadMismatch means a producer reused an event identity for different
// content. Treating that as an ordinary duplicate would hide corruption.
var ErrPayloadMismatch = errors.New("inbox: event identity reused with different payload")

// InboxRepository deduplicates inbound events by (source_service, event_id),
// so redelivery from an at-least-once transport is a safe no-op. Any handler
// consuming an event from another service (wallet settlement callbacks,
// dispatch/hauling/taxi mission lifecycle events, verification decisions,
// etc.) must call Claim before acting and only act if claimed == true.
type InboxRepository struct {
	db *pgxpool.Pool
}

func NewInboxRepository(db *pgxpool.Pool) *InboxRepository {
	return &InboxRepository{db: db}
}

// Claim records the event as processed. Returns claimed=true the first time
// an (sourceService, eventID) pair is seen, and false on every redelivery.
func (r *InboxRepository) Claim(ctx context.Context, sourceService, eventID, eventType string) (claimed bool, err error) {
	return r.ClaimTx(ctx, r.db, sourceService, eventID, eventType, "")
}

// ClaimTx claims an event through the caller's transaction. The consumer must
// perform its business mutation through the same transaction, so a rollback
// also rolls back the inbox marker and leaves the delivery replayable.
// payloadFingerprint should be a canonical, secret-free digest when supplied.
func (r *InboxRepository) ClaimTx(ctx context.Context, q querier, sourceService, eventID, eventType, payloadFingerprint string) (claimed bool, err error) {
	var inserted int
	err = q.QueryRow(ctx, `
		INSERT INTO vendor_inbox_processed_events
			(source_service, event_id, event_type, processed_at, payload_fingerprint)
		VALUES ($1, $2, $3, now(), NULLIF($4, ''))
		ON CONFLICT (source_service,event_id) DO NOTHING
		RETURNING 1`,
		sourceService, eventID, eventType, payloadFingerprint,
	).Scan(&inserted)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		var storedType string
		var storedFingerprint *string
		if loadErr := q.QueryRow(ctx, `
			SELECT event_type, payload_fingerprint
			FROM vendor_inbox_processed_events
			WHERE source_service=$1 AND event_id=$2`, sourceService, eventID).
			Scan(&storedType, &storedFingerprint); loadErr != nil {
			return false, fmt.Errorf("inbox: inspect duplicate: %w", loadErr)
		}
		if storedType != eventType ||
			(payloadFingerprint != "" && (storedFingerprint == nil || *storedFingerprint != payloadFingerprint)) {
			return false, ErrPayloadMismatch
		}
		return false, nil
	}
	return false, fmt.Errorf("inbox: claim event: %w", err)
}
