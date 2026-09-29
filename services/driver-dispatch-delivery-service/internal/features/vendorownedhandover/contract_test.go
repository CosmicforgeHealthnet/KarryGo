package vendorownedhandover

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

func TestOpenAPIContainsEveryVendorOwnedAuthorityRoute(t *testing.T) {
	raw, err := os.ReadFile("../../../docs/openapi/vendor-owned-secure-handover.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{"/missions": "post", "/missions/{missionRef}/handover/{stage}": "get", "/missions/{missionRef}/handover/{stage}/confirm": "post", "/missions/{missionRef}/handover/{stage}/resend": "post", "/missions/{missionRef}/reassignments": "post", "/missions/{missionRef}/receiver-changes": "post", "/missions/{missionRef}/cancellations": "post"}
	if len(spec.Paths) != len(expected) {
		t.Fatalf("paths=%d want=%d", len(spec.Paths), len(expected))
	}
	for path, method := range expected {
		if spec.Paths[path] == nil || spec.Paths[path][method] == nil {
			t.Fatalf("missing %s %s", method, path)
		}
	}
}

func TestReceiverCredentialRouteIsMountedAndDocumented(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	RegisterReceiverCredentialRead(engine, NewHandler(NewService(&Repository{}, Policy{Enabled: true})), sharedauth.NewTokenSigner([]byte("test-customer-secret")))
	const path = "/api/v1/customer/vendor-owned/missions/{missionRef}/handover/delivery/{challengeRef}/credential"
	var found bool
	for _, route := range engine.Routes() {
		if route.Method == http.MethodGet && route.Path == "/api/v1/customer/vendor-owned/missions/:missionRef/handover/delivery/:challengeRef/credential" {
			found = true
		}
	}
	if !found {
		t.Fatal("receiver credential route is not mounted")
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
	if spec.Paths[path]["get"] == nil {
		t.Fatal("receiver credential route missing from OpenAPI")
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/customer/vendor-owned/missions/mission/handover/delivery/challenge/credential", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous credential read status=%d", response.Code)
	}
}
