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
//
// Section 29 reliability layer (all applied uniformly inside Exchange, so no
// call site across this package's ~15 concrete clients needs to change):
//
//   - W3C trace context (internal/platform/tracing) is read from ctx, a
//     child span is opened for the call, a "traceparent" header is set on
//     the outbound request, and the span is closed with a structured log
//     line + duration metric when the call (including any retries) finishes.
//   - A bounded-cardinality path template (normalizePathTemplate) replaces
//     ID-shaped path segments with "{id}" before it is ever used as a metric
//     label or span name — the raw path itself may legitimately contain an
//     organization, provider or media object ID (see e.g.
//     media_file.go's Path: c.apiBase+"/objects/"+url.PathEscape(id)), and
//     that must never become an unbounded metrics/span label.
//   - A classified-transient failure is automatically retried, bounded,
//     with exponential backoff and jitter, honoring a Retry-After response
//     header — but ONLY for a safe HTTP method (GET/HEAD/OPTIONS) or when
//     the caller supplied CallMeta.IdempotencyKey, which is exactly the
//     "writes require idempotency before automatic retry" rule expressed as
//     a gate every call site automatically satisfies or fails safely.
//   - A per-downstream-service circuit breaker (internal/platform/
//     circuitbreaker) wraps the whole call (including its retries): OPEN
//     refuses new calls immediately without another network attempt.
//     Validation/auth/not-found/conflict outcomes never count as breaker
//     failures, matching Section 29's explicit "do not count caller
//     validation errors, authorization denials or normal business conflicts
//     as dependency failures."
package platformclients

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/circuitbreaker"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tracing"
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

// isBreakerFailure reports whether an outcome counts against the circuit
// breaker. Caller mistakes (validation, auth, not-found, conflict, rate
// limit) are never held against the dependency's own health.
func (o Outcome) isBreakerFailure() bool {
	return o == OutcomeTransient || o == OutcomePermanent || o == OutcomeUnknown
}

// ErrNotConfigured is returned by adapters missing a base URL or credential.
var ErrNotConfigured = errors.New("platformclients: client is not configured")

// ErrCircuitOpen is returned when the per-service circuit breaker is
// refusing calls; the caller must treat this exactly like OutcomeTransient
// (or OutcomeUnknown for a state-changing call already in flight elsewhere)
// — never as a confirmed failure.
var ErrCircuitOpen = circuitbreaker.ErrOpen

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
	// Environment is THIS service's own deployment environment (cfg.AppEnv),
	// used only for the "environment" field of the structured span/call log
	// — it has nothing to do with the downstream service being called.
	Environment string

	// MaxRetries/RetryBackoffBase/RetryBackoffMax bound the automatic retry
	// applied to classified-transient outcomes. Zero takes the same safe
	// defaults config.Config.Validate requires of the equivalent env vars
	// (FLEET_PARTNER_EXTERNAL_CLIENT_*), so a client built in a test without
	// explicit retry options still behaves like production.
	MaxRetries       int
	RetryBackoffBase time.Duration
	RetryBackoffMax  time.Duration

	// Breaker tunes this client's own circuit breaker. Zero value takes safe
	// defaults (see circuitbreaker.Config.normalized). One Client always has
	// exactly one breaker instance scoped to its own ServiceName — never a
	// breaker shared across unrelated downstream services.
	Breaker circuitbreaker.Config
	// OnBreakerTransition observes this client's own breaker state changes
	// (for tests and for a caller that wants a readiness/metrics hook beyond
	// the structured log line Exchange already emits on every transition).
	OnBreakerTransition func(circuitbreaker.Transition)
}

// Client is the shared foundational HTTP adapter.
type Client struct {
	service     string
	baseURL     string
	secret      []byte
	timeout     time.Duration
	metrics     *observability.Metrics
	httpClient  *http.Client
	environment string

	maxRetries       int
	retryBackoffBase time.Duration
	retryBackoffMax  time.Duration

	breaker *circuitbreaker.Breaker
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
	maxRetries := opts.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}
	backoffBase := opts.RetryBackoffBase
	if backoffBase <= 0 {
		backoffBase = 200 * time.Millisecond
	}
	backoffMax := opts.RetryBackoffMax
	if backoffMax <= 0 {
		backoffMax = 5 * time.Second
	}
	client := &Client{
		service:          opts.ServiceName,
		baseURL:          opts.BaseURL,
		secret:           opts.Secret,
		timeout:          opts.Timeout,
		metrics:          opts.Metrics,
		httpClient:       httpClient,
		environment:      opts.Environment,
		maxRetries:       maxRetries,
		retryBackoffBase: backoffBase,
		retryBackoffMax:  backoffMax,
	}
	client.breaker = circuitbreaker.New(opts.ServiceName, opts.Breaker, func(tr circuitbreaker.Transition) {
		observability.Log(config.ServiceName, client.environment, "warn", "circuit_breaker_transition", map[string]any{
			"dependency": tr.Name, "fromState": string(tr.From), "toState": string(tr.To),
		})
		if client.metrics != nil {
			client.metrics.SetGauge("fleet_partner_circuit_breaker_state", map[string]string{"dependency": tr.Name}, breakerStateValue(tr.To))
		}
		if opts.OnBreakerTransition != nil {
			opts.OnBreakerTransition(tr)
		}
	})
	return client
}

func breakerStateValue(s circuitbreaker.State) float64 {
	switch s {
	case circuitbreaker.Closed:
		return 0
	case circuitbreaker.HalfOpen:
		return 1
	case circuitbreaker.Open:
		return 2
	default:
		return -1
	}
}

// Configured reports whether live calls are possible.
func (c *Client) Configured() bool {
	return c != nil && c.baseURL != "" && len(c.secret) > 0
}

// BreakerState exposes this client's own circuit state (for readiness/
// diagnostics reads only — never used to gate a business decision).
func (c *Client) BreakerState() circuitbreaker.State {
	if c == nil || c.breaker == nil {
		return circuitbreaker.Closed
	}
	return c.breaker.State()
}

// CallMeta carries tracing identifiers onto downstream requests.
type CallMeta struct {
	OrganizationID string
	RequestID      string
	CorrelationID  string
	CausationID    string
	IdempotencyKey string
}

// Exchange is the general signed call.
//
// Do is the JSON-in/JSON-out special case. This form exists because a private
// media contract has to move file bytes: a request body that is not JSON and a
// response body that is not JSON both have to be signed and verified the same
// way as everything else, and re-implementing the signing to achieve that is
// how two subtly different authentication paths get created.
type Exchange struct {
	Method string
	Path   string
	// DisableRetries forces a single HTTP attempt even when the method is safe
	// or carries an idempotency key. Use it for commands whose worker contract
	// resolves an ambiguous outcome by authoritative read-back instead of a
	// second write.
	DisableRetries bool
	// JSONBody is marshalled when set. RawBody is sent verbatim; at most one
	// of the two may be present.
	JSONBody any
	RawBody  []byte
	// ContentType overrides the default application/json.
	ContentType string
	// Headers are additional request headers. Never put a secret here that the
	// signature does not already cover.
	Headers map[string]string
	// Out decodes a JSON response body when set.
	Out any
	// RawOut receives the response body verbatim when set, for content reads.
	RawOut *[]byte
	// MaxResponseBytes bounds the response read. Zero uses 1 MiB, which is the
	// right ceiling for a JSON contract and far too small for file content.
	MaxResponseBytes int64
	// Operation optionally overrides the bounded metric/span label derived
	// from Path (see normalizePathTemplate). Most call sites never need this
	// — the automatic ID-segment normalization already produces a bounded
	// template — but it is available for a path shape the heuristic cannot
	// safely template.
	Operation string
}

// Do performs one signed JSON call with timeout, tracing headers, error
// normalization and metrics. out may be nil to discard the response body.
func (c *Client) Do(ctx context.Context, meta CallMeta, method string, path string, body any, out any) error {
	return c.Exchange(ctx, meta, Exchange{Method: method, Path: path, JSONBody: body, Out: out})
}

// Exchange performs one signed call: circuit breaker -> bounded classified
// retry -> a single HTTP attempt per try, all under one trace span.
func (c *Client) Exchange(ctx context.Context, meta CallMeta, exchange Exchange) error {
	if !c.Configured() {
		return fmt.Errorf("%w: %s", ErrNotConfigured, c.service)
	}

	operation := exchange.Operation
	if operation == "" {
		operation = normalizePathTemplate(exchange.Path)
	}
	spanCtx, endSpan := tracing.StartSpan(ctx, c.metrics, config.ServiceName, c.environment,
		"client."+c.service, map[string]any{"method": exchange.Method, "operation": operation})
	ctx = spanCtx

	if err := c.breaker.Allow(); err != nil {
		c.observe(exchange.Method, operation, OutcomeUnknown, 0)
		endSpan(err)
		return &Error{Service: c.service, Outcome: OutcomeUnknown, Message: "circuit breaker open"}
	}

	retryable := !exchange.DisableRetries && (exchange.Method == "" || exchange.Method == http.MethodGet ||
		exchange.Method == http.MethodHead || exchange.Method == http.MethodOptions ||
		meta.IdempotencyKey != "")

	var lastErr error
	var lastOutcome Outcome
	retryAfterSeconds := -1 // per-call only: never shared across concurrent Exchange calls on one Client
	attempts := 1
	if retryable {
		attempts = c.maxRetries + 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			wait := c.backoffFor(attempt, retryAfterSeconds)
			retryAfterSeconds = -1
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				lastErr = ctx.Err()
				break
			case <-timer.C:
			}
			if ctx.Err() != nil {
				break
			}
			if c.metrics != nil {
				c.metrics.IncCounter("fleet_partner_client_retry_attempts_total",
					map[string]string{"service": c.service, "operation": operation}, 1)
			}
			observability.Log(config.ServiceName, c.environment, "info", "client_retry_attempt", map[string]any{
				"service": c.service, "operation": operation, "retryAttempt": attempt,
			})
		}

		var outcome Outcome
		outcome, lastErr, retryAfterSeconds = c.attempt(ctx, meta, exchange, operation)
		lastOutcome = outcome
		if outcome == OutcomeSuccess {
			c.breaker.RecordSuccess()
			endSpan(nil)
			return nil
		}
		if !outcome.Retryable() || attempt == attempts-1 {
			break
		}
	}

	if lastOutcome.isBreakerFailure() {
		c.breaker.RecordFailure()
	} else {
		c.breaker.RecordSuccess()
	}
	endSpan(lastErr)
	return lastErr
}

// attempt performs exactly one HTTP round trip. The returned int is a
// server-supplied Retry-After in seconds, or -1 when none was sent.
func (c *Client) attempt(ctx context.Context, meta CallMeta, exchange Exchange, operation string) (Outcome, error, int) {
	method := exchange.Method
	path := exchange.Path
	out := exchange.Out

	payload := exchange.RawBody
	if exchange.JSONBody != nil {
		if payload != nil {
			return OutcomePermanent, &Error{Service: c.service, Outcome: OutcomePermanent, Message: "request carries two bodies"}, -1
		}
		var err error
		payload, err = json.Marshal(exchange.JSONBody)
		if err != nil {
			return OutcomePermanent, &Error{Service: c.service, Outcome: OutcomePermanent, Message: "request encoding failed"}, -1
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return OutcomePermanent, &Error{Service: c.service, Outcome: OutcomePermanent, Message: "request creation failed"}, -1
	}
	contentType := exchange.ContentType
	if contentType == "" {
		contentType = "application/json"
	}
	req.Header.Set("Content-Type", contentType)
	if meta.OrganizationID != "" {
		req.Header.Set("X-Organization-ID", meta.OrganizationID)
	}
	for key, value := range exchange.Headers {
		if value != "" {
			req.Header.Set(key, value)
		}
	}
	if meta.RequestID != "" {
		req.Header.Set("X-Request-ID", meta.RequestID)
	}
	if meta.CorrelationID != "" {
		req.Header.Set("X-Correlation-ID", meta.CorrelationID)
	}
	if meta.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", meta.IdempotencyKey)
	}
	if meta.CausationID != "" {
		req.Header.Set("X-Causation-ID", meta.CausationID)
	}
	if tc, ok := tracing.FromContext(ctx); ok {
		req.Header.Set("traceparent", tc.ChildSpan().TraceParent())
	}
	// Private consumers persist this nonce to reject a replay even while the
	// shared signature timestamp remains fresh.
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return OutcomePermanent, &Error{Service: c.service, Outcome: OutcomePermanent, Message: "nonce generation failed"}, -1
	}
	req.Header.Set("X-Service-Nonce", hex.EncodeToString(nonce[:]))
	if err := serviceauth.SignRequest(req, config.ServiceName, c.secret, payload, time.Time{}); err != nil {
		return OutcomePermanent, &Error{Service: c.service, Outcome: OutcomePermanent, Message: "request signing failed"}, -1
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
		c.observe(method, operation, outcome, duration)
		return outcome, &Error{Service: c.service, Outcome: outcome, Message: "request did not complete"}, -1
	}
	defer resp.Body.Close()

	outcome := ClassifyStatus(resp.StatusCode)
	c.observe(method, operation, outcome, duration)

	if outcome != OutcomeSuccess {
		retryAfter := -1
		if outcome == OutcomeRateLimited || outcome == OutcomeTransient {
			retryAfter = parseRetryAfterSeconds(resp.Header.Get("Retry-After"))
		}
		return outcome, &Error{
			Service: c.service,
			Outcome: outcome,
			Status:  resp.StatusCode,
			Message: "downstream returned " + http.StatusText(resp.StatusCode),
		}, retryAfter
	}

	if out == nil && exchange.RawOut == nil {
		return OutcomeSuccess, nil, -1
	}

	limit := exchange.MaxResponseBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return OutcomeUnknown, &Error{Service: c.service, Outcome: OutcomeUnknown, Status: resp.StatusCode, Message: "response read failed"}, -1
	}
	if exchange.RawOut != nil {
		*exchange.RawOut = raw
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return OutcomeUnknown, &Error{Service: c.service, Outcome: OutcomeUnknown, Status: resp.StatusCode, Message: "response decoding failed"}, -1
		}
	}
	return OutcomeSuccess, nil, -1
}

// parseRetryAfterSeconds parses a "Retry-After" header's delta-seconds form.
// The HTTP-date form is intentionally not supported: this client already
// caps whatever it returns at retryBackoffMax, and parsing an HTTP-date
// correctly (versus a delta) is not worth the added surface for a value
// that is capped either way. -1 means "no usable value".
func parseRetryAfterSeconds(header string) int {
	if header == "" {
		return -1
	}
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		return seconds
	}
	return -1
}

// backoffFor computes the wait before the given attempt number (1-based: the
// delay before the SECOND try). A server-supplied Retry-After
// (retryAfterSeconds >= 0) takes precedence over the computed exponential
// backoff, capped at retryBackoffMax either way so a large Retry-After
// cannot stall a request indefinitely.
func (c *Client) backoffFor(attempt int, retryAfterSeconds int) time.Duration {
	if retryAfterSeconds >= 0 {
		d := time.Duration(retryAfterSeconds) * time.Second
		if d > c.retryBackoffMax {
			d = c.retryBackoffMax
		}
		return d
	}
	backoff := c.retryBackoffBase * time.Duration(1<<uint(attempt-1))
	if backoff > c.retryBackoffMax || backoff <= 0 {
		backoff = c.retryBackoffMax
	}
	jitter := time.Duration(mrand.Int64N(int64(backoff) + 1))
	return backoff/2 + jitter/2
}

// Ping performs the foundational reachability call used by contract tests
// and (optionally) readiness policies. Remote availability is never a
// liveness requirement.
func (c *Client) Ping(ctx context.Context, meta CallMeta) error {
	return c.Do(ctx, meta, http.MethodGet, "/health", nil, nil)
}

func (c *Client) observe(method string, operation string, outcome Outcome, duration time.Duration) {
	if c.metrics == nil {
		return
	}
	labels := map[string]string{"service": c.service, "method": method, "path": operation, "outcome": string(outcome)}
	c.metrics.IncCounter("fleet_partner_client_calls_total", labels, 1)
	c.metrics.ObserveDuration("fleet_partner_client_call_duration", map[string]string{"service": c.service}, duration)
}

// idSegmentPattern matches a UUID (with or without dashes) or this
// repository's own opaque prefixed-ID shape (see internal/platform/ids).
var idSegmentPattern = regexp.MustCompile(`(?i)^[a-z]{2,10}_[0-9a-f]{20,}$|^[0-9a-f]{8}-?[0-9a-f]{4}-?[0-9a-f]{4}-?[0-9a-f]{4}-?[0-9a-f]{12}$|^[0-9a-f]{24,40}$|^[0-9]+$`)

// normalizePathTemplate replaces ID-shaped path segments with "{id}" so a
// path built by string concatenation with a live resource ID (this
// package's own, or an opaque external one) never becomes an unbounded
// metrics or span label — mirroring what gin's c.FullPath() route template
// already gives inbound HTTP handlers for free.
func normalizePathTemplate(path string) string {
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		path = path[:idx]
	}
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		if idSegmentPattern.MatchString(seg) || len(seg) > 24 {
			segments[i] = "{id}"
		}
	}
	return strings.Join(segments, "/")
}
