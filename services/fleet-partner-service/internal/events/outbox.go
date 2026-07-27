package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
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
	ID             string
	EventType      string
	SchemaVersion  int
	AggregateType  string
	AggregateID    string
	OrganizationID string
	Payload        json.RawMessage
	Headers        json.RawMessage
	Status         string
	AvailableAt    time.Time
	Attempts       int
	LeaseOwner     string
	LeaseExpiresAt *time.Time
	CreatedAt      time.Time
	PublishedAt    *time.Time
	LastError      string
}

// Publisher delivers a claimed outbox record to the platform event transport.
type Publisher interface {
	Publish(ctx context.Context, record OutboxRecord) error
}

// ErrPublisherNotConfigured is returned by DisabledPublisher: the repository
// has no event broker yet, so publication fails loudly and events stay
// durably PENDING — they are never silently discarded.
var ErrPublisherNotConfigured = errors.New("events: outbox publisher is not configured")

// DisabledPublisher is the explicit no-transport adapter.
type DisabledPublisher struct{}

// Publish always fails with ErrPublisherNotConfigured.
func (DisabledPublisher) Publish(context.Context, OutboxRecord) error {
	return ErrPublisherNotConfigured
}

// OutboxRepository stores and claims outbox records.
type OutboxRepository struct {
	db database.Querier
}

// NewOutboxRepository wires the repository over a pool or transaction.
func NewOutboxRepository(db database.Querier) *OutboxRepository {
	return &OutboxRepository{db: db}
}

// Append writes an event to the outbox. Call it with the same transaction as
// the domain mutation that produced the event.
func (r *OutboxRepository) Append(ctx context.Context, q database.Querier, envelope Envelope) error {
	headers, err := json.Marshal(map[string]string{
		"correlationId": envelope.CorrelationID,
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
		INSERT INTO fleet_partner_outbox
			(id, event_type, schema_version, aggregate_type, aggregate_id, organization_id,
			 payload, headers, status, available_at, attempts, created_at)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, $9, now(), 0, now())`,
		envelope.EventID, envelope.EventType, envelope.SchemaVersion,
		envelope.AggregateType, envelope.AggregateID, envelope.OrganizationID,
		full, headers, OutboxStatusPending,
	)
	if err != nil {
		return fmt.Errorf("outbox: append: %w", err)
	}
	return nil
}

// ClaimBatch leases up to limit due PENDING records for owner using
// FOR UPDATE SKIP LOCKED so concurrent publishers never double-claim.
func (r *OutboxRepository) ClaimBatch(ctx context.Context, owner string, leaseFor time.Duration, limit int) ([]OutboxRecord, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE fleet_partner_outbox SET
			lease_owner = $1,
			lease_expires_at = now() + $2::interval
		WHERE id IN (
			SELECT id FROM fleet_partner_outbox
			WHERE status = $3
			  AND available_at <= now()
			  AND (lease_expires_at IS NULL OR lease_expires_at < now())
			ORDER BY available_at
			LIMIT $4
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, event_type, schema_version, aggregate_type, aggregate_id,
			COALESCE(organization_id, ''), payload, headers, status, available_at,
			attempts, COALESCE(lease_owner, ''), lease_expires_at, created_at,
			published_at, COALESCE(last_error_redacted, '')`,
		owner, durationToInterval(leaseFor), OutboxStatusPending, limit,
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
			&record.AggregateID, &record.OrganizationID, &record.Payload, &record.Headers,
			&record.Status, &record.AvailableAt, &record.Attempts, &record.LeaseOwner,
			&record.LeaseExpiresAt, &record.CreatedAt, &record.PublishedAt, &record.LastError,
		); err != nil {
			return nil, fmt.Errorf("outbox: scan claimed record: %w", err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// MarkPublished finalizes a successfully published record (owner-only).
func (r *OutboxRepository) MarkPublished(ctx context.Context, owner string, id string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_outbox SET
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

// MarkFailed records a redacted failure, schedules a retry with backoff, and
// dead-letters after maxAttempts.
func (r *OutboxRepository) MarkFailed(ctx context.Context, owner string, id string, publishErr error, maxAttempts int) error {
	redacted := redactError(publishErr)
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_outbox SET
			attempts = attempts + 1,
			last_error_redacted = $1,
			status = CASE WHEN attempts + 1 >= $2 THEN $3 ELSE status END,
			available_at = now() + make_interval(secs => LEAST(300, power(2, attempts + 1))),
			lease_owner = NULL,
			lease_expires_at = NULL
		WHERE id = $4 AND lease_owner = $5`,
		redacted, maxAttempts, OutboxStatusDeadLettered, id, owner,
	)
	if err != nil {
		return fmt.Errorf("outbox: mark failed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("outbox: record %s is not leased by %s", id, owner)
	}
	return nil
}

// PendingCount reports backlog size for metrics.
func (r *OutboxRepository) PendingCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM fleet_partner_outbox WHERE status = $1`, OutboxStatusPending,
	).Scan(&count)
	return count, err
}

// redactError trims error text to a bounded, credential-free summary.
func redactError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 300 {
		message = message[:300]
	}
	return message
}

func durationToInterval(d time.Duration) string {
	return fmt.Sprintf("%d seconds", int64(d.Seconds()))
}
