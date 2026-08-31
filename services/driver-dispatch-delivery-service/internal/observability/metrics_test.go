package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMetricsBoundLabelsAndRecordFailuresConcurrently(t *testing.T) {
	var wg sync.WaitGroup
	for n := 0; n < 50; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Handover("verification", "SECRET_STAGE", "SECRET_METHOD", errors.New("SECRET_ERROR"))
			Worker("assignment", time.Now(), context.DeadlineExceeded)
		}()
	}
	wg.Wait()
	value := Metrics.CounterValue("dispatch_handover_verification_total", map[string]string{"stage": "other", "method": "other", "outcome": "rejected_or_failed"})
	if value < 50 {
		t.Fatal("lost concurrent observations")
	}
	text := Metrics.Render()
	if strings.Contains(text, "SECRET_") {
		t.Fatal("unbounded telemetry leaked input")
	}
	if !strings.Contains(text, "timeout_unknown") || !strings.Contains(text, "dispatch_worker_processing_seconds_count") {
		t.Fatal("worker failure/latency missing")
	}
}

func TestDependencyMetricsDoNotInspectURLBodyOrHeaders(t *testing.T) {
	req := httptest.NewRequest("POST", "http://wallet.test/SECRET_PATH?otp=SECRET_QUERY", strings.NewReader("SECRET_BODY"))
	req.Header.Set("Authorization", "SECRET_HEADER")
	for _, remoteErr := range []error{nil, context.DeadlineExceeded} {
		transport := DependencyTransport{Hosts: map[string]string{"wallet.test": "wallet"}, Base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r != req {
				t.Fatal("request replaced")
			}
			return &http.Response{StatusCode: 503}, remoteErr
		})}
		_, err := transport.RoundTrip(req)
		if !errors.Is(err, remoteErr) {
			t.Fatal("transport outcome changed")
		}
	}
	rendered := Metrics.Render()
	if strings.Contains(rendered, "SECRET_") {
		t.Fatal("request leaked")
	}
	if !strings.Contains(rendered, `dependency="wallet"`) || !strings.Contains(rendered, "remote_error") {
		t.Fatal("dependency metrics missing")
	}
}

func TestDurableMetricsCoverSemanticBacklogsWithBoundedLabels(t *testing.T) {
	want := map[string]bool{"dispatch_assignment_offer_total": false, "dispatch_offer_latency_seconds_bucket": false, "dispatch_queue_depth": false, "dispatch_delivery_attempt_total": false, "dispatch_route_alert_total": false, "dispatch_replacement_total": false, "dispatch_event_outbox_depth": false, "dispatch_reconciliation_backlog": false, "dispatch_booking_notification_backlog": false}
	for _, q := range durableQueries {
		if _, ok := want[q.name]; ok {
			want[q.name] = true
		}
		if len(q.allowedA) == 0 || len(q.allowedB) == 0 {
			t.Fatal("unbounded labels")
		}
		if strings.Contains(q.labelA, "_id") || strings.Contains(q.labelB, "_id") {
			t.Fatal("identity label")
		}
	}
	for metric, present := range want {
		if !present {
			t.Errorf("missing %s", metric)
		}
	}
	for _, q := range durableQueries {
		if q.name == "dispatch_queue_depth" || q.name == "dispatch_event_outbox_depth" {
			if !strings.Contains(q.sql, "LEFT JOIN") {
				t.Fatal("empty backlog cannot reset stale gauge")
			}
		}
	}
}

func TestWorkerRunningStopsExactlyOnce(t *testing.T) {
	stop := WorkerRunning("collector")
	stop()
	stop()
	if !strings.Contains(Metrics.Render(), `dispatch_workers_running{worker="collector"} 0`) {
		t.Fatal("worker running state leaked or decremented twice")
	}
}
