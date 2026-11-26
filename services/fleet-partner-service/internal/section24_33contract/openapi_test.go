// Package section24_33contract proves the Sections 24-33 OpenAPI documents
// match the routes the handlers actually mount, in both directions, and
// that the analytics contract exposes no protected action.
package section24_33contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	analyticshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/http"
	analyticsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/models"
	governancehttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/http"
)

type route struct {
	method, path, operation, permission, flag, auth string
	idem, version                                   bool
}

func load(t *testing.T, name string) map[string]map[string]any {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "docs", "openapi", name))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]any{}
	for path, item := range doc["paths"].(map[string]any) {
		for method, op := range item.(map[string]any) {
			out[strings.ToUpper(method)+" "+path] = op.(map[string]any)
		}
	}
	return out
}

func braces(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			parts[i] = "{" + strings.TrimPrefix(p, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}

func check(t *testing.T, name string, routes []route) {
	t.Helper()
	ops := load(t, name)
	if len(ops) != len(routes) {
		t.Fatalf("%s operations=%d routes=%d", name, len(ops), len(routes))
	}
	for _, r := range routes {
		key := r.method + " /fleet-partner/v1" + braces(r.path)
		op, ok := ops[key]
		if !ok {
			t.Errorf("mounted route missing from %s: %s", name, key)
			continue
		}
		if op["operationId"] != r.operation || op["x-permission"] != r.permission || op["x-feature-flag"] != r.flag || op["x-auth-class"] != r.auth {
			t.Errorf("metadata mismatch %s", key)
		}
		if v, _ := op["x-idempotency-required"].(bool); v != r.idem {
			t.Errorf("idempotency mismatch %s", key)
		}
		if v, _ := op["x-expected-version-required"].(bool); v != r.version {
			t.Errorf("version mismatch %s", key)
		}
		if r.method != "GET" && op["requestBody"] == nil {
			t.Errorf("%s has no request body", key)
		}
	}
}

func TestSection24BidirectionalRouteParity(t *testing.T) {
	var rs []route
	for _, r := range analyticshttp.TenantRoutes() {
		rs = append(rs, route{r.Method, r.Path, r.OperationID, r.Permission, r.FeatureFlag, r.AuthClass, r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	check(t, "section24-analytics-reporting-automation.openapi.json", rs)
}

func TestSection28And56GovernanceRouteParity(t *testing.T) {
	var rs []route
	for _, r := range governancehttp.TenantRoutes() {
		rs = append(rs, route{r.Method, r.Path, r.OperationID, r.Permission, r.FeatureFlag, r.AuthClass, r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	check(t, "section28-56-governance.openapi.json", rs)
}

// The automation action catalogue contains no protected action (spec 24,
// 54.2): nothing that moves money, disciplines or suspends a provider,
// changes a live assignment, or lifts a safety/compliance restriction.
func TestAutomationCatalogueExcludesProtectedActions(t *testing.T) {
	forbidden := []string{"WALLET", "ALLOCAT", "PAYROLL", "WITHDRAW", "DEDUCT", "SUSPEND", "DISCIPLIN", "ASSIGN", "ACCEPT", "DECLINE", "RESTRICTION", "RETURN_TO_SERVICE", "LIFT", "DELETE", "ERASE", "SCRIPT", "CODE", "HTTP", "WEBHOOK"}
	for action := range analyticsmodels.AutomationActions {
		for _, f := range forbidden {
			if strings.Contains(action, f) {
				t.Errorf("automation action %s looks protected (%s)", action, f)
			}
		}
	}
	for _, r := range analyticshttp.TenantRoutes() {
		if strings.Contains(r.Path, "execute") || strings.Contains(r.Path, "/run-action") {
			t.Errorf("analytics exposes a direct execution route: %s", r.Path)
		}
	}
}
