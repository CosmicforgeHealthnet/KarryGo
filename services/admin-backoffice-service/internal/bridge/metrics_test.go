package bridge

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrivateMetricsAreLowCardinalityAndProbeOnly(t *testing.T) {
	f := newFixture(t)
	rec := httptest.NewRecorder()
	f.b.Probe(rec, httptest.NewRequest("GET", "/metrics", nil))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "karryam_admin_requests_denied_total") {
		t.Fatalf("metrics %d %s", rec.Code, body)
	}
	for _, forbidden := range []string{"staff-1", "Bearer", "org-1", "nonce", "token"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metrics leak %q", forbidden)
		}
	}
	// The mTLS HQ router never serves metrics.
	rec = httptest.NewRecorder()
	f.b.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code == 200 {
		t.Fatal("metrics reachable on HQ listener")
	}
}
