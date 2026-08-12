package section17contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce"
)

func TestRouteOpenAPIParity(t *testing.T) {
	_, f, _, _ := runtime.Caller(0)
	raw, e := os.ReadFile(filepath.Join(filepath.Dir(f), "..", "..", "docs", "openapi", "section17-workforce-training.openapi.json"))
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
	routes := workforce.TenantRoutes()
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
