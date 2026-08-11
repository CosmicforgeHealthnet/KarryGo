package events

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
	_, err = r.db.Exec(ctx, `
		INSERT INTO vendor_inbox_processed_events (source_service, event_id, event_type, processed_at)
		VALUES ($1, $2, $3, now())`,
		sourceService, eventID, eventType,
	)
	if err == nil {
		return true, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return false, nil
	}
	return false, fmt.Errorf("inbox: claim event: %w", err)
}
