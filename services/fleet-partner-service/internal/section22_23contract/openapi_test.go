package section22_23contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
)

type route struct {
	method, path, operation, permission, flag, auth string
	idem, version                                   bool
}

func check(t *testing.T, name string, routes []route) {
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
	ops := map[string]map[string]any{}
	for p, item := range doc["paths"].(map[string]any) {
		for m, v := range item.(map[string]any) {
			if m == "get" || m == "post" || m == "put" || m == "patch" || m == "delete" {
				ops[strings.ToUpper(m)+" "+p] = v.(map[string]any)
			}
		}
	}
	if len(ops) != len(routes) {
		t.Fatalf("%s operations=%d routes=%d", name, len(ops), len(routes))
	}
	for _, r := range routes {
		key := r.method + " /fleet-partner/v1" + braces(r.path)
		op, ok := ops[key]
		if !ok {
			t.Errorf("mounted route missing: %s", key)
			continue
		}
		if op["operationId"] != r.operation || op["x-permission"] != r.permission || op["x-feature-flag"] != r.flag || op["x-auth-class"] != r.auth || op["x-idempotency-required"] != r.idem || op["x-expected-version-required"] != r.version {
			t.Errorf("metadata mismatch: %s", key)
		}
		if r.method != "GET" && op["requestBody"] == nil {
			t.Errorf("missing body schema: %s", key)
		}
		if op["responses"] == nil {
			t.Errorf("missing responses: %s", key)
		}
		delete(ops, key)
	}
	for key := range ops {
		t.Errorf("fictional OpenAPI operation: %s", key)
	}
	components := doc["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	for _, n := range []string{"CommandRequest", "Envelope", "Problem"} {
		if schemas[n] == nil {
			t.Errorf("missing schema %s", n)
		}
	}
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
func TestSection22BidirectionalRouteParity(t *testing.T) {
	var rs []route
	for _, r := range financeops.TenantRoutes() {
		rs = append(rs, route{r.Method, r.Path, r.OperationID, r.Permission, r.FeatureFlag, r.AuthClass, r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	check(t, "section22-finance-billing-payroll.openapi.json", rs)
}
func TestSection23BidirectionalRouteParity(t *testing.T) {
	var rs []route
	for _, r := range engagement.TenantRoutes() {
		rs = append(rs, route{r.Method, r.Path, r.OperationID, r.Permission, r.FeatureFlag, r.AuthClass, r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	check(t, "section23-communications-support-sla.openapi.json", rs)
}
func TestContractsPreserveAuthorityBoundaries(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	for _, name := range []string{"section22-finance-billing-payroll.openapi.json", "section23-communications-support-sla.openapi.json"} {
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "docs", "openapi", name))
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(raw))
		for _, bad := range []string{"fleet ledger", "bank account number", "independent email delivery", "automatic decline deduction", "automatic safety deduction"} {
			if strings.Contains(lower, bad) {
				t.Errorf("%s claims forbidden authority: %s", name, bad)
			}
		}
	}
}
