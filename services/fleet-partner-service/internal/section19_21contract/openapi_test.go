package section19_21contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/compliance"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/proof"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/specialized"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse"
)

type route struct {
	method, path, operation, permission, flag, auth string
	idem, version                                   bool
}

func load(t *testing.T, name string) (map[string]any, []byte) {
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
	return doc, raw
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
func operations(t *testing.T, doc map[string]any) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for path, raw := range doc["paths"].(map[string]any) {
		for method, value := range raw.(map[string]any) {
			switch method {
			case "get", "post", "put", "patch", "delete":
				out[strings.ToUpper(method)+" "+path] = value.(map[string]any)
			}
		}
	}
	return out
}
func check(t *testing.T, name string, routes []route) {
	t.Helper()
	doc, _ := load(t, name)
	ops := operations(t, doc)
	if len(ops) != len(routes) {
		t.Fatalf("%s spec operations=%d routes=%d", name, len(ops), len(routes))
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
		delete(ops, key)
	}
	for key := range ops {
		t.Errorf("fictional OpenAPI operation: %s", key)
	}
}

func TestSection19BidirectionalRouteParity(t *testing.T) {
	var rs []route
	for _, r := range compliance.TenantRoutes() {
		rs = append(rs, route{r.Method, r.Path, r.OperationID, r.Permission, r.FeatureFlag, r.AuthClass, r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	for _, r := range safety.OperationsTenantRoutes() {
		rs = append(rs, route{r.Method, r.Path, r.OperationID, r.Permission, r.FeatureFlag, r.AuthClass, r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	for _, r := range proof.TenantRoutes() {
		rs = append(rs, route{r.Method, r.Path, r.OperationID, r.Permission, r.FeatureFlag, r.AuthClass, r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	check(t, "section19-compliance-safety-proof.openapi.json", rs)
}
func TestSection20BidirectionalRouteParity(t *testing.T) {
	var rs []route
	for _, r := range specialized.TenantRoutes() {
		rs = append(rs, route{r.Method, r.Path, r.OperationID, r.Permission, r.FeatureFlag, r.AuthClass, r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	check(t, "section20-specialized-logistics.openapi.json", rs)
}
func TestSection21BidirectionalRouteParity(t *testing.T) {
	var rs []route
	for _, r := range warehouse.TenantRoutes() {
		rs = append(rs, route{r.Method, r.Path, r.OperationID, r.Permission, r.FeatureFlag, r.AuthClass, r.IdempotencyRequired, r.ExpectedVersionRequired})
	}
	check(t, "section21-warehouse-fulfilment.openapi.json", rs)
}
func TestContractsExcludeFinancialMutationAndTaxiExecution(t *testing.T) {
	for _, name := range []string{"section19-compliance-safety-proof.openapi.json", "section20-specialized-logistics.openapi.json", "section21-warehouse-fulfilment.openapi.json"} {
		_, raw := load(t, name)
		lower := strings.ToLower(string(raw))
		for _, forbidden := range []string{"wallet balance", "ledger entry", "taxi execution", "calculate duty", "calculate tax"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("%s contains forbidden authority: %s", name, forbidden)
			}
		}
	}
}

func TestContractsContainMachineReadableBodiesAndResponses(t *testing.T) {
	for _, name := range []string{"section19-compliance-safety-proof.openapi.json", "section20-specialized-logistics.openapi.json", "section21-warehouse-fulfilment.openapi.json"} {
		doc, _ := load(t, name)
		components, ok := doc["components"].(map[string]any)
		if !ok {
			t.Fatalf("%s has no components", name)
		}
		schemas, ok := components["schemas"].(map[string]any)
		if !ok || schemas["Envelope"] == nil || schemas["Problem"] == nil || schemas["CommandRequest"] == nil {
			t.Fatalf("%s is missing shared request/response schemas", name)
		}
		for key, operation := range operations(t, doc) {
			method := strings.SplitN(key, " ", 2)[0]
			if method != "GET" && operation["requestBody"] == nil {
				t.Errorf("%s %s has no machine-readable request body", name, key)
			}
			responses, ok := operation["responses"].(map[string]any)
			if !ok || len(responses) == 0 {
				t.Errorf("%s %s has no responses", name, key)
				continue
			}
			for status, raw := range responses {
				response, ok := raw.(map[string]any)
				if !ok || response["content"] == nil {
					t.Errorf("%s %s response %s has no JSON schema", name, key, status)
				}
			}
		}
	}
}
