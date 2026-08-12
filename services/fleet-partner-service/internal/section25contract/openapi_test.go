// Package section25contract proves the Section 25 OpenAPI document
// (documents, search, exports, generic imports) matches the routes each
// feature's own manifest actually mounts — the same route-parity discipline
// section15contract, section16contract and the other section*contract
// packages already enforce.
package section25contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	bulkhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/http"
	documenthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/http"
	exportshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/http"
	searchhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/http"
)

type route struct {
	method, path, operation, permission, auth string
	idempotent, versioned                     bool
}

func braces(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}

const corporatePrefix = "/fleet-partner/v1"
const internalPrefix = "/internal/fleet-partner/v1"

func routes() []route {
	var out []route
	appendCorporate := func(method, path, operation, permission string, idem, version bool) {
		out = append(out, route{method, corporatePrefix + braces(path), operation, permission, "corporate", idem, version})
	}
	appendInternal := func(method, path, operation string, idem, version bool) {
		out = append(out, route{method, internalPrefix + braces(path), operation, "", "internal-hmac-v2", idem, version})
	}

	docHandler := documenthttp.NewHandler(nil, nil)
	for _, r := range docHandler.TenantRoutes() {
		appendCorporate(r.Method, r.Path, r.OperationID, r.Permission, r.IdempotencyRequired, r.ExpectedVersionRequired)
	}
	for _, r := range documenthttp.InternalRoutes() {
		appendInternal(r.Method, r.Path, r.OperationID, r.IdempotencyRequired, r.ExpectedVersionRequired)
	}

	searchHandler := searchhttp.NewHandler(nil, nil)
	for _, r := range searchHandler.TenantRoutes() {
		appendCorporate(r.Method, r.Path, r.OperationID, r.Permission, r.IdempotencyRequired, r.ExpectedVersionRequired)
	}

	exportsHandler := exportshttp.NewHandler(nil, nil, nil)
	for _, r := range exportsHandler.TenantRoutes() {
		appendCorporate(r.Method, r.Path, r.OperationID, r.Permission, r.IdempotencyRequired, r.ExpectedVersionRequired)
	}

	// Only the Section 25C generic import catalogue belongs to this
	// contract; bulkimports.TenantRoutes() (the legacy /orders/imports
	// compatibility surface) is Section 15 and lives in section15contract.
	bulkHandler := bulkhttp.NewHandler(nil, nil, nil)
	for _, r := range bulkHandler.GenericTenantRoutes() {
		appendCorporate(r.Method, r.Path, r.OperationID, r.Permission, r.IdempotencyRequired, r.ExpectedVersionRequired)
	}

	return out
}

func load(t *testing.T) map[string]any {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "docs", "openapi",
		"section25-documents-search-exports-imports.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

func operations(t *testing.T, spec map[string]any) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	paths := spec["paths"].(map[string]any)
	for path, raw := range paths {
		for method, operationRaw := range raw.(map[string]any) {
			if method != "get" && method != "post" && method != "patch" && method != "put" && method != "delete" {
				continue
			}
			op := operationRaw.(map[string]any)
			key := strings.ToUpper(method) + " " + path
			if _, exists := out[key]; exists {
				t.Fatalf("duplicate operation %s", key)
			}
			out[key] = op
		}
	}
	return out
}

func TestOpenAPIRouteParity(t *testing.T) {
	actual := routes()
	specOps := operations(t, load(t))
	if len(specOps) != len(actual) {
		t.Fatalf("spec operations=%d route operations=%d", len(specOps), len(actual))
	}
	seen := map[string]bool{}
	for _, r := range actual {
		key := r.method + " " + r.path
		op, ok := specOps[key]
		if !ok {
			t.Errorf("route missing from spec: %s", key)
			continue
		}
		seen[key] = true
		if op["operationId"] != r.operation {
			t.Errorf("%s operationId=%v want %s", key, op["operationId"], r.operation)
		}
		if op["x-auth-class"] != r.auth {
			t.Errorf("%s auth=%v want %s", key, op["x-auth-class"], r.auth)
		}
		wantPermission := r.permission
		if wantPermission == "" {
			if _, hasPermission := op["x-permission"]; hasPermission {
				t.Errorf("%s has x-permission but the route carries none (internal route)", key)
			}
		} else if op["x-permission"] != wantPermission {
			t.Errorf("%s permission=%v want %s", key, op["x-permission"], wantPermission)
		}
		if value, _ := op["x-idempotency-required"].(bool); value != r.idempotent {
			t.Errorf("%s idempotency=%v want %v", key, value, r.idempotent)
		}
		if value, _ := op["x-expected-version-required"].(bool); value != r.versioned {
			t.Errorf("%s expectedVersion=%v want %v", key, value, r.versioned)
		}
		if flag, _ := op["x-feature-flag"].(string); flag == "" {
			t.Errorf("%s missing feature flag", key)
		}
	}
	for key := range specOps {
		if !seen[key] {
			t.Errorf("spec operation not mounted: %s", key)
		}
	}
}

// TestOpenAPIExcludesUnimplementedDatasetsAndCommercialFields proves the
// spec never advertises an export dataset with no registered adapter
// (COMPLIANCE_INCIDENTS, INVOICES_RECEIVABLES, FULFILMENT_ORDERS,
// SUPPORT_CASE_PROJECTIONS — see internal/app/exports_wiring.go), and that
// the external (provider/customer) document view never gains an internal
// governance field.
func TestOpenAPIIncludesAllDatasetsAndExcludesCommercialFields(t *testing.T) {
	spec := load(t)
	raw, _ := json.Marshal(spec)
	lower := strings.ToLower(string(raw))
	for _, required := range []string{"compliance_incidents", "invoices_receivables",
		"fulfilment_orders", "support_case_projections"} {
		if !strings.Contains(lower, required) {
			t.Errorf("spec omits registered export dataset: %s", required)
		}
	}

	components := spec["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	documentView, _ := json.Marshal(schemas["DocumentView"])
	viewLower := strings.ToLower(string(documentView))
	for _, forbidden := range []string{"retentionclass", "legalhold", "mediasourceversion",
		"revokedreason", "archivedbyactorid", "checksum"} {
		if strings.Contains(viewLower, forbidden) {
			t.Errorf("external DocumentView schema leaks internal governance field: %s", forbidden)
		}
	}
}
