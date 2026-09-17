package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MovementPublisher delivers queued movement envelopes to the customer
// context owner. Delivery is at-least-once with bounded retries; the consumer
// deduplicates on event_id, which is what makes that safe.
type MovementPublisher struct {
	Pool      *pgxpool.Pool
	Deliverer *HTTPDeliverer
	Owner     string
	BatchSize int
	// MaxAttempts bounds retries before a row is parked as DEAD for an
	// operator. A dead row is never silently dropped.
	MaxAttempts int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	Now         func() time.Time
}

func (p *MovementPublisher) now() time.Time {
	if p.Now == nil {
		return time.Now().UTC()
	}
	return p.Now().UTC()
}

func (p *MovementPublisher) backoff(attempts int) time.Duration {
	base, max := p.BaseBackoff, p.MaxBackoff
	if base <= 0 {
		base = 5 * time.Second
	}
	if max <= 0 {
		max = 10 * time.Minute
	}
	delay := base
	for i := 0; i < attempts && delay < max; i++ {
		delay *= 2
	}
	if delay > max {
		delay = max
	}
	return delay
}

type pendingMovement struct {
	id       int64
	payload  []byte
	attempts int
}

// PublishBatch claims and delivers one batch. It returns how many rows were
// delivered.
func (p *MovementPublisher) PublishBatch(ctx context.Context) (int, error) {
	if p == nil || p.Pool == nil || p.Deliverer == nil {
		return 0, ErrEmitterNotConfigured
	}
	size := p.BatchSize
	if size <= 0 {
		size = 25
	}
	owner := p.Owner
	if owner == "" {
		owner = "customer-movement-publisher"
	}
	now := p.now()
	rows, err := p.Pool.Query(ctx, `UPDATE customer_movement_outbox SET leased_until=$1, leased_by=$2
		WHERE id IN (
			SELECT id FROM customer_movement_outbox
			WHERE status='PENDING' AND next_attempt_at <= $3 AND (leased_until IS NULL OR leased_until < $3)
			ORDER BY id FOR UPDATE SKIP LOCKED LIMIT $4)
		RETURNING id, payload, attempts`, now.Add(2*time.Minute), owner, now, size)
	if err != nil {
		return 0, err
	}
	var claimed []pendingMovement
	for rows.Next() {
		var row pendingMovement
		if err = rows.Scan(&row.id, &row.payload, &row.attempts); err != nil {
			rows.Close()
			return 0, err
		}
		claimed = append(claimed, row)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}

	delivered := 0
	for _, row := range claimed {
		var envelope Envelope
		deliverErr := json.Unmarshal(row.payload, &envelope)
		if deliverErr == nil {
			deliverErr = p.Deliverer.Deliver(ctx, envelope)
		}
		if deliverErr == nil {
			if _, err = p.Pool.Exec(ctx, `UPDATE customer_movement_outbox SET status='DELIVERED', delivered_at=$2,
				leased_until=NULL, leased_by=NULL, last_error=NULL WHERE id=$1`, row.id, p.now()); err != nil {
				return delivered, err
			}
			delivered++
			continue
		}
		attempts := row.attempts + 1
		status := "PENDING"
		maxAttempts := p.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = 12
		}
		if attempts >= maxAttempts {
			status = "DEAD"
		}
		if _, err = p.Pool.Exec(ctx, `UPDATE customer_movement_outbox SET attempts=$2, status=$3,
			next_attempt_at=$4, leased_until=NULL, leased_by=NULL, last_error=$5 WHERE id=$1`,
			row.id, attempts, status, p.now().Add(p.backoff(attempts)), deliverErr.Error()); err != nil {
			return delivered, err
		}
	}
	return delivered, nil
}

// Run publishes until the context ends.
func (p *MovementPublisher) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				delivered, err := p.PublishBatch(ctx)
				if err != nil || delivered == 0 {
					break
				}
			}
		}
	}
}
