package booking

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustomerProviderOperationsOpenAPIContract(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "docs", "openapi", "customer-provider-dispatch-operations.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("invalid OpenAPI document: %v", err)
	}
	text := string(raw)
	for _, route := range []string{
		"/api/v1/customer/dispatch/bookings",
		"/api/v1/customer/dispatch/bookings/{id}",
		"/api/v1/provider/trips/{id}",
	} {
		if !strings.Contains(text, route) {
			t.Fatalf("OpenAPI document missing %s", route)
		}
	}
	register, err := os.ReadFile(filepath.Join(root, "docs", "API_ROUTE_REGISTER.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{
		"/api/v1/customer/dispatch/bookings",
		"/api/v1/customer/dispatch/bookings/:id",
		"/api/v1/provider/trips/:id",
	} {
		if !strings.Contains(string(register), route) {
			t.Fatalf("route register missing %s", route)
		}
	}
}
