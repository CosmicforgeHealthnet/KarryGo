package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"
)

// routerWithRequestID mirrors main.go: httpx.RequestID sets X-Request-ID on every response.
func routerWithRequestID(t *testing.T) *gin.Engine {
	t.Helper()
	svc, _ := newTestService()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(httpx.RequestID())
	RegisterPartnerRoutes(r, Dependencies{KeyRepo: newTestRepo(t), Deliveries: svc})
	return r
}

func TestRequestIDHeaderPresent(t *testing.T) {
	r := routerWithRequestID(t)
	// Even an unauthenticated (401) response must carry X-Request-ID.
	w := requestBody(r, http.MethodPost, "/api/v1/partner/v1/quotes", "", false)
	if w.Header().Get("X-Request-ID") == "" {
		t.Fatal("X-Request-ID header missing on response")
	}
}

// envelopeShape asserts a Partner API error envelope: success=false, error has a
// non-empty code, a message, and a fields array (never null).
func assertEnvelope(t *testing.T, raw []byte) {
	t.Helper()
	var env struct {
		Success *bool `json:"success"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Fields  []any  `json:"fields"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("not JSON: %v (%s)", err, raw)
	}
	if env.Success == nil || *env.Success {
		t.Fatalf("success != false: %s", raw)
	}
	if env.Error == nil || env.Error.Code == "" || env.Error.Message == "" {
		t.Fatalf("error code/message missing: %s", raw)
	}
	if !strings.Contains(string(raw), `"fields"`) {
		t.Fatalf("error envelope missing fields array: %s", raw)
	}
}

func TestPartnerErrorsUseConsistentEnvelope(t *testing.T) {
	r := routerWithRequestID(t)

	// missing api key -> 401
	assertEnvelope(t, requestBody(r, http.MethodPost, "/api/v1/partner/v1/quotes", "", false).Body.Bytes())
	// validation error -> 400
	assertEnvelope(t, requestBody(r, http.MethodPost, "/api/v1/partner/v1/quotes", `{}`, true).Body.Bytes())
	// booking not found -> 404
	assertEnvelope(t, requestBody(r, http.MethodGet, "/api/v1/partner/v1/deliveries/KGP-NONE0000", "", true).Body.Bytes())
}

func TestAPIKeyNotEchoedInResponses(t *testing.T) {
	r := routerWithRequestID(t)
	// A valid key that authenticates then fails validation must not echo the key.
	w := requestBody(r, http.MethodPost, "/api/v1/partner/v1/quotes", `{}`, true)
	if strings.Contains(w.Body.String(), testRawKey) {
		t.Fatalf("response leaked the API key: %s", w.Body.String())
	}
}
