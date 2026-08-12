package section26contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/synchronization"
)

func TestSynchronizationOpenAPIRouteParity(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "docs", "openapi", "section26-synchronization.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	paths := spec["paths"].(map[string]any)
	seen := map[string]bool{}
	for _, route := range synchronization.TenantRoutes() {
		path := strings.ReplaceAll(strings.ReplaceAll(route.Path, ":caseId", "{caseId}"), ":deadLetterId", "{deadLetterId}")
		path = "/fleet-partner/v1" + path
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
	if len(seen) != len(synchronization.TenantRoutes()) {
		t.Fatalf("documented %d of %d synchronization routes", len(seen), len(synchronization.TenantRoutes()))
	}
}
