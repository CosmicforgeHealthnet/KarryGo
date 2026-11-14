package section16contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	dispatchhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/http"
	routeshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/routes/http"
	trackinghttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/tracking/http"
)

// route is the shared shape of the dispatch, routes and tracking manifests.
type route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

// tenantRoutes is the Section 16 corporate surface across the three
// features it is split into.
func tenantRoutes() []route {
	out := []route{}
	for _, r := range dispatchhttp.TenantRoutes() {
		out = append(out, route(r))
	}
	for _, r := range routeshttp.TenantRoutes() {
		out = append(out, route(r))
	}
	for _, r := range trackinghttp.TenantRoutes() {
		out = append(out, route(r))
	}
	return out
}

func internalRoutes() []route {
	out := []route{}
	for _, r := range trackinghttp.InternalRoutes() {
		out = append(out, route(r))
	}
	return out
}

func spec(t *testing.T) map[string]any {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	raw, e := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "docs", "openapi", "section16-dispatch-routes-control.openapi.json"))
	if e != nil {
		t.Fatal(e)
	}
	var out map[string]any
	if e = json.Unmarshal(raw, &out); e != nil {
		t.Fatal(e)
	}
	return out
}
func braces(p string) string {
	parts := strings.Split(p, "/")
	for i, v := range parts {
		if strings.HasPrefix(v, ":") {
			parts[i] = "{" + strings.TrimPrefix(v, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}
func TestSection16OpenAPIRouteParity(t *testing.T) {
	s := spec(t)
	paths := s["paths"].(map[string]any)
	ops := map[string]map[string]any{}
	for path, raw := range paths {
		for method, value := range raw.(map[string]any) {
			if method != "get" && method != "post" && method != "patch" && method != "delete" {
				continue
			}
			ops[strings.ToUpper(method)+" "+path] = value.(map[string]any)
		}
	}
	routes := append(tenantRoutes(), internalRoutes()...)
	if len(ops) != len(routes) {
		t.Fatalf("spec operations=%d mounted operations=%d", len(ops), len(routes))
	}
	for _, r := range routes {
		prefix := "/fleet-partner/v1"
		if r.AuthClass == "internal-hmac-v2" {
			prefix = "/internal/fleet-partner/v1"
		}
		key := r.Method + " " + prefix + braces(r.Path)
		op, ok := ops[key]
		if !ok {
			t.Errorf("mounted route missing from spec: %s", key)
			continue
		}
		if op["operationId"] != r.OperationID || op["x-auth-class"] != r.AuthClass || op["x-permission"] != r.Permission || op["x-feature-flag"] != r.FeatureFlag {
			t.Errorf("metadata mismatch %s spec=%v route=%+v", key, op, r)
		}
		if v, _ := op["x-idempotency-required"].(bool); v != r.IdempotencyRequired {
			t.Errorf("%s idempotency=%v want %v", key, v, r.IdempotencyRequired)
		}
		if v, _ := op["x-expected-version-required"].(bool); v != r.ExpectedVersionRequired {
			t.Errorf("%s expected version=%v want %v", key, v, r.ExpectedVersionRequired)
		}
		delete(ops, key)
	}
	for key := range ops {
		t.Errorf("undocumented mounted route or fictional spec route: %s", key)
	}
}
func TestSection16OpenAPIScopeAndSchemas(t *testing.T) {
	s := spec(t)
	paths := s["paths"].(map[string]any)
	if len(paths) != 36 {
		t.Fatalf("path count=%d", len(paths))
	}
	schemas := s["components"].(map[string]any)["schemas"].(map[string]any)
	if len(schemas) < 16 {
		t.Fatalf("schema count=%d", len(schemas))
	}
	raw, _ := json.Marshal(s)
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"/partner/", "/taxi/", "workforce", "maintenance schedule", "payroll", "predictive ai", "commission", "margin", "fare"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("deferred, fictional or commercial surface present: %s", forbidden)
		}
	}
	for _, required := range []string{"gapped", "hardblocks", "overridereason", "sourceversion", "resolutionreason", "passenger route publication is disabled"} {
		if !strings.Contains(lower, required) {
			t.Errorf("missing contract behavior %s", required)
		}
	}
}

func TestSection16RouteCatalogue(t *testing.T) {
	routes := tenantRoutes()
	if len(routes) != 44 {
		t.Fatalf("tenant route count=%d", len(routes))
	}
	seen := map[string]bool{}
	for _, r := range routes {
		k := r.Method + " " + r.Path
		if seen[k] {
			t.Fatalf("duplicate route %s", k)
		}
		seen[k] = true
		if r.Permission == "" || r.FeatureFlag == "" || r.AuthClass != "corporate" {
			t.Fatalf("incomplete route metadata %+v", r)
		}
	}
	for _, required := range []string{"GET /dispatch/board", "GET /dispatch/unassigned", "GET /dispatch/candidates", "POST /routes/:routeId/publish", "GET /control-tower", "GET /tracking/:canonicalTripReference", "POST /alerts/:alertId/resolve"} {
		if !seen[required] {
			t.Errorf("missing %s", required)
		}
	}
	for key := range seen {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "/partner") || strings.Contains(lower, "/taxi") {
			t.Errorf("deferred route present: %s", key)
		}
	}
}
