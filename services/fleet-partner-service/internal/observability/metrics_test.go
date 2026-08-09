package observability

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCountersGaugesAndDurations(t *testing.T) {
	metrics := NewMetrics()
	metrics.IncCounter("requests_total", map[string]string{"route": "/x"}, 1)
	metrics.IncCounter("requests_total", map[string]string{"route": "/x"}, 2)
	metrics.SetGauge("backlog", nil, 7)
	metrics.ObserveDuration("latency", nil, 250*time.Millisecond)

	rendered := metrics.Render()
	for _, want := range []string{
		`requests_total{route="/x"} 3`,
		"backlog 7",
		"latency_seconds_count 1",
		"latency_seconds_sum 0.25",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("render missing %q:\n%s", want, rendered)
		}
	}
}

func TestConcurrentAccessSafe(t *testing.T) {
	metrics := NewMetrics()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				metrics.IncCounter("c", nil, 1)
				metrics.AddGauge("g", nil, 1)
				metrics.ObserveDuration("d", nil, time.Millisecond)
			}
		}()
	}
	wg.Wait()
	if got := metrics.CounterValue("c", nil); got != 2000 {
		t.Fatalf("counter = %g", got)
	}
}

func TestLogRedactsSensitiveFields(t *testing.T) {
	var buf bytes.Buffer
	original := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(original)

	Log("svc", "test", "info", "message", map[string]any{
		"authorization": "Bearer super-secret-value",
		"refresh_token": "rt_super_secret",
		"api_key":       "ak_super_secret",
		"bvn":           "12345678901",
		"route":         "/x",
	})

	line := buf.String()
	for _, secret := range []string{"super-secret-value", "rt_super_secret", "ak_super_secret", "12345678901"} {
		if strings.Contains(line, secret) {
			t.Fatalf("log line leaked a sensitive value: %s", line)
		}
	}
	if !strings.Contains(line, `"route":"/x"`) {
		t.Fatalf("non-sensitive field was dropped: %s", line)
	}
}
