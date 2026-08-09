package section15contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	bulkhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/http"
	manifestshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/http"
	ordershttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/http"
	packageshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/http"
	privatehttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/http"
	recurringhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/http"
	cataloghttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/http"
	shipmentshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/http"
)

type route struct {
	method, path, operation, permission, auth string
	idempotent, versioned                     bool
}

func braces(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}

func routes() []route {
	var out []route
	appendRoute := func(method, path, operation, permission string, idem, version bool) {
		out = append(out, route{method, "/fleet-partner/v1" + braces(path), operation, permission, "corporate", idem, version})
	}
	for _, r := range cataloghttp.NewHandler(nil, nil).TenantRoutes() {
		appendRoute(r.Method, r.Path, r.OperationID, r.Permission, r.IdempotencyRequired, r.ExpectedVersionRequired)
	}
	for _, r := range ordershttp.NewHandler(nil, nil).TenantRoutes() {
		appendRoute(r.Method, r.Path, r.OperationID, r.Permission, r.IdempotencyRequired, r.ExpectedVersionRequired)
	}
	for _, r := range shipmentshttp.NewHandler(nil, nil, nil).TenantRoutes() {
		appendRoute(r.Method, r.Path, r.OperationID, r.Permission, r.IdempotencyRequired, r.ExpectedVersionRequired)
	}
	for _, r := range packageshttp.NewHandler(nil, nil, nil).TenantRoutes() {
		appendRoute(r.Method, r.Path, r.OperationID, r.Permission, r.IdempotencyRequired, r.ExpectedVersionRequired)
	}
	for _, r := range manifestshttp.NewHandler(nil, nil, nil).TenantRoutes() {
		appendRoute(r.Method, r.Path, r.OperationID, r.Permission, r.IdempotencyRequired, r.ExpectedVersionRequired)
	}
	for _, r := range recurringhttp.NewHandler(nil, nil, nil).TenantRoutes() {
		appendRoute(r.Method, r.Path, r.OperationID, r.Permission, r.IdempotencyRequired, r.ExpectedVersionRequired)
	}
	for _, r := range bulkhttp.NewHandler(nil, nil, nil).TenantRoutes() {
		appendRoute(r.Method, r.Path, r.OperationID, r.Permission, r.IdempotencyRequired, r.ExpectedVersionRequired)
	}
	privateHandler := privatehttp.NewHandler(nil, nil, nil)
	for _, r := range privateHandler.TenantRoutes() {
		appendRoute(r.Method, r.Path, r.OperationID, r.Permission, r.IdempotencyRequired, r.ExpectedVersionRequired)
	}
	for _, r := range privateHandler.InternalRoutes() {
		out = append(out, route{r.Method, "/internal/fleet-partner/v1" + braces(r.Path), r.OperationID, r.Permission, "internal-hmac-v2", r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	return out
}

func load(t *testing.T) map[string]any {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "docs", "openapi", "section15-private-operations.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

func operations(t *testing.T, spec map[string]any) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	paths := spec["paths"].(map[string]any)
	for path, raw := range paths {
		for method, operationRaw := range raw.(map[string]any) {
			if method != "get" && method != "post" && method != "patch" && method != "put" && method != "delete" {
				continue
			}
			op := operationRaw.(map[string]any)
			key := strings.ToUpper(method) + " " + path
			if _, exists := out[key]; exists {
				t.Fatalf("duplicate operation %s", key)
			}
			out[key] = op
		}
	}
	return out
}

func TestOpenAPIRouteParity(t *testing.T) {
	actual := routes()
	specOps := operations(t, load(t))
	if len(specOps) != len(actual) {
		t.Fatalf("spec operations=%d route operations=%d", len(specOps), len(actual))
	}
	seen := map[string]bool{}
	for _, r := range actual {
		key := r.method + " " + r.path
		op, ok := specOps[key]
		if !ok {
			t.Errorf("route missing from spec: %s", key)
			continue
		}
		seen[key] = true
		if op["operationId"] != r.operation {
			t.Errorf("%s operationId=%v want %s", key, op["operationId"], r.operation)
		}
		if op["x-auth-class"] != r.auth {
			t.Errorf("%s auth=%v want %s", key, op["x-auth-class"], r.auth)
		}
		if op["x-permission"] != r.permission {
			t.Errorf("%s permission=%v want %s", key, op["x-permission"], r.permission)
		}
		if value, _ := op["x-idempotency-required"].(bool); value != r.idempotent {
			t.Errorf("%s idempotency=%v want %v", key, value, r.idempotent)
		}
		if value, _ := op["x-expected-version-required"].(bool); value != r.versioned {
			t.Errorf("%s expectedVersion=%v want %v", key, value, r.versioned)
		}
		if flag, _ := op["x-feature-flag"].(string); flag == "" {
			t.Errorf("%s missing feature flag", key)
		}
	}
	for key := range specOps {
		if !seen[key] {
			t.Errorf("spec operation not mounted: %s", key)
		}
	}
}

func TestOpenAPIExcludesDeferredSurfacesAndMinimizesProviderDTO(t *testing.T) {
	spec := load(t)
	raw, _ := json.Marshal(spec["paths"])
	paths := strings.ToLower(string(raw))
	for _, forbidden := range []string{"/partner", "/dispatch-board", "/route-plans", "/control-tower", "/taxi"} {
		if strings.Contains(paths, forbidden) {
			t.Errorf("deferred or fictional path present: %s", forbidden)
		}
	}
	components := spec["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	provider, _ := json.Marshal(schemas["ProviderAssignment"])
	lower := strings.ToLower(string(provider))
	for _, forbidden := range []string{"fare", "quotation", "contract", "settlement", "commission", "margin", "customercontact", "receiverphone"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("provider schema leaks %s", forbidden)
		}
	}
}
