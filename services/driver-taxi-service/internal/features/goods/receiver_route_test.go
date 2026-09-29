package goods

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

func TestTaxiGoodsReceiverReadRequiresCustomerBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	signer := sharedauth.NewTokenSigner([]byte("receiver-route-test-secret"))
	RegisterReceiverRoutes(engine.Group("/api/v1"), NewService(Options{Enabled: true}), signer)
	path := "/api/v1/customer/goods/missions/mission/delivery/credentials/challenge"
	var mounted bool
	for _, route := range engine.Routes() {
		if route.Method == http.MethodGet && route.Path == "/api/v1/customer/goods/missions/:missionRef/delivery/credentials/:challengeRef" {
			mounted = true
		}
	}
	if !mounted {
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
	if spec.Paths["/api/v1/customer/goods/missions/{missionRef}/delivery/credentials/{challengeRef}"]["get"] == nil {
		t.Fatal("receiver credential route missing from OpenAPI")
	}
	for _, token := range []string{"", signedReceiverRouteToken(t, signer, "taxi_provider", "taxi")} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		if response.Code == http.StatusOK {
			t.Fatalf("non-customer session read credential: %d", response.Code)
		}
	}
}

func signedReceiverRouteToken(t *testing.T, signer *sharedauth.TokenSigner, role, service string) string {
	t.Helper()
	now := time.Now()
	token, err := signer.Sign(sharedauth.Claims{Subject: "provider", Role: role, Service: service, SessionID: "session", Type: sharedauth.TokenTypeAccess, IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	return token
}
