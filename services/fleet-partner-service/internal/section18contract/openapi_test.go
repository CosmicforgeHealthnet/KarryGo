package section18contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	assetshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/http"
	fuelhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/fuel/http"
	inspectionshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/inspections/http"
	maintenancehttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/maintenance/http"
)

func TestRouteOpenAPIParity(t *testing.T) {
	_, f, _, _ := runtime.Caller(0)
	raw, e := os.ReadFile(filepath.Join(filepath.Dir(f), "..", "..", "docs", "openapi", "section18-assets-maintenance.openapi.json"))
	if e != nil {
		t.Fatal(e)
	}
	var s map[string]any
	if e = json.Unmarshal(raw, &s); e != nil {
		t.Fatal(e)
	}
	ops := map[string]map[string]any{}
	for p, v := range s["paths"].(map[string]any) {
		for m, o := range v.(map[string]any) {
			ops[strings.ToUpper(m)+" "+p] = o.(map[string]any)
		}
	}
	routes := section18Routes()
	if len(ops) != len(routes) {
		t.Fatalf("spec operations=%d routes=%d", len(ops), len(routes))
	}
	for _, r := range routes {
		p := r.Path
		for _, part := range strings.Split(p, "/") {
			if strings.HasPrefix(part, ":") {
				p = strings.Replace(p, part, "{"+strings.TrimPrefix(part, ":")+"}", 1)
			}
		}
		key := r.Method + " " + p
		o, ok := ops[key]
		if !ok {
			t.Errorf("missing %s", key)
			continue
		}
		if o["operationId"] != r.OperationID || o["x-permission"] != r.Permission || o["x-feature-flag"] != r.FeatureFlag || o["x-auth-class"] != r.AuthClass {
			t.Errorf("metadata mismatch %s", key)
		}
		if o["x-idempotency-required"] != r.IdempotencyRequired || o["x-expected-version-required"] != r.ExpectedVersionRequired {
			t.Errorf("command metadata mismatch %s", key)
		}
		delete(ops, key)
	}
	if len(ops) > 0 {
		t.Errorf("fictional spec operations: %v", ops)
	}
}

// section18Route is the route metadata every Section 18 feature declares.
type section18Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

// section18Routes is the Section 18 route catalogue: assets, inspections,
// maintenance and fuel together serve the one OpenAPI document.
func section18Routes() []section18Route {
	var out []section18Route
	for _, r := range assetshttp.TenantRoutes() {
		out = append(out, section18Route(r))
	}
	for _, r := range inspectionshttp.TenantRoutes() {
		out = append(out, section18Route(r))
	}
	for _, r := range maintenancehttp.TenantRoutes() {
		out = append(out, section18Route(r))
	}
	for _, r := range fuelhttp.TenantRoutes() {
		out = append(out, section18Route(r))
	}
	return out
}

func TestSection18RouteCatalogueAndNoFinancialSurface(t *testing.T) {
	routes := section18Routes()
	if len(routes) != 62 {
		t.Fatalf("routes=%d", len(routes))
	}
	seen := map[string]bool{}
	for _, r := range routes {
		k := r.Method + " " + r.Path
		if seen[k] {
			t.Fatalf("duplicate %s", k)
		}
		seen[k] = true
		if r.Permission == "" || r.FeatureFlag == "" || r.AuthClass != "corporate" {
			t.Fatalf("incomplete %+v", r)
		}
		lower := strings.ToLower(k)
		for _, bad := range []string{"wallet", "ledger", "payroll", "procurement", "invoice"} {
			if strings.Contains(lower, bad) {
				t.Fatalf("excluded surface %s", k)
			}
		}
	}
}
