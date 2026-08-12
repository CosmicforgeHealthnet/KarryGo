package events

import (
	"context"
	"errors"
	"time"
)

// WorkerConfig tunes the outbox publisher loop.
type WorkerConfig struct {
	Owner           string
	BatchSize       int
	Interval, Lease time.Duration
	MaxAttempts     int
}

func (c WorkerConfig) normalized() WorkerConfig {
	if c.Owner == "" {
		c.Owner = "media-outbox"
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 50
	}
	if c.Interval <= 0 {
		c.Interval = 5 * time.Second
	}
	if c.Lease <= 0 {
		c.Lease = 30 * time.Second
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = DefaultOutboxMaxAttempts
	}
	return c
}

// Worker publishes durable outbox records until its context ends.
type Worker struct {
	repo      *OutboxRepository
	publisher Publisher
	config    WorkerConfig
}

// NewWorker builds the outbox publisher worker.
func NewWorker(repo *OutboxRepository, publisher Publisher, config WorkerConfig) *Worker {
	return &Worker{repo: repo, publisher: publisher, config: config.normalized()}
}

// RunPass performs one claim-publish-settle cycle. Exposed so tests drive the
// worker deterministically instead of waiting on a ticker.
func (w *Worker) RunPass(ctx context.Context) error {
	if w == nil || w.repo == nil || w.publisher == nil {
		return ErrPublisherNotConfigured
	}
	// Lease recovery first: a publisher that died mid-batch left records
	// leased to a process that will never settle them.
	_, _ = w.repo.RecoverExpiredLeases(ctx)
	records, err := w.repo.ClaimBatch(ctx, w.config.Owner, w.config.Lease, w.config.BatchSize)
	if err != nil {
		return err
	}
	var passErr error
	for _, record := range records {
		if err := w.publisher.Publish(ctx, record); err != nil {
			if markErr := w.repo.MarkFailed(ctx, w.config.Owner, record.ID, err, w.config.MaxAttempts); markErr != nil {
				passErr = errors.Join(passErr, markErr)
			}
			continue
		}
		if err := w.repo.MarkPublished(ctx, w.config.Owner, record.ID); err != nil {
			passErr = errors.Join(passErr, err)
		}
	}
	return passErr
}

// Run loops until ctx is cancelled, which is how the composition root stops it
// gracefully.
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()
	for {
		_ = w.RunPass(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
