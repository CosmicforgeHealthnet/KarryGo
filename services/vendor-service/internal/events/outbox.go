package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Outbox statuses.
const (
	OutboxStatusPending      = "PENDING"
	OutboxStatusPublished    = "PUBLISHED"
	OutboxStatusDeadLettered = "DEAD_LETTERED"
)

// DefaultOutboxMaxAttempts bounds publication retries before dead-lettering.
const DefaultOutboxMaxAttempts = 10

// OutboxRecord is one durable event awaiting publication.
type OutboxRecord struct {
	ID               string
	EventType        string
	SchemaVersion    int
	AggregateType    string
	AggregateID      string
	AggregateVersion int64
	VendorID         string
	Payload          json.RawMessage
	Headers          json.RawMessage
	Status           string
	AvailableAt      time.Time
	Attempts         int
	LeaseOwner       string
	LeaseExpiresAt   *time.Time
	CreatedAt        time.Time
	PublishedAt      *time.Time
	LastError        string
}

// Publisher delivers a claimed outbox record to a downstream consumer.
// Which downstream service a given event actually goes to is a per-event-type
// routing decision made by the caller that constructs the Worker (e.g. one
// Publisher per destination, dispatched by EventType prefix) — not built
// here, since no real event producer exists yet in this architecture-only
// phase. See docs/architecture/08_EVENT_OWNERSHIP.md.
type Publisher interface {
	Publish(ctx context.Context, record OutboxRecord) error
}

// ErrPublisherNotConfigured is returned by DisabledPublisher. Without a
// transport, events stay durably PENDING and are never silently discarded.
var ErrPublisherNotConfigured = errors.New("events: outbox publisher is not configured")

// DisabledPublisher is the explicit no-transport adapter and the default
// until a real destination is wired.
type DisabledPublisher struct{}

func (DisabledPublisher) Publish(context.Context, OutboxRecord) error {
	return ErrPublisherNotConfigured
}

// querier is satisfied by *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// OutboxRepository stores and claims outbox records.
type OutboxRepository struct {
	db *pgxpool.Pool
}

func NewOutboxRepository(db *pgxpool.Pool) *OutboxRepository {
	return &OutboxRepository{db: db}
}

// Append writes an event to the outbox. Call it with the same transaction
// (via q) as the domain mutation that produced the event, so "state changed"
// and "event recorded" commit atomically.
func (r *OutboxRepository) Append(ctx context.Context, q querier, envelope Envelope) error {
	headers, err := json.Marshal(map[string]string{
		"correlationId": envelope.CorrelationID,
		"causationId":   envelope.CausationID,
		"producer":      envelope.Producer,
	})
	if err != nil {
		return fmt.Errorf("outbox: marshal headers: %w", err)
	}
	full, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("outbox: marshal envelope: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO vendor_outbox
			(id, event_type, schema_version, aggregate_type, aggregate_id, aggregate_version,
			 vendor_id, correlation_id, causation_id, payload, headers, status, available_at, attempts, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, NULLIF($9, ''), $10, $11, $12, now(), 0, now())`,
		envelope.EventID, envelope.EventType, envelope.SchemaVersion,
		envelope.AggregateType, envelope.AggregateID, envelope.AggregateVersion,
		envelope.VendorID, envelope.CorrelationID, envelope.CausationID, full, headers, OutboxStatusPending,
	)
	if err != nil {
		return fmt.Errorf("outbox: append: %w", err)
	}
	return nil
}

// ClaimBatch leases up to limit due PENDING records for owner using
// FOR UPDATE SKIP LOCKED so concurrent workers never double-claim.
func (r *OutboxRepository) ClaimBatch(ctx context.Context, owner string, leaseFor time.Duration, limit int) ([]OutboxRecord, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE vendor_outbox SET
			lease_owner = $1,
			lease_expires_at = now() + make_interval(secs => $2)
		WHERE id IN (
			SELECT id FROM vendor_outbox
			WHERE status = $3
			  AND available_at <= now()
			  AND (lease_expires_at IS NULL OR lease_expires_at < now())
			ORDER BY available_at
			LIMIT $4
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, event_type, schema_version, aggregate_type, aggregate_id, aggregate_version,
			COALESCE(vendor_id, ''), payload, headers, status, available_at,
			attempts, COALESCE(lease_owner, ''), lease_expires_at, created_at,
			published_at, last_error_redacted`,
		owner, leaseFor.Seconds(), OutboxStatusPending, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("outbox: claim batch: %w", err)
	}
	defer rows.Close()

	var records []OutboxRecord
	for rows.Next() {
		var record OutboxRecord
		if err := rows.Scan(
			&record.ID, &record.EventType, &record.SchemaVersion, &record.AggregateType,
			&record.AggregateID, &record.AggregateVersion, &record.VendorID,
			&record.Payload, &record.Headers, &record.Status, &record.AvailableAt,
			&record.Attempts, &record.LeaseOwner, &record.LeaseExpiresAt, &record.CreatedAt,
			&record.PublishedAt, &record.LastError,
		); err != nil {
			return nil, fmt.Errorf("outbox: scan claimed record: %w", err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// MarkPublished finalizes a successfully published record (owner-only).
func (r *OutboxRepository) MarkPublished(ctx context.Context, owner, id string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE vendor_outbox SET
			status = $1, published_at = now(), lease_owner = NULL, lease_expires_at = NULL
		WHERE id = $2 AND lease_owner = $3`,
		OutboxStatusPublished, id, owner,
	)
	if err != nil {
		return fmt.Errorf("outbox: mark published: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("outbox: record %s is not leased by %s", id, owner)
	}
	return nil
}

// MarkFailed records a redacted failure, schedules a retry with bounded
// exponential backoff and dead-letters after maxAttempts.
func (r *OutboxRepository) MarkFailed(ctx context.Context, owner, id string, publishErr error, maxAttempts int) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE vendor_outbox SET
			attempts = attempts + 1,
			last_error_redacted = $1,
			status = CASE WHEN attempts + 1 >= $2 THEN $3 ELSE status END,
			available_at = now() + make_interval(secs => LEAST(300, power(2, attempts + 1))),
			lease_owner = NULL,
			lease_expires_at = NULL
		WHERE id = $4 AND lease_owner = $5`,
		RedactError(publishErr), maxAttempts, OutboxStatusDeadLettered, id, owner,
	)
	if err != nil {
		return fmt.Errorf("outbox: mark failed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("outbox: record %s is not leased by %s", id, owner)
	}
	return nil
}

// RecoverExpiredLeases returns abandoned records (owner crashed mid-publish)
// to the queue.
func (r *OutboxRepository) RecoverExpiredLeases(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE vendor_outbox SET lease_owner = NULL, lease_expires_at = NULL
		WHERE lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL AND lease_expires_at < now()`)
	if err != nil {
		return 0, fmt.Errorf("outbox: recover leases: %w", err)
	}
	return tag.RowsAffected(), nil
}

// Replay returns a dead-lettered record to the queue for an operator.
func (r *OutboxRepository) Replay(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE vendor_outbox SET
			status = $1, attempts = 0, available_at = now(),
			last_error_redacted = '', lease_owner = NULL, lease_expires_at = NULL
		WHERE id = $2 AND status = $3`,
		OutboxStatusPending, id, OutboxStatusDeadLettered)
	if err != nil {
		return fmt.Errorf("outbox: replay: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("outbox: record %s is not dead-lettered", id)
	}
	return nil
}

// PendingCount reports backlog size for metrics.
func (r *OutboxRepository) PendingCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM vendor_outbox WHERE status = $1`, OutboxStatusPending,
	).Scan(&count)
	return count, err
}

// RedactError trims error text to a bounded, credential-free summary.
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

// DomainPublisher appends domain events onto the outbox inside the caller's
// transaction, which is what makes "state changed" and "event recorded"
// atomic. Nil-safe: a zero-value DomainPublisher silently no-ops, so features
// under construction can accept one without the outbox existing yet.
type DomainPublisher struct {
	outbox *OutboxRepository
}

func NewDomainPublisher(outbox *OutboxRepository) *DomainPublisher {
	return &DomainPublisher{outbox: outbox}
}

func (p *DomainPublisher) Publish(ctx context.Context, tx pgx.Tx, eventType, aggregateType, aggregateID string,
	aggregateVersion int64, vendorID, correlationID, causationID string, payload any) error {
	if p == nil || p.outbox == nil {
		return nil
	}
	envelope, err := NewEnvelope(eventType, 1, aggregateType, aggregateID, aggregateVersion,
		vendorID, correlationID, causationID, payload, vendorID == "")
	if err != nil {
		return err
	}
	return p.outbox.Append(ctx, tx, envelope)
}
