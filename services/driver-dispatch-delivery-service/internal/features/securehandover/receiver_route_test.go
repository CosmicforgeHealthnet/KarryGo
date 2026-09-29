package securehandover

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
)

func TestDispatchReceiverCredentialRouteAuthAndOpenAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, err := NewService(&Repository{}, []byte("01234567890123456789012345678901"), Policy{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	RegisterCustomer(engine, sharedauth.NewTokenSigner([]byte("test-customer-secret")), NewHandler(service))
	const actual = "/api/v1/customer/dispatch/bookings/handover/:challengeRef/receiver-credential"
	var mounted bool
	for _, route := range engine.Routes() {
		if route.Method == http.MethodGet && route.Path == actual {
			mounted = true
		}
	}
	if !mounted {
		t.Fatal("Dispatch receiver credential route is not mounted")
	}
	raw, err := os.ReadFile("../../../docs/openapi/receiver-secure-credential.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.Paths["/api/v1/customer/dispatch/bookings/handover/{challengeRef}/receiver-credential"]["get"] == nil {
		t.Fatal("Dispatch receiver credential route missing from OpenAPI")
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/customer/dispatch/bookings/handover/challenge/receiver-credential", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous credential read status=%d", response.Code)
	}
}
