package events

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// HTTPPublisher delivers the already-versioned envelope to Fleet using the
// shared service HMAC contract. Redelivery reuses the same event ID and body,
// which is what lets the consumer's inbox recognise a duplicate rather than
// applying the same state change twice.
type HTTPPublisher struct {
	baseURL, path, serviceName string
	secret                     []byte
	client                     *http.Client
}

// DefaultFleetEventPath is Fleet's media event ingestion route.
const DefaultFleetEventPath = "/internal/fleet-partner/v1/documents/media-events"

// NewHTTPPublisher builds the signed HTTP transport. An unconfigured target
// returns ErrPublisherNotConfigured rather than a publisher that silently
// drops events.
func NewHTTPPublisher(baseURL, path, serviceName string, secret []byte, timeout time.Duration) (*HTTPPublisher, error) {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(serviceName) == "" || len(secret) == 0 {
		return nil, ErrPublisherNotConfigured
	}
	if strings.TrimSpace(path) == "" {
		path = DefaultFleetEventPath
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HTTPPublisher{
		baseURL:     strings.TrimRight(baseURL, "/"),
		path:        path,
		serviceName: serviceName,
		secret:      secret,
		client:      &http.Client{Timeout: timeout},
	}, nil
}

// Publish signs and delivers one record.
//
// A 409 is treated as delivered: the consumer refused a payload conflict on an
// event ID it already holds, and retrying the identical body forever would
// only keep the record leased.
func (p *HTTPPublisher) Publish(ctx context.Context, record OutboxRecord) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+p.path, bytes.NewReader(record.Payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-ID", record.ID)
	req.Header.Set("Idempotency-Key", record.ID)
	if err = serviceauth.SignRequest(req, p.serviceName, p.secret, record.Payload, time.Now().UTC()); err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		// The outcome is unknown, not failed: the consumer may have applied it.
		// Redelivery is safe because the event ID and body are unchanged.
		return fmt.Errorf("outbox: delivery outcome unknown: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode == http.StatusConflict || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil
	}
	return fmt.Errorf("outbox: consumer returned http_%d", resp.StatusCode)
}
