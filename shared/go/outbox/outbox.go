// Package outbox is a shared transactional outbox.
//
// A producer appends an event in the SAME database transaction as the state
// change the event describes, so "state changed" and "event recorded" commit
// together or not at all. A relay Worker later claims due rows with
// FOR UPDATE SKIP LOCKED and hands them to a Publisher (normally the signed
// HTTPPublisher). Delivery is at-least-once: consumers must dedupe on the
// event ID, and anything involving money must also dedupe on a business key.
//
// Each service owns its own outbox table and creates it in its own migration.
// The table must have this shape (the name is the service's choice):
//
//	CREATE TABLE <table> (
//	    id                  text PRIMARY KEY,
//	    event_type          text NOT NULL,
//	    schema_version      int  NOT NULL CHECK (schema_version > 0),
//	    aggregate_type      text NOT NULL,
//	    aggregate_id        text NOT NULL,
//	    payload             jsonb NOT NULL,
//	    status              text NOT NULL DEFAULT 'PENDING'
//	                            CHECK (status IN ('PENDING', 'PUBLISHED', 'DEAD_LETTERED')),
//	    available_at        timestamptz NOT NULL DEFAULT now(),
//	    attempts            int NOT NULL DEFAULT 0 CHECK (attempts >= 0),
//	    lease_owner         text,
//	    lease_expires_at    timestamptz,
//	    created_at          timestamptz NOT NULL DEFAULT now(),
//	    published_at        timestamptz,
//	    last_error_redacted text NOT NULL DEFAULT ''
//	);
//	CREATE INDEX <table>_due_idx ON <table> (status, available_at) WHERE status = 'PENDING';
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Outbox row statuses.
const (
	StatusPending      = "PENDING"
	StatusPublished    = "PUBLISHED"
	StatusDeadLettered = "DEAD_LETTERED"
)

// DefaultMaxAttempts bounds publication retries before a row is
// dead-lettered. With the capped exponential backoff below this is roughly
// eight hours of retrying, long enough to ride out a consumer outage.
const DefaultMaxAttempts = 100

// Querier is the subset of pgx used here. *pgxpool.Pool and pgx.Tx both
// satisfy it, which is what lets Append run inside the caller's transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Envelope is the self-describing wire form of every event. The whole
// envelope is what gets stored and delivered, so a consumer never needs the
// outbox row to interpret an event.
type Envelope struct {
	EventID          string          `json:"eventId"`
	EventType        string          `json:"eventType"`
	SchemaVersion    int             `json:"schemaVersion"`
	AggregateType    string          `json:"aggregateType"`
	AggregateID      string          `json:"aggregateId"`
	AggregateVersion int64           `json:"aggregateVersion"`
	CorrelationID    string          `json:"correlationId,omitempty"`
	CausationID      string          `json:"causationId,omitempty"`
	OccurredAt       time.Time       `json:"occurredAt"`
	Producer         string          `json:"producer"`
	Payload          json.RawMessage `json:"payload"`
}

// NewEnvelope builds a validated envelope with a fresh event ID.
func NewEnvelope(producer string, eventType string, aggregateType string, aggregateID string, aggregateVersion int64, payload any) (Envelope, error) {
	if producer == "" || eventType == "" || aggregateType == "" || aggregateID == "" {
		return Envelope{}, errors.New("outbox: producer, event type and aggregate identity are required")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("outbox: marshal payload: %w", err)
	}
	return Envelope{
		EventID:          "evt_" + uuid.NewString(),
		EventType:        eventType,
		SchemaVersion:    1,
		AggregateType:    aggregateType,
		AggregateID:      aggregateID,
		AggregateVersion: aggregateVersion,
		OccurredAt:       time.Now().UTC(),
		Producer:         producer,
		Payload:          raw,
	}, nil
}

// Record is one claimed outbox row. Payload is the full JSON envelope.
type Record struct {
	ID        string
	EventType string
	Payload   json.RawMessage
	Attempts  int
	CreatedAt time.Time
}

var tableNamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// Store reads and writes one outbox table.
type Store struct {
	db    Querier
	table string
}

// NewStore binds a Store to an outbox table. db is used by the relay for
// claiming and marking rows; Append always writes through the caller's
// transaction instead. The table name is validated because it is
// interpolated into SQL.
func NewStore(db Querier, table string) (*Store, error) {
	if !tableNamePattern.MatchString(table) {
		return nil, fmt.Errorf("outbox: invalid table name %q", table)
	}
	return &Store{db: db, table: table}, nil
}

// Table reports the bound table name.
func (s *Store) Table() string { return s.table }

// Append writes an event. Call it with the same transaction as the state
// change that produced the event; that is the whole point of an outbox.
func (s *Store) Append(ctx context.Context, q Querier, envelope Envelope) error {
	if q == nil {
		return errors.New("outbox: append requires a transaction")
	}
	full, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("outbox: marshal envelope: %w", err)
	}
	_, err = q.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s (id, event_type, schema_version, aggregate_type, aggregate_id, payload,
			status, available_at, attempts, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now(), 0, now())`, s.table),
		envelope.EventID, envelope.EventType, envelope.SchemaVersion,
		envelope.AggregateType, envelope.AggregateID, full, StatusPending,
	)
	if err != nil {
		return fmt.Errorf("outbox: append: %w", err)
	}
	return nil
}

// ClaimBatch leases up to limit due PENDING rows for owner. When eventTypes
// is non-empty only those types are claimed, so one table can feed several
// independent relays without one marking another's events published.
func (s *Store) ClaimBatch(ctx context.Context, owner string, leaseFor time.Duration, limit int, eventTypes []string) ([]Record, error) {
	typeFilter := ""
	args := []any{owner, leaseFor.Seconds(), StatusPending, limit}
	if len(eventTypes) > 0 {
		typeFilter = "AND event_type = ANY($5)"
		args = append(args, eventTypes)
	}
	rows, err := s.db.Query(ctx, fmt.Sprintf(`
		UPDATE %[1]s SET
			lease_owner = $1,
			lease_expires_at = now() + make_interval(secs => $2)
		WHERE id IN (
			SELECT id FROM %[1]s
			WHERE status = $3
			  AND available_at <= now()
			  AND (lease_expires_at IS NULL OR lease_expires_at < now())
			  %[2]s
			ORDER BY available_at, created_at
			LIMIT $4
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, event_type, payload, attempts, created_at`, s.table, typeFilter),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("outbox: claim batch: %w", err)
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var record Record
		if err := rows.Scan(&record.ID, &record.EventType, &record.Payload, &record.Attempts, &record.CreatedAt); err != nil {
			return nil, fmt.Errorf("outbox: scan claimed row: %w", err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// MarkPublished finalizes a delivered row. Only the lease owner may do so.
func (s *Store) MarkPublished(ctx context.Context, owner string, id string) error {
	tag, err := s.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s SET status = $1, published_at = now(), lease_owner = NULL, lease_expires_at = NULL
		WHERE id = $2 AND lease_owner = $3`, s.table),
		StatusPublished, id, owner,
	)
	if err != nil {
		return fmt.Errorf("outbox: mark published: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("outbox: row %s is not leased by %s", id, owner)
	}
	return nil
}

// MarkFailed records a redacted failure and schedules a retry with capped
// exponential backoff (2s, 4s, ... up to 5 minutes), dead-lettering after
// maxAttempts.
func (s *Store) MarkFailed(ctx context.Context, owner string, id string, publishErr error, maxAttempts int) error {
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	tag, err := s.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s SET
			attempts = attempts + 1,
			last_error_redacted = $1,
			status = CASE WHEN attempts + 1 >= $2 THEN $3 ELSE status END,
			available_at = now() + make_interval(secs => LEAST(300, power(2, LEAST(attempts + 1, 20)))),
			lease_owner = NULL,
			lease_expires_at = NULL
		WHERE id = $4 AND lease_owner = $5`, s.table),
		RedactError(publishErr), maxAttempts, StatusDeadLettered, id, owner,
	)
	if err != nil {
		return fmt.Errorf("outbox: mark failed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("outbox: row %s is not leased by %s", id, owner)
	}
	return nil
}

// RecoverExpiredLeases returns rows abandoned by a crashed relay to the queue.
func (s *Store) RecoverExpiredLeases(ctx context.Context) (int64, error) {
	tag, err := s.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s SET lease_owner = NULL, lease_expires_at = NULL
		WHERE status = $1 AND lease_owner IS NOT NULL AND lease_expires_at < now()`, s.table),
		StatusPending,
	)
	if err != nil {
		return 0, fmt.Errorf("outbox: recover leases: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ReplayDeadLettered returns up to limit dead-lettered rows to the queue with
// a fresh attempt budget. It is an operator action, never automatic.
func (s *Store) ReplayDeadLettered(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	tag, err := s.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %[1]s SET status = $1, attempts = 0, available_at = now(),
			last_error_redacted = '', lease_owner = NULL, lease_expires_at = NULL
		WHERE id IN (SELECT id FROM %[1]s WHERE status = $2 ORDER BY created_at LIMIT $3)`, s.table),
		StatusPending, StatusDeadLettered, limit,
	)
	if err != nil {
		return 0, fmt.Errorf("outbox: replay dead letters: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Counts reports the pending and dead-lettered backlog for metrics and alerts.
func (s *Store) Counts(ctx context.Context) (pending int64, deadLettered int64, err error) {
	err = s.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT count(*) FILTER (WHERE status = $1), count(*) FILTER (WHERE status = $2) FROM %s`, s.table),
		StatusPending, StatusDeadLettered,
	).Scan(&pending, &deadLettered)
	return pending, deadLettered, err
}

// RedactError trims an error to a bounded summary before it is persisted.
func RedactError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 300 {
		message = message[:300]
	}
	return message
}
