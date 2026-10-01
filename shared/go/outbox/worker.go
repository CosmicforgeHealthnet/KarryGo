package outbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// Publisher delivers one claimed record.
type Publisher interface {
	Publish(ctx context.Context, record Record) error
}

// WorkerConfig tunes a relay. Zero values get safe defaults.
type WorkerConfig struct {
	// Owner identifies this relay in lease columns. Must be unique per relay.
	Owner       string
	BatchSize   int
	Interval    time.Duration
	Lease       time.Duration
	MaxAttempts int
	// EventTypes restricts the relay to these event types. Empty means all.
	EventTypes []string
	// OnError, when set, receives pass-level failures for logging.
	OnError func(error)
}

func (c WorkerConfig) normalized() WorkerConfig {
	if c.Owner == "" {
		c.Owner = "outbox-relay"
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 50
	}
	if c.Interval <= 0 {
		c.Interval = 2 * time.Second
	}
	if c.Lease <= 0 {
		c.Lease = 30 * time.Second
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = DefaultMaxAttempts
	}
	return c
}

// Worker relays outbox rows to a Publisher.
type Worker struct {
	store     *Store
	publisher Publisher
	config    WorkerConfig
}

// NewWorker wires a relay.
func NewWorker(store *Store, publisher Publisher, config WorkerConfig) *Worker {
	return &Worker{store: store, publisher: publisher, config: config.normalized()}
}

// RunPass claims one batch and tries to deliver each row once.
func (w *Worker) RunPass(ctx context.Context) error {
	if w == nil || w.store == nil || w.publisher == nil {
		return errors.New("outbox: worker is not configured")
	}
	_, _ = w.store.RecoverExpiredLeases(ctx)
	records, err := w.store.ClaimBatch(ctx, w.config.Owner, w.config.Lease, w.config.BatchSize, w.config.EventTypes)
	if err != nil {
		return err
	}
	var passErr error
	for _, record := range records {
		if publishErr := w.publisher.Publish(ctx, record); publishErr != nil {
			if markErr := w.store.MarkFailed(ctx, w.config.Owner, record.ID, publishErr, w.config.MaxAttempts); markErr != nil {
				passErr = errors.Join(passErr, markErr)
			}
			continue
		}
		if markErr := w.store.MarkPublished(ctx, w.config.Owner, record.ID); markErr != nil {
			passErr = errors.Join(passErr, markErr)
		}
	}
	return passErr
}

// Run relays until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.config.Interval)
	defer ticker.Stop()
	for {
		if err := w.RunPass(ctx); err != nil && w.config.OnError != nil && ctx.Err() == nil {
			w.config.OnError(err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// HTTPPublisher POSTs the stored envelope to a consumer, signed with the
// shared service HMAC contract. Redelivery reuses the same event ID and body,
// so the consumer can dedupe. 2xx means delivered; 409 means the consumer
// already holds a conflicting copy, which retrying cannot fix, so it is also
// treated as delivered (the consumer must alert on it).
type HTTPPublisher struct {
	url         string
	serviceName string
	secret      []byte
	client      *http.Client
}

// NewHTTPPublisher builds a publisher for baseURL+path.
func NewHTTPPublisher(baseURL string, path string, serviceName string, secret []byte, timeout time.Duration) (*HTTPPublisher, error) {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(path) == "" || strings.TrimSpace(serviceName) == "" || len(secret) == 0 {
		return nil, errors.New("outbox: publisher needs a base URL, path, service name and secret")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HTTPPublisher{
		url:         strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/"),
		serviceName: serviceName,
		secret:      secret,
		client:      &http.Client{Timeout: timeout},
	}, nil
}

// Publish delivers one record.
func (p *HTTPPublisher) Publish(ctx context.Context, record Record) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(record.Payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-ID", record.ID)
	req.Header.Set("Idempotency-Key", record.ID)
	if err := serviceauth.SignRequest(req, p.serviceName, p.secret, record.Payload, time.Now().UTC()); err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("outbox: delivery outcome unknown: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode == http.StatusConflict || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil
	}
	return fmt.Errorf("outbox: consumer returned http_%d", resp.StatusCode)
}
