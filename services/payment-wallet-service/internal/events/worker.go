package events

import (
	"context"
	"errors"
	"time"
)

type WorkerConfig struct {
	Owner           string
	BatchSize       int
	Interval, Lease time.Duration
	MaxAttempts     int
}

func (c WorkerConfig) normalized() WorkerConfig {
	if c.Owner == "" {
		c.Owner = "wallet-outbox"
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

type Worker struct {
	repo      *OutboxRepository
	publisher Publisher
	config    WorkerConfig
}

func NewWorker(repo *OutboxRepository, publisher Publisher, config WorkerConfig) *Worker {
	return &Worker{repo: repo, publisher: publisher, config: config.normalized()}
}

func (w *Worker) RunPass(ctx context.Context) error {
	if w == nil || w.repo == nil || w.publisher == nil {
		return ErrPublisherNotConfigured
	}
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
