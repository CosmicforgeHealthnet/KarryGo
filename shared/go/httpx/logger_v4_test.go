package httpx

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// Customer V4 §27.1: one structured access record per request, carrying the
// correlation fields and bounded outcome, never raw path parameters, query,
// headers or client-supplied identifiers.
func TestLoggerEmitsStructuredAccessRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	before := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(before) })
	log.SetFlags(0)
	t.Cleanup(func() { log.SetFlags(log.LstdFlags) })

	r := gin.New()
	r.Use(RequestID(), Logger())
	r.GET("/shares/:token", func(c *gin.Context) { c.Status(http.StatusConflict) })
	req := httptest.NewRequest(http.MethodGet, "/shares/SECRET_SHARE_TOKEN?otp=SECRET_QUERY", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	req.Header.Set("X-Request-ID", "SECRET_CLIENT_ID")
	r.ServeHTTP(httptest.NewRecorder(), req)

	var record map[string]any
	line := strings.TrimSpace(logs.String())
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("access log is not structured JSON: %q", line)
	}
	want := map[string]any{"event": "http_request", "route": "/shares/:token", "method": "GET", "outcome": "conflict",
		"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736"}
	for key, value := range want {
		if record[key] != value {
			t.Errorf("%s = %v, want %v", key, record[key], value)
		}
	}
	if _, ok := record["latency_ms"]; !ok {
		t.Error("latency missing")
	}
	if id, _ := record["request_id"].(string); !strings.HasPrefix(id, "request-") {
		t.Errorf("client-supplied request id not hashed: %v", record["request_id"])
	}
	if strings.Contains(line, "SECRET_") {
		t.Fatalf("access log leaks: %s", line)
	}
}

func TestTraceIDRejectsMalformedTraceparent(t *testing.T) {
	for _, header := range []string{"", "garbage", "00-XYZ92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01", "00-4bf92f3577b34da6a3ce929d0e0e4736-01"} {
		if got := TraceID(header); got != "" {
			t.Errorf("%q accepted as %q", header, got)
		}
	}
}
