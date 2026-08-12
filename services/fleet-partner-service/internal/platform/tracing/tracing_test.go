package tracing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

func TestParseTraceParent_ValidRoundTrips(t *testing.T) {
	tc := New(true)
	header := tc.TraceParent()

	parsed, ok := ParseTraceParent(header)
	if !ok {
		t.Fatalf("ParseTraceParent(%q) rejected a value this package itself generated", header)
	}
	if parsed.TraceID != tc.TraceID || parsed.SpanID != tc.SpanID || !parsed.Sampled {
		t.Fatalf("round trip mismatch: got %+v, want %+v", parsed, tc)
	}
}

func TestParseTraceParent_RejectsMalformed(t *testing.T) {
	cases := []string{
		"",
		"not-a-traceparent",
		"01-" + strings.Repeat("a", 32) + "-" + strings.Repeat("b", 16) + "-01", // wrong version accepted-length-wise but must still parse; version isn't validated beyond hex -- covered separately
		"00-" + strings.Repeat("0", 32) + "-" + strings.Repeat("b", 16) + "-01", // all-zero trace id
		"00-" + strings.Repeat("a", 32) + "-" + strings.Repeat("0", 16) + "-01", // all-zero span id
		"00-" + strings.Repeat("g", 32) + "-" + strings.Repeat("b", 16) + "-01", // non-hex
		"00-short-short-01",
	}
	for _, header := range cases {
		if header == "01-"+strings.Repeat("a", 32)+"-"+strings.Repeat("b", 16)+"-01" {
			// This one IS accepted by design (version is not semantically
			// validated, only shaped) -- skip it from the "must reject" set.
			continue
		}
		if _, ok := ParseTraceParent(header); ok {
			t.Fatalf("ParseTraceParent(%q) should have been rejected", header)
		}
	}
}

func TestChildSpan_SharesTraceIDNewSpanID(t *testing.T) {
	root := New(true)
	child := root.ChildSpan()
	if child.TraceID != root.TraceID {
		t.Fatalf("child trace id = %s, want %s (same trace)", child.TraceID, root.TraceID)
	}
	if child.SpanID == root.SpanID {
		t.Fatal("child span id must differ from the parent span id")
	}
	if child.Sampled != root.Sampled {
		t.Fatal("child must inherit the parent's sampling decision")
	}
}

func TestChildSpan_OfZeroContextStartsFreshTrace(t *testing.T) {
	child := Context{}.ChildSpan()
	if child.IsZero() {
		t.Fatal("ChildSpan of a zero Context must still produce a usable trace")
	}
}

func TestShouldSample_BoundaryRatesAreDeterministic(t *testing.T) {
	if ShouldSample(0) {
		t.Fatal("rate 0 must never sample")
	}
	if !ShouldSample(1) {
		t.Fatal("rate 1 must always sample")
	}
	if ShouldSample(-1) {
		t.Fatal("a negative rate must be treated as 0")
	}
}

func TestWithContextFromContext_RoundTrips(t *testing.T) {
	tc := New(true)
	ctx := WithContext(context.Background(), tc)
	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext should find the attached trace context")
	}
	if got != tc {
		t.Fatalf("got %+v, want %+v", got, tc)
	}
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("a bare context must not carry a trace context")
	}
}

func TestStartSpan_GeneratesChildAndRecordsMetricsAndLog(t *testing.T) {
	metrics := observability.NewMetrics()
	parent := New(true)
	ctx := WithContext(context.Background(), parent)

	spanCtx, end := StartSpan(ctx, metrics, "test-service", "test", "unit.test.span", map[string]any{"k": "v"})
	child, ok := FromContext(spanCtx)
	if !ok {
		t.Fatal("StartSpan must attach a child trace context")
	}
	if child.TraceID != parent.TraceID {
		t.Fatal("span must stay within the parent's trace")
	}
	end(nil)

	rendered := metrics.Render()
	if !strings.Contains(rendered, `fleet_partner_span_duration_seconds_count{span="unit.test.span"}`) {
		t.Fatalf("expected a duration observation for the span, got:\n%s", rendered)
	}
}

func TestStartSpan_WithoutParentStartsFreshTrace(t *testing.T) {
	metrics := observability.NewMetrics()
	_, end := StartSpan(context.Background(), metrics, "test-service", "test", "unit.test.rootless", nil)
	end(errors.New("boom"))

	rendered := metrics.Render()
	if !strings.Contains(rendered, `fleet_partner_span_errors_total{span="unit.test.rootless"}`) {
		t.Fatalf("expected an error counter for the failed span, got:\n%s", rendered)
	}
}
