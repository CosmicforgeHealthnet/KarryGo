package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
)

// The verification decision ingress refuses callers that are not allowlisted
// and envelopes it cannot apply, before touching any state.
func TestVerificationEventIngressRefusals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewVerificationEventHandler(nil, []string{"verification-compliance-service"})
	call := func(caller, body string) (int, string) {
		router := gin.New()
		group := router.Group("/internal/fleet-partner/v1", func(c *gin.Context) { c.Set(middleware.ServiceCallerKey, caller) })
		RegisterVerificationEventRoutes(group, handler)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/internal/fleet-partner/v1"+VerificationEventsPath, strings.NewReader(body)))
		return w.Code, w.Body.String()
	}
	valid := `{"eventId":"e1","eventType":"organization.verification.status_changed.v1","organizationId":"org_1","payload":{"status":"VERIFIED","sourceVersion":2}}`
	if code, body := call("driver-taxi-service", valid); code != 403 || !strings.Contains(body, "caller_not_allowed") {
		t.Fatalf("other caller: %d %s", code, body)
	}
	for _, body := range []string{`not json`, `{"eventId":"e1","organizationId":"org_1","payload":{"status":"VERIFIED"}}`, `{"organizationId":"org_1","payload":{"status":"VERIFIED","sourceVersion":1}}`} {
		if code, out := call("verification-compliance-service", body); code != 400 || !strings.Contains(out, "invalid_envelope") {
			t.Fatalf("bad envelope %q: %d %s", body, code, out)
		}
	}
	if code, out := call("verification-compliance-service", `{"eventId":"e1","eventType":"something.else.v1","organizationId":"org_1","payload":{"status":"VERIFIED","sourceVersion":1}}`); code != 200 || !strings.Contains(out, "IGNORED") {
		t.Fatalf("unsupported type: %d %s", code, out)
	}
}
