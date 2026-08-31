package wallethttp

import (
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"os"
	"strings"
	"testing"
)

func TestDeliveryResponsibilityRoutesEqualOpenAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/payment-wallet"), nil, []byte("customer"), nil, serviceauth.Secrets{"vendor-service": []byte("secret")})
	code := map[string]bool{}
	for _, route := range router.Routes() {
		if strings.Contains(route.Path, "delivery-responsibility") {
			path := strings.ReplaceAll(strings.ReplaceAll(route.Path, ":vendorOrderId", "{vendorOrderId}"), ":operationRef", "{operationRef}")
			code[route.Method+" "+path] = true
		}
	}
	raw, err := os.ReadFile("../../../../docs/openapi/commerce-delivery-responsibility.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for path, methods := range spec.Paths {
		for method := range methods {
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}
	if len(code) != 2 || len(documented) != 2 {
		t.Fatalf("route count code=%v spec=%v", code, documented)
	}
	for route := range code {
		if !documented[route] {
			t.Fatalf("missing OpenAPI route %s", route)
		}
	}
}
