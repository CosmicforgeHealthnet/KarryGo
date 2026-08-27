// Package promotioncontract pins the promotions management surface to its
// capability-named OpenAPI document.
package promotioncontract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/promotions"
)

type document struct {
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

func normalize(method, path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}
	return strings.ToUpper(method) + " " + strings.Join(parts, "/")
}

func sorted(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func TestPromotionRoutesEqualOpenAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	promotions.RegisterRoutes(router.Group("/api/v1/vendor"), promotions.NewHandler(promotions.NewService(nil, nil, nil, true)), true)
	code := map[string]bool{}
	for _, route := range router.Routes() {
		code[normalize(route.Method, route.Path)] = true
	}

	raw, err := os.ReadFile("../../docs/openapi/promotions-delivery-funding.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec document
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for path, methods := range spec.Paths {
		for method := range methods {
			switch method {
			case "get", "post", "put", "patch", "delete":
				documented[normalize(method, path)] = true
			}
		}
	}
	if len(code) != 7 || strings.Join(sorted(code), "\n") != strings.Join(sorted(documented), "\n") {
		t.Fatalf("promotion route/OpenAPI drift\ncode=%v\nspec=%v", sorted(code), sorted(documented))
	}
}
