// Package platformclients provides typed interfaces and foundational HTTP
// adapters for every authoritative platform service.
//
// Only foundational contract surface is implemented: configuration,
// service-authentication header creation, request/correlation ID
// propagation, timeout application, error normalization, retry
// classification and metrics instrumentation. Business methods arrive with
// later work packages. Unconfigured clients return an explicit
// ErrNotConfigured — they never invent live endpoints and never fall back to
// another service's database.
package platformclients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// Outcome classifies a downstream call result.
type Outcome string

const (
	OutcomeSuccess         Outcome = "success"
	OutcomeValidation      Outcome = "validation"
	OutcomeUnauthenticated Outcome = "unauthenticated"
	OutcomeForbidden       Outcome = "forbidden"
	OutcomeNotFound        Outcome = "not_found"
	OutcomeConflict        Outcome = "conflict"
	OutcomeRateLimited     Outcome = "rate_limited"
	OutcomeTransient       Outcome = "transient"
	OutcomePermanent       Outcome = "permanent"
	OutcomeUnknown         Outcome = "unknown_outcome"
)

// Retryable reports whether a call with this outcome may be retried.
// Only classified transient failures retry. A timed-out state-changing call
// (OutcomeUnknown) must not be automatically repeated unless the downstream
// contract is idempotent and the idempotency key is preserved, or a
// read-back establishes the outcome.
func (o Outcome) Retryable() bool {
	return o == OutcomeTransient
}

// ErrNotConfigured is returned by adapters missing a base URL or credential.
var ErrNotConfigured = errors.New("platformclients: client is not configured")

// Error is the normalized downstream failure. Downstream response bodies are
// truncated and never logged with credentials.
type Error struct {
	Service string
	Outcome Outcome
	Status  int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s call failed: outcome=%s status=%d %s", e.Service, e.Outcome, e.Status, e.Message)
}

// ClassifyStatus maps an HTTP status to an outcome.
func ClassifyStatus(status int) Outcome {
	switch {
	case status >= 200 && status < 300:
		return OutcomeSuccess
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return OutcomeValidation
	case status == http.StatusUnauthorized:
		return OutcomeUnauthenticated
	case status == http.StatusForbidden:
		return OutcomeForbidden
	case status == http.StatusNotFound:
		return OutcomeNotFound
	case status == http.StatusConflict:
		return OutcomeConflict
	case status == http.StatusTooManyRequests:
		return OutcomeRateLimited
	case status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout:
		return OutcomeTransient
	case status >= 500:
		return OutcomePermanent
	default:
		return OutcomePermanent
	}
}

// Options configures one platform client adapter.
type Options struct {
	ServiceName string // downstream service identity
	BaseURL     string
	Secret      []byte
	Timeout     time.Duration
	Metrics     *observability.Metrics
	HTTPClient  *http.Client
}

// Client is the shared foundational HTTP adapter.
type Client struct {
	service    string
	baseURL    string
	secret     []byte
	timeout    time.Duration
	metrics    *observability.Metrics
	httpClient *http.Client
}

// New builds a foundational client adapter.
func New(opts Options) *Client {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	return &Client{
		service:    opts.ServiceName,
		baseURL:    opts.BaseURL,
		secret:     opts.Secret,
		timeout:    opts.Timeout,
		metrics:    opts.Metrics,
		httpClient: httpClient,
	}
}

// Configured reports whether live calls are possible.
func (c *Client) Configured() bool {
	return c != nil && c.baseURL != "" && len(c.secret) > 0
}

// CallMeta carries tracing identifiers onto downstream requests.
type CallMeta struct {
	RequestID      string
	CorrelationID  string
	IdempotencyKey string
}

// Do performs one signed JSON call with timeout, tracing headers, error
// normalization and metrics. out may be nil to discard the response body.
func (c *Client) Do(ctx context.Context, meta CallMeta, method string, path string, body any, out any) error {
	if !c.Configured() {
		return fmt.Errorf("%w: %s", ErrNotConfigured, c.service)
	}

	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return &Error{Service: c.service, Outcome: OutcomePermanent, Message: "request encoding failed"}
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return &Error{Service: c.service, Outcome: OutcomePermanent, Message: "request creation failed"}
	}
	req.Header.Set("Content-Type", "application/json")
	if meta.RequestID != "" {
		req.Header.Set("X-Request-ID", meta.RequestID)
	}
	if meta.CorrelationID != "" {
		req.Header.Set("X-Correlation-ID", meta.CorrelationID)
	}
	if meta.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", meta.IdempotencyKey)
	}
	if err := serviceauth.SignRequest(req, config.ServiceName, c.secret, payload, time.Time{}); err != nil {
		return &Error{Service: c.service, Outcome: OutcomePermanent, Message: "request signing failed"}
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		outcome := OutcomeTransient
		if callCtx.Err() == context.DeadlineExceeded {
			// A timeout on a state-changing call has an unknown outcome.
			outcome = OutcomeUnknown
		}
		c.observe(method, path, outcome, duration)
		return &Error{Service: c.service, Outcome: outcome, Message: "request did not complete"}
	}
	defer resp.Body.Close()

	outcome := ClassifyStatus(resp.StatusCode)
	c.observe(method, path, outcome, duration)

	if outcome != OutcomeSuccess {
		return &Error{
			Service: c.service,
			Outcome: outcome,
			Status:  resp.StatusCode,
			Message: "downstream returned " + http.StatusText(resp.StatusCode),
		}
	}

	if out != nil {
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return &Error{Service: c.service, Outcome: OutcomeUnknown, Status: resp.StatusCode, Message: "response read failed"}
		}
		if err := json.Unmarshal(raw, out); err != nil {
			return &Error{Service: c.service, Outcome: OutcomeUnknown, Status: resp.StatusCode, Message: "response decoding failed"}
		}
	}
	return nil
}

// Ping performs the foundational reachability call used by contract tests
// and (optionally) readiness policies. Remote availability is never a
// liveness requirement.
func (c *Client) Ping(ctx context.Context, meta CallMeta) error {
	return c.Do(ctx, meta, http.MethodGet, "/health", nil, nil)
}

func (c *Client) observe(method string, path string, outcome Outcome, duration time.Duration) {
	if c.metrics == nil {
		return
	}
	labels := map[string]string{"service": c.service, "method": method, "path": path, "outcome": string(outcome)}
	c.metrics.IncCounter("fleet_partner_client_calls_total", labels, 1)
	c.metrics.ObserveDuration("fleet_partner_client_call_duration", map[string]string{"service": c.service}, duration)
}
