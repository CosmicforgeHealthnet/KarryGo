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
// shared service HMAC contract. Redelivery reuses the same event ID and body.
type HTTPPublisher struct {
	baseURL, path, serviceName string
	secret                     []byte
	client                     *http.Client
}

func NewHTTPPublisher(baseURL, path, serviceName string, secret []byte, timeout time.Duration) (*HTTPPublisher, error) {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(serviceName) == "" || len(secret) == 0 {
		return nil, ErrPublisherNotConfigured
	}
	if strings.TrimSpace(path) == "" {
		path = "/internal/fleet-partner/v1/finance/events"
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HTTPPublisher{baseURL: strings.TrimRight(baseURL, "/"), path: path, serviceName: serviceName, secret: secret, client: &http.Client{Timeout: timeout}}, nil
}

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
		return fmt.Errorf("outbox: delivery outcome unknown: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode == http.StatusConflict || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil
	}
	return fmt.Errorf("outbox: consumer returned http_%d", resp.StatusCode)
}
