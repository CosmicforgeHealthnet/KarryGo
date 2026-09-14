package marketplace

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// This file is Dispatch's operator repair surface over the Fleet delivery
// outbox, mirroring the one Taxi and Hauling already have.
//
// Dispatch had the outbox, the worker, the retry schedule and the DEAD_LETTER
// terminal state, but nothing to look at them with: an operator investigating
// a stuck dispatch event had to open psql. EVT-010 asks for that inspection
// without SQL, and EVT-011 asks for a requeue that preserves the event's
// identity rather than inventing a new one.

// Outbox delivery states, restated here so the operator surface can validate
// a filter without importing the worker's internals.
const (
	OutboxPending    = "PENDING"
	OutboxInFlight   = "IN_FLIGHT"
	OutboxPublished  = "PUBLISHED"
	OutboxFailed     = "FAILED"
	OutboxDeadLetter = "DEAD_LETTER"
)

// ErrNotRequeueable means the row is not in a state a requeue can help: it
// was already delivered, or it does not exist.
var ErrNotRequeueable = errors.New("outbox row is not requeueable")

// OperatorOutboxRow is the operator-facing view of one queued event.
//
// It deliberately carries identifiers, delivery state and the last error, but
// NOT the payload: a dispatch payload can name a customer's address and a
// provider's identity, and an operator inspecting delivery health has no
// business reading either. The payload is what gets redelivered, not what
// gets displayed.
type OperatorOutboxRow struct {
	ID               string     `json:"id"`
	EventType        string     `json:"eventType"`
	AggregateType    string     `json:"aggregateType"`
	AggregateID      string     `json:"aggregateId"`
	AggregateVersion int64      `json:"aggregateVersion"`
	TargetService    string     `json:"targetService"`
	CorrelationID    string     `json:"correlationId"`
	CausationID      string     `json:"causationId"`
	Status           string     `json:"status"`
	AttemptCount     int        `json:"attemptCount"`
	LastError        string     `json:"lastError,omitempty"`
	NextAttemptAt    *time.Time `json:"nextAttemptAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	PublishedAt      *time.Time `json:"publishedAt,omitempty"`
	RequeueCount     int        `json:"requeueCount"`
	LastRequeuedAt   *time.Time `json:"lastRequeuedAt,omitempty"`
	LastRequeuedBy   string     `json:"lastRequeuedBy,omitempty"`
}

// OperatorOutboxStore reads and repairs the delivery outbox.
type OperatorOutboxStore struct{ DB *pgxpool.Pool }

// ValidOutboxStatus reports whether a filter names a state the outbox has.
// An empty filter means "every state".
func ValidOutboxStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "", OutboxPending, OutboxInFlight, OutboxPublished, OutboxFailed, OutboxDeadLetter:
		return true
	default:
		return false
	}
}

// List returns queued events, newest first, optionally filtered by delivery
// state and destination.
func (s *OperatorOutboxStore) List(
	ctx context.Context, status, targetService string, limit int,
) ([]OperatorOutboxRow, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("outbox store is not configured")
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.DB.Query(ctx, `
		SELECT id::text, event_type, aggregate_type, aggregate_id, aggregate_version,
		       target_service, correlation_id, causation_id, status, attempt_count,
		       COALESCE(last_error,''), next_attempt_at, created_at, published_at,
		       requeue_count, last_requeued_at, last_requeued_by
		  FROM dispatch_marketplace_outbox
		 WHERE ($1='' OR status=$1)
		   AND ($2='' OR target_service=$2)
		 ORDER BY created_at DESC
		 LIMIT $3`,
		strings.ToUpper(strings.TrimSpace(status)), strings.TrimSpace(targetService), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]OperatorOutboxRow, 0, limit)
	for rows.Next() {
		var item OperatorOutboxRow
		if err := rows.Scan(&item.ID, &item.EventType, &item.AggregateType,
			&item.AggregateID, &item.AggregateVersion, &item.TargetService,
			&item.CorrelationID, &item.CausationID, &item.Status, &item.AttemptCount,
			&item.LastError, &item.NextAttemptAt, &item.CreatedAt, &item.PublishedAt,
			&item.RequeueCount, &item.LastRequeuedAt, &item.LastRequeuedBy); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// Requeue returns a stuck event to the delivery queue.
//
// Everything that identifies the event is left exactly as it was: the
// payload, its hash, the aggregate version, the correlation and causation
// ids. Only the delivery schedule is reset, and the repair itself is
// recorded. That is what makes this safe against a destination that may
// already hold the event — it receives the same event again and dedupes it,
// rather than a second one it has never seen.
//
// A row that has already been delivered is refused: there is nothing to
// repair, and resetting it would republish a settled fact.
func (s *OperatorOutboxStore) Requeue(ctx context.Context, id, operator string) error {
	if s == nil || s.DB == nil {
		return errors.New("outbox store is not configured")
	}
	if strings.TrimSpace(id) == "" {
		return ErrNotRequeueable
	}
	tag, err := s.DB.Exec(ctx, `
		UPDATE dispatch_marketplace_outbox
		   SET status=$2, attempt_count=0, next_attempt_at=now(),
		       lease_owner=NULL, lease_expires_at=NULL, last_error=NULL,
		       requeue_count=requeue_count+1, last_requeued_at=now(),
		       last_requeued_by=$3
		 WHERE id=$1::uuid AND status <> $4 AND published_at IS NULL`,
		strings.TrimSpace(id), OutboxPending, strings.TrimSpace(operator), OutboxPublished)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotRequeueable
	}
	return nil
}
