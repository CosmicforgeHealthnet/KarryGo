// Package section28contract proves the Section 28 OpenAPI document (audit
// log reads and generic legal holds) matches the routes audit/http and
// legalhold/http's own manifests actually mount -- the same route-parity
// discipline every prior sectionNNcontract package enforces.
package section28contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	audithttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/audit/http"
	legalholdhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/http"
	retentionpolicyhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/http"
	securitysettingshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/http"
)

func braces(path string) string {
	path = strings.ReplaceAll(path, ":auditId", "{auditId}")
	path = strings.ReplaceAll(path, ":holdId", "{holdId}")
	path = strings.ReplaceAll(path, ":class", "{class}")
	return path
}

func TestSection28OpenAPIRouteParity(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "docs", "openapi", "section28-security-privacy-audit.openapi.json"))
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

	type route struct {
		Method, Path, OperationID, Permission string
	}
	var all []route

	auditHandler := audithttp.NewHandler(nil, nil)
	for _, r := range auditHandler.TenantRoutes() {
		all = append(all, route{r.Method, r.Path, r.OperationID, r.Permission})
	}
	legalHoldHandler := legalholdhttp.NewHandler(nil, nil)
	for _, r := range legalHoldHandler.TenantRoutes() {
		all = append(all, route{r.Method, r.Path, r.OperationID, r.Permission})
	}
	securitySettingsHandler := securitysettingshttp.NewHandler(nil, nil)
	for _, r := range securitySettingsHandler.TenantRoutes() {
		all = append(all, route{r.Method, r.Path, r.OperationID, r.Permission})
	}
	retentionPolicyHandler := retentionpolicyhttp.NewHandler(nil, nil)
	for _, r := range retentionPolicyHandler.TenantRoutes() {
		all = append(all, route{r.Method, r.Path, r.OperationID, r.Permission})
	}

	for _, r := range all {
		path := "/fleet-partner/v1" + braces(r.Path)
		actual[strings.ToUpper(r.Method)+" "+path] = true
		item, ok := paths[path].(map[string]any)
		if !ok {
			t.Errorf("missing %s", path)
			continue
		}
		op, ok := item[strings.ToLower(r.Method)].(map[string]any)
		if !ok {
			t.Errorf("missing %s %s", r.Method, path)
			continue
		}
		seen[r.OperationID] = true
		if op["operationId"] != r.OperationID {
			t.Errorf("operation mismatch for %s", path)
		}
		if op["x-permission"] != r.Permission {
			t.Errorf("permission mismatch for %s: doc=%v manifest=%v", path, op["x-permission"], r.Permission)
		}
	}

	if len(seen) != len(all) {
		t.Fatalf("documented %d of %d Section 28 routes", len(seen), len(all))
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
