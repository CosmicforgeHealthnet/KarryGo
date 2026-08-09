// Package tracing implements W3C Trace Context propagation
// (https://www.w3.org/TR/trace-context/) without an external tracing SDK or
// collector.
//
// The repository has no existing distributed-tracing library or approved
// exporter -- the same situation internal/observability's own doc comment
// already documents for metrics ("The repository has no existing metrics
// library; this registry is a small, dependency-free ... store"). This
// package follows the identical precedent: a small, dependency-free
// traceparent parser/generator plus a span-recording helper that emits
// spans as structured log lines (internal/observability.Log) and duration
// metrics (internal/observability.Metrics) instead of shipping to an OTLP
// collector. Any future real exporter can be added behind StartSpan's
// signature without changing call sites, and a missing/unconfigured
// backend can never block core service operation because there is no
// network call in the critical path -- span recording is local logging and
// in-memory metrics only.
//
// Two propagation surfaces are supported:
//
//  1. Synchronous, over the wire: the standard "traceparent" (and, opaquely,
//     "tracestate") HTTP headers, extracted on inbound requests and
//     re-issued on every outbound platformclients call -- see
//     internal/middleware's Tracing() and internal/platformclients/base.go's
//     Exchange.
//  2. Asynchronous, across a durable boundary (a job row, an outbox event):
//     these do not carry HTTP headers, so they link back to the originating
//     trace via the correlationId field every Job and Envelope already
//     carries (see internal/jobs.Job.CorrelationID and
//     internal/events.Envelope.CorrelationID) rather than a new persisted
//     trace-id column -- correlationId was already threaded end to end
//     through this exact path before this package existed, and duplicating
//     it into a second identifier would only create two IDs to reconcile
//     instead of one. An async span's own traceId/spanId are freshly
//     generated for that unit of work and logged alongside the inherited
//     correlationId, so an operator can pivot from "the HTTP request log
//     line" to "every job/event log line for the same correlationId" even
//     though the two are not, strictly, one continuous W3C trace.
package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	mrand "math/rand/v2"
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

const (
	version      = "00"
	flagSampled  = 0x01
	traceIDBytes = 16
	spanIDBytes  = 8
)

var zeroTraceID = strings.Repeat("0", traceIDBytes*2)
var zeroSpanID = strings.Repeat("0", spanIDBytes*2)

// Context carries a W3C trace/span identity. Never put secrets or personal
// data here -- it is logged and sent as an outbound HTTP header verbatim.
type Context struct {
	TraceID string // 32 lowercase hex chars
	SpanID  string // 16 lowercase hex chars
	Sampled bool
	// State is the raw "tracestate" value, forwarded opaquely (capped, see
	// maxTraceStateLen) and never parsed or trusted for a business decision.
	State string
}

// IsZero reports an empty Context (no trace established).
func (c Context) IsZero() bool { return c.TraceID == "" || c.SpanID == "" }

// New starts a fresh root trace.
func New(sampled bool) Context {
	return Context{TraceID: randomHex(traceIDBytes), SpanID: randomHex(spanIDBytes), Sampled: sampled}
}

// ChildSpan derives a new span within the same trace -- used for every
// outbound call and every internal unit of work performed while handling
// one inbound request.
func (c Context) ChildSpan() Context {
	if c.IsZero() {
		return New(false)
	}
	return Context{TraceID: c.TraceID, SpanID: randomHex(spanIDBytes), Sampled: c.Sampled, State: c.State}
}

// TraceParent renders the standard "traceparent" header value.
func (c Context) TraceParent() string {
	flags := "00"
	if c.Sampled {
		flags = "01"
	}
	return version + "-" + c.TraceID + "-" + c.SpanID + "-" + flags
}

const maxTraceStateLen = 256

// ParseTraceParent parses an inbound "traceparent" header. An unrecognized
// version, malformed segment, or the reserved all-zero trace/span ID is
// rejected (ok=false) rather than trusted -- the caller falls back to
// starting a fresh root trace, exactly as the spec requires for an invalid
// header.
func ParseTraceParent(header string) (Context, bool) {
	header = strings.TrimSpace(header)
	parts := strings.Split(header, "-")
	if len(parts) != 4 {
		return Context{}, false
	}
	ver, traceID, spanID, flags := parts[0], parts[1], parts[2], parts[3]
	if !isHex(ver, 2) || !isHex(traceID, traceIDBytes*2) || !isHex(spanID, spanIDBytes*2) || !isHex(flags, 2) {
		return Context{}, false
	}
	if traceID == zeroTraceID || spanID == zeroSpanID {
		return Context{}, false
	}
	sampled := flags[len(flags)-1] == '1'
	return Context{TraceID: traceID, SpanID: spanID, Sampled: sampled}, true
}

// WithTraceState attaches a capped, opaque "tracestate" value.
func (c Context) WithTraceState(state string) Context {
	state = strings.TrimSpace(state)
	if len(state) > maxTraceStateLen {
		state = state[:maxTraceStateLen]
	}
	c.State = state
	return c
}

func isHex(s string, length int) bool {
	if len(s) != length {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read on this platform does not fail in practice; a
		// span identity is not a security boundary, so degrade to a
		// still-unique, non-cryptographic value rather than panicking a
		// request over an observability concern.
		for i := range b {
			b[i] = byte(mrand.IntN(256))
		}
	}
	return hex.EncodeToString(b)
}

// ShouldSample applies a configurable sample rate (0..1) to a NEW root
// trace only -- a trace propagated from an upstream caller always keeps the
// upstream's own sampling decision (W3C "head-based" sampling: the first
// service to see the request decides for the whole trace).
func ShouldSample(rate float64) bool {
	if rate <= 0 {
		return false
	}
	if rate >= 1 {
		return true
	}
	return mrand.Float64() < rate
}

type ctxKey struct{}

// WithContext attaches a trace Context to ctx.
func WithContext(ctx context.Context, tc Context) context.Context {
	return context.WithValue(ctx, ctxKey{}, tc)
}

// FromContext reads the trace Context, if any.
func FromContext(ctx context.Context) (Context, bool) {
	tc, ok := ctx.Value(ctxKey{}).(Context)
	return tc, ok && !tc.IsZero()
}

// End finalizes a span: records its duration metric (bounded, low-
// cardinality labels only) and emits one structured log line. err's safe
// class/code are recorded, never the raw error text.
type End func(err error)

// StartSpan begins a span named by a bounded, caller-supplied name (a route
// template, an operation name, a service name -- never a raw ID or URL) and
// returns a child context plus the completion callback. Safe to call with a
// nil metrics registry or an empty parent Context (a new root trace is
// started transparently); tracing must never require a live backend to
// function.
func StartSpan(ctx context.Context, metrics *observability.Metrics, service, environment, spanName string, extra map[string]any) (context.Context, End) {
	parent, ok := FromContext(ctx)
	if !ok {
		parent = New(true)
	}
	child := parent.ChildSpan()
	spanCtx := WithContext(ctx, child)
	start := time.Now()

	return spanCtx, func(err error) {
		duration := time.Since(start)
		if metrics != nil {
			metrics.ObserveDuration("fleet_partner_span_duration", map[string]string{"span": spanName}, duration)
			if err != nil {
				metrics.IncCounter("fleet_partner_span_errors_total", map[string]string{"span": spanName}, 1)
			}
		}
		fields := map[string]any{
			"traceId":    child.TraceID,
			"spanId":     child.SpanID,
			"span":       spanName,
			"durationMs": duration.Milliseconds(),
		}
		for k, v := range extra {
			fields[k] = v
		}
		level := "info"
		if err != nil {
			level = "error"
			fields["safeErrorClass"] = "span_error"
		}
		observability.Log(service, environment, level, "span", fields)
	}
}
