// Package section29contract proves the Section 29 OpenAPI document (liveness,
// readiness, protected metrics, and the two new platform-scoped operational
// routes) matches what is actually mounted — the same route-parity
// discipline every prior sectionNNcontract package enforces (see
// section28contract/openapi_test.go for the established pattern this
// mirrors).
//
// Scope: this package proves parity for the routes THIS section adds or
// documents. It deliberately does not re-implement section28contract's own
// full "every mounted route" enumeration (route_coverage_test.go) — that
// remains the single source of truth for the whole service's route
// inventory; duplicating it here would just be a second, divergence-prone
// copy.
package section29contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	backgroundopshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/backgroundops/http"
)

func loadSpec(t *testing.T) map[string]any {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "docs", "openapi", "section29-reliability-observability.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

// TestSection29OpenAPIRouteParity_OperationalRoutes proves the two new
// internal operational routes (backgroundopshttp.InternalRoutes) this
// session added are documented, and documented under the correct method,
// path and operationId — a route renamed or removed from the manifest
// without a matching spec update fails this test.
func TestSection29OpenAPIRouteParity_OperationalRoutes(t *testing.T) {
	spec := loadSpec(t)
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("spec has no paths object")
	}

	wantOperationIDs := map[string]bool{"getPlatformBacklogs": true, "replayOutboxRecord": true}
	found := map[string]bool{}

	for _, r := range backgroundopshttp.InternalRoutes() {
		if !wantOperationIDs[r.OperationID] {
			continue // every other worker-operations route is Section 27's own, documented in its own spec
		}
		fullPath := "/internal/fleet-partner/v1" + r.Path
		item, ok := paths[fullPath].(map[string]any)
		if !ok {
			t.Errorf("spec is missing path %s (operation %s)", fullPath, r.OperationID)
			continue
		}
		op, ok := item[strings.ToLower(r.Method)].(map[string]any)
		if !ok {
			t.Errorf("spec path %s is missing method %s", fullPath, r.Method)
			continue
		}
		if op["operationId"] != r.OperationID {
			t.Errorf("operationId mismatch for %s %s: spec=%v manifest=%s", r.Method, fullPath, op["operationId"], r.OperationID)
		}
		found[r.OperationID] = true
	}

	for opID := range wantOperationIDs {
		if !found[opID] {
			t.Errorf("operation %s is documented but no longer mounted in backgroundopshttp.InternalRoutes()", opID)
		}
	}
}

// TestSection29OpenAPIRouteParity_LivenessReadinessMetrics proves the three
// always-on operational routes this section names explicitly are present in
// the spec with the safe (no-auth for liveness/readiness, token-protected
// for metrics) security posture documented — these are not manifest-driven
// (they are constructed directly in internal/app/app.go, not through a
// feature's Route slice), so this test names them directly rather than
// enumerating a manifest that does not exist for them.
func TestSection29OpenAPIRouteParity_LivenessReadinessMetrics(t *testing.T) {
	spec := loadSpec(t)
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("spec has no paths object")
	}

	for path, method := range map[string]string{
		"/live":                     "get",
		"/health":                   "get",
		"/ready":                    "get",
		"/fleet-partner/v1/metrics": "get",
	} {
		item, ok := paths[path].(map[string]any)
		if !ok {
			t.Errorf("spec is missing path %s", path)
			continue
		}
		if _, ok := item[method].(map[string]any); !ok {
			t.Errorf("spec path %s is missing method %s", path, method)
		}
	}

	// Liveness/readiness must declare an empty security requirement (never
	// gated behind auth — see docs/section29-reliability-observability.md's
	// Liveness section for why: an orchestrator's health probe cannot be
	// expected to hold a bearer token).
	for _, path := range []string{"/live", "/health", "/ready"} {
		item := paths[path].(map[string]any)
		get := item["get"].(map[string]any)
		security, ok := get["security"].([]any)
		if !ok || len(security) != 0 {
			t.Errorf("%s must declare an empty security requirement, got %v", path, get["security"])
		}
	}

	// Metrics must NOT declare an empty security requirement — it is
	// protected, never public.
	metricsItem := paths["/fleet-partner/v1/metrics"].(map[string]any)
	metricsGet := metricsItem["get"].(map[string]any)
	if security, ok := metricsGet["security"].([]any); !ok || len(security) == 0 {
		t.Error("/fleet-partner/v1/metrics must declare a non-empty security requirement — it is never public")
	}
}
