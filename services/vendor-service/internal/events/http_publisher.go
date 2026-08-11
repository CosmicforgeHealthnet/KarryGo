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

// HTTPPublisher delivers an already-versioned envelope to one downstream
// service using the platform's shared HMAC service-auth contract (see
// shared/go/serviceauth and docs/architecture/09_SECURITY_AND_AUTHORITY_MODEL.md).
// Redelivery reuses the same event ID and body, so the receiving service's
// own inbox can dedupe on Idempotency-Key.
//
// A single Worker is wired to a single HTTPPublisher instance (one
// destination). Routing different event types to different destinations
// means constructing one Worker per destination and partitioning ClaimBatch
// by event-type prefix — not built here, since no real event producer exists
// yet in this architecture-only phase.
type HTTPPublisher struct {
	baseURL, path, serviceName string
	secret                     []byte
	client                     *http.Client
}

func NewHTTPPublisher(baseURL, path, serviceName string, secret []byte, timeout time.Duration) (*HTTPPublisher, error) {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(path) == "" || strings.TrimSpace(serviceName) == "" || len(secret) == 0 {
		return nil, ErrPublisherNotConfigured
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
