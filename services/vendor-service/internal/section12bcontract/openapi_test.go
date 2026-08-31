package section12bcontract

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/shop"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type document struct {
	Servers []struct {
		URL string `json:"url"`
	} `json:"servers"`
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]json.RawMessage `json:"schemas"`
	} `json:"components"`
}

func load(t *testing.T, path string) document {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc document
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}
func normalize(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}
func operations(doc document) map[string]bool {
	out := map[string]bool{}
	prefix := doc.Servers[0].URL
	for path, methods := range doc.Paths {
		for method := range methods {
			if contains([]string{"get", "post", "put", "patch", "delete"}, method) {
				out[strings.ToUpper(method)+" "+prefix+path] = true
			}
		}
	}
	return out
}
func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func TestSection12BPublicAndInternalRouteParity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	flags := shop.Flags{ShopEnabled: true, WritesEnabled: true}
	service := shop.NewService(shop.Options{Flags: flags})
	shop.RegisterRoutes(router.Group("/api/v1/vendor"), shop.NewHandler(service, idempotency.NewStore(nil, time.Hour)), flags)
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{}, time.Minute)
	shop.RegisterInternalInventoryRoutes(router, shop.NewInventoryInternalHandler(service, []string{"test-caller"}), verifier, flags)
	actual := map[string]bool{}
	for _, route := range router.Routes() {
		if strings.Contains(route.Path, "/inventory") || strings.Contains(route.Path, "/bulk-operations") || strings.HasPrefix(route.Path, "/internal/vendor/v1/shop") {
			actual[route.Method+" "+normalize(route.Path)] = true
		}
	}
	documented := operations(load(t, "../../docs/openapi/section12b-shop-inventory.openapi.json"))
	for key := range operations(load(t, "../../docs/openapi/section12-shop-discovery.openapi.json")) {
		documented[key] = true
	}
	if len(actual) != 53 || len(documented) != 53 {
		t.Fatalf("Section 12B routes code=%d spec=%d want=53", len(actual), len(documented))
	}
	for key := range actual {
		if !documented[key] {
			t.Errorf("undocumented route %s", key)
		}
	}
	for key := range documented {
		if !actual[key] {
			t.Errorf("phantom route %s", key)
		}
	}
}

func TestSection12BContractsAreStrictAndBounded(t *testing.T) {
	for _, path := range []string{"../../docs/openapi/section12b-shop-inventory.openapi.json", "../../docs/openapi/section12-shop-discovery.openapi.json"} {
		doc := load(t, path)
		var command struct {
			AdditionalProperties *bool                      `json:"additionalProperties"`
			Properties           map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(doc.Components.Schemas["InventoryCommand"], &command); err != nil {
			t.Fatal(err)
		}
		if command.AdditionalProperties == nil || *command.AdditionalProperties {
			t.Errorf("%s inventory command is not strict", path)
		}
		for _, field := range []string{"lines", "quantity", "stock_basis_id", "source_ref"} {
			if _, ok := command.Properties[field]; !ok {
				t.Errorf("%s command missing %s", path, field)
			}
		}
	}
}
