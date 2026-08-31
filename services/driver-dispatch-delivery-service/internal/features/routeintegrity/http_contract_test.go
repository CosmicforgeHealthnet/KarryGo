package routeintegrity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/auth"
)

func TestRouteIntegrityRouteContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	handler := NewHandler(&Service{})
	RegisterProvider(engine, nil, handler)
	RegisterCustomer(engine, auth.NewTokenSigner([]byte("01234567890123456789012345678901")), handler)
	want := map[string]bool{
		"POST /api/v1/provider/trips/:id/reroute":                 false,
		"POST /api/v1/provider/offline-commands":                  false,
		"GET /api/v1/customer/dispatch/bookings/:id/route-status": false,
	}
	for _, route := range engine.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for route, found := range want {
		if !found {
			t.Errorf("missing route %s", route)
		}
	}
	root := filepath.Join("..", "..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "docs", "openapi", "delivery-recovery-route-integrity.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]any `json:"paths"`
	}
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	register, err := os.ReadFile(filepath.Join(root, "docs", "API_ROUTE_REGISTER.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/provider/trips/{id}/reroute", "/api/v1/provider/offline-commands", "/api/v1/customer/dispatch/bookings/{id}/route-status"} {
		if _, ok := document.Paths[path]; !ok {
			t.Errorf("OpenAPI missing %s", path)
		}
		if !strings.Contains(string(register), strings.ReplaceAll(path, "{id}", ":id")) {
			t.Errorf("route register missing %s", path)
		}
	}
}
