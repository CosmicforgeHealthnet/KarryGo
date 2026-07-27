package marketplace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// DeliveryOutcome classifies the result of one delivery attempt.
type DeliveryOutcome string

const (
	// DeliveryAccepted — the consumer durably committed the event.
	DeliveryAccepted DeliveryOutcome = "ACCEPTED"
	// DeliveryDuplicate — the consumer already had it. Also a success.
	DeliveryDuplicate DeliveryOutcome = "DUPLICATE"
	// DeliveryRejected — permanently invalid; retrying cannot help.
	DeliveryRejected DeliveryOutcome = "REJECTED"
	// DeliveryRetryable — a transient failure; the consumer did not commit.
	DeliveryRetryable DeliveryOutcome = "RETRYABLE"
	// DeliveryUnknown — we do not know whether the consumer committed.
	DeliveryUnknown DeliveryOutcome = "UNKNOWN"
)

// DeliveryResult is one attempt's classified outcome.
type DeliveryResult struct {
	Outcome DeliveryOutcome
	Code    string
}

// EventDeliverer sends one envelope to a consumer.
type EventDeliverer interface {
	Deliver(ctx context.Context, envelope Envelope) (DeliveryResult, error)
	Configured() bool
}

// UnconfiguredDeliverer fails closed when no target is configured.
//
// A distinct type rather than a nil check so tests can assert by type that a
// fallback has not silently taken the place of a live client.
type UnconfiguredDeliverer struct{}

// Configured reports that no delivery target exists.
func (UnconfiguredDeliverer) Configured() bool { return false }

// Deliver always reports a retryable failure without contacting anything.
func (UnconfiguredDeliverer) Deliver(context.Context, Envelope) (DeliveryResult, error) {
	return DeliveryResult{Outcome: DeliveryRetryable, Code: "delivery_target_not_configured"},
		fmt.Errorf("marketplace: event delivery target is not configured")
}

// DelivererConfig configures the authenticated HTTP delivery client.
type DelivererConfig struct {
	BaseURL     string
	Path        string
	ServiceName string
	Secret      string
	Timeout     time.Duration
}

// HTTPEventDeliverer posts envelopes to a consumer's ingestion endpoint.
type HTTPEventDeliverer struct {
	baseURL     string
	path        string
	serviceName string
	secret      []byte
	client      *http.Client
}

// NewHTTPEventDeliverer builds a delivery client, reporting whether it is live.
func NewHTTPEventDeliverer(config DelivererConfig) (EventDeliverer, bool) {
	if strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.Secret) == "" ||
		strings.TrimSpace(config.ServiceName) == "" {
		return UnconfiguredDeliverer{}, false
	}
	path := strings.TrimSpace(config.Path)
	if path == "" {
		path = "/internal/fleet-partner/v1/events"
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HTTPEventDeliverer{
		baseURL:     strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		path:        path,
		serviceName: strings.TrimSpace(config.ServiceName),
		secret:      []byte(config.Secret),
		client:      &http.Client{Timeout: timeout},
	}, true
}

// Configured reports that a live target exists.
func (d *HTTPEventDeliverer) Configured() bool { return true }

// permanentDeliveryCodes are consumer rejections that retrying cannot fix.
var permanentDeliveryCodes = map[string]bool{
	"unknown_event_type":     true,
	"invalid_envelope":       true,
	"payload_hash_mismatch":  true,
	"schema_version_invalid": true,
	"forbidden_field":        true,
	"caller_not_allowed":     true,
}

// ingestResponse is the consumer's reply contract.
type ingestResponse struct {
	Status string `json:"status"`
	Code   string `json:"code"`
}

// Deliver posts one envelope and classifies the response.
//
// The three-way split matters more than the transport: a 5xx means the
// consumer did not commit and a plain retry is safe; a timeout or a broken
// connection means we never learned, so the retry must be a redelivery of the
// same event id — never a newly minted one.
func (d *HTTPEventDeliverer) Deliver(ctx context.Context, envelope Envelope) (DeliveryResult, error) {
	body, err := json.Marshal(envelope)
	if err != nil {
		return DeliveryResult{Outcome: DeliveryRejected, Code: "envelope_encode_failed"}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+d.path,
		bytes.NewReader(body))
	if err != nil {
		return DeliveryResult{Outcome: DeliveryRetryable, Code: "request_build_failed"}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Event-Id", envelope.EventID)
	req.Header.Set("Idempotency-Key", envelope.EventID)
	if err := serviceauth.SignRequest(req, d.serviceName, d.secret, body, time.Now()); err != nil {
		return DeliveryResult{Outcome: DeliveryRetryable, Code: "request_sign_failed"}, err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		// The request may have been received and committed before the
		// connection broke. Anything stronger than UNKNOWN would be a guess.
		return DeliveryResult{Outcome: DeliveryUnknown, Code: "transport_failure"}, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	var decoded ingestResponse
	_ = json.Unmarshal(raw, &decoded)
	// Only the code is retained. The consumer's message text is free-form and
	// may quote payload content back at us; it does not belong in our rows.
	code := strings.TrimSpace(decoded.Code)

	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated ||
		resp.StatusCode == http.StatusAccepted:
		if strings.EqualFold(decoded.Status, "DUPLICATE") {
			return DeliveryResult{Outcome: DeliveryDuplicate, Code: code}, nil
		}
		return DeliveryResult{Outcome: DeliveryAccepted, Code: code}, nil

	case resp.StatusCode == http.StatusConflict:
		// The consumer already holds this event id. Idempotent success.
		return DeliveryResult{Outcome: DeliveryDuplicate, Code: code}, nil

	case resp.StatusCode == http.StatusRequestTimeout ||
		resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode >= 500:
		return DeliveryResult{Outcome: DeliveryRetryable, Code: statusCode(code, resp.StatusCode)}, nil

	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		// Credentials or allowlisting are wrong. Retrying with the same
		// credentials cannot succeed, and hammering an authenticated endpoint
		// with rejected signatures is exactly what looks like an attack.
		return DeliveryResult{Outcome: DeliveryRejected, Code: statusCode(code, resp.StatusCode)}, nil

	case resp.StatusCode >= 400:
		if code != "" && !permanentDeliveryCodes[code] {
			return DeliveryResult{Outcome: DeliveryRetryable, Code: code}, nil
		}
		return DeliveryResult{Outcome: DeliveryRejected, Code: statusCode(code, resp.StatusCode)}, nil

	default:
		return DeliveryResult{Outcome: DeliveryUnknown, Code: statusCode(code, resp.StatusCode)}, nil
	}
}

// statusCode falls back to the HTTP status when no code was supplied.
func statusCode(code string, status int) string {
	if code != "" {
		return code
	}
	return fmt.Sprintf("http_%d", status)
}
