// Package section27contract proves the Section 27 OpenAPI document
// (background jobs and scheduled tasks) matches the routes
// backgroundops/http's own manifest actually mounts -- the same
// route-parity discipline section25contract and section26contract already
// enforce.
package section27contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	backgroundopshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/backgroundops/http"
)

func braces(path string) string {
	path = strings.ReplaceAll(path, ":jobId", "{jobId}")
	path = strings.ReplaceAll(path, ":taskId", "{taskId}")
	return path
}

func TestBackgroundOpsOpenAPIRouteParity(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "docs", "openapi", "section27-background-jobs-scheduling.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	paths := spec["paths"].(map[string]any)
	seen := map[string]bool{}
	actual := map[string]bool{}

	handler := backgroundopshttp.NewHandler(nil, nil, nil)
	for _, route := range handler.TenantRoutes() {
		path := "/fleet-partner/v1" + braces(route.Path)
		actual[strings.ToUpper(route.Method)+" "+path] = true
		item, ok := paths[path].(map[string]any)
		if !ok {
			t.Errorf("missing %s", path)
			continue
		}
		op, ok := item[strings.ToLower(route.Method)].(map[string]any)
		if !ok {
			t.Errorf("missing %s %s", route.Method, path)
			continue
		}
		seen[route.OperationID] = true
		if op["operationId"] != route.OperationID {
			t.Errorf("operation mismatch for %s", path)
		}
		if op["x-permission"] != route.Permission {
			t.Errorf("permission mismatch for %s", path)
		}
	}

	for _, route := range backgroundopshttp.InternalRoutes() {
		path := "/internal/fleet-partner/v1" + braces(route.Path)
		actual[strings.ToUpper(route.Method)+" "+path] = true
		item, ok := paths[path].(map[string]any)
		if !ok {
			t.Errorf("missing %s", path)
			continue
		}
		op, ok := item[strings.ToLower(route.Method)].(map[string]any)
		if !ok {
			t.Errorf("missing %s %s", route.Method, path)
			continue
		}
		seen[route.OperationID] = true
		if op["operationId"] != route.OperationID {
			t.Errorf("operation mismatch for %s", path)
		}
		if op["x-auth-class"] != "internal-hmac-v2" {
			t.Errorf("%s missing x-auth-class internal-hmac-v2", path)
		}
		if _, hasPermission := op["x-permission"]; hasPermission {
			t.Errorf("%s has x-permission but internal routes carry none", path)
		}
	}

	wantTotal := len(handler.TenantRoutes()) + len(backgroundopshttp.InternalRoutes())
	if len(seen) != wantTotal {
		t.Fatalf("documented %d of %d background-ops routes", len(seen), wantTotal)
	}

	// Every documented path/method must correspond to an actual mounted
	// route -- catches a spec entry for a route that was renamed or removed.
	for path, rawItem := range paths {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		for method := range item {
			if method != "get" && method != "post" && method != "patch" && method != "put" && method != "delete" {
				continue
			}
			key := strings.ToUpper(method) + " " + path
			if !actual[key] {
				t.Errorf("spec operation not mounted: %s", key)
			}
		}
	}
}
