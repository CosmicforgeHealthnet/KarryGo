package crmcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type section43Declaration struct {
	Route  string
	Source string
}

type section43Artifact struct {
	Section                 string              `json:"section"`
	ProductionRouteCount    int                 `json:"productionRouteCount"`
	OpenAPIOperationCount   int                 `json:"openapiOperationCount"`
	UniqueOpenAPIRouteCount int                 `json:"uniqueOpenapiRouteCount"`
	RegisteredRoutes        []string            `json:"registeredRoutes"`
	OpenAPIRoutes           []string            `json:"openapiRoutes"`
	MissingFromOpenAPI      []string            `json:"missingFromOpenapi"`
	MissingFromRouter       []string            `json:"missingFromRouter"`
	Exclusions              []string            `json:"exclusions"`
	FamilyCounts            map[string]int      `json:"familyCounts"`
	DuplicateDeclarations   map[string][]string `json:"approvedDuplicateDeclarations"`
}

var approvedSection43Duplicates = map[string][]string{
	"GET /api/v1/commerce/checkout/{sessionId}/fulfilment-plan": {
		"commerce-fulfilment-optimization.openapi.json", "section17c-fulfilment-planning.openapi.json",
	},
	"POST /api/v1/commerce/checkout/{sessionId}/fulfilment-plan": {
		"commerce-fulfilment-optimization.openapi.json", "section17c-fulfilment-planning.openapi.json",
	},
	"GET /api/v1/commerce/substitution-cases": {
		"section18-vendor-order-lifecycle.openapi.json", "shop-picking-substitution-variable-measurement.openapi.json",
	},
	"GET /api/v1/commerce/substitution-cases/{caseId}": {
		"section18-vendor-order-lifecycle.openapi.json", "shop-picking-substitution-variable-measurement.openapi.json",
	},
	"POST /api/v1/commerce/substitution-cases/{caseId}/accept": {
		"section18-vendor-order-lifecycle.openapi.json", "shop-picking-substitution-variable-measurement.openapi.json",
	},
	"POST /api/v1/commerce/substitution-cases/{caseId}/reject": {
		"section18-vendor-order-lifecycle.openapi.json", "shop-picking-substitution-variable-measurement.openapi.json",
	},
	"POST /api/v1/vendor/commerce/orders/{vendorOrderId}/lines/{lineId}/actual-quantity": {
		"section18-vendor-order-lifecycle.openapi.json", "shop-picking-substitution-variable-measurement.openapi.json",
	},
	"POST /api/v1/vendor/commerce/orders/{vendorOrderId}/lines/{lineId}/quality": {
		"section18-vendor-order-lifecycle.openapi.json", "shop-picking-substitution-variable-measurement.openapi.json",
	},
	"GET /api/v1/vendor/commerce/orders/{vendorOrderId}/picking": {
		"section18-vendor-order-lifecycle.openapi.json", "shop-picking-substitution-variable-measurement.openapi.json",
	},
	"GET /api/v1/vendor/commerce/orders/{vendorOrderId}/picking-queue": {
		"section18-vendor-order-lifecycle.openapi.json", "shop-picking-substitution-variable-measurement.openapi.json",
	},
}

func section43Declarations(t *testing.T) []section43Declaration {
	t.Helper()
	var declarations []section43Declaration
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(specDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var document specDocument
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		base := ""
		if len(document.Servers) > 0 {
			base = strings.TrimRight(document.Servers[0].URL, "/")
		}
		for routePath, item := range document.Paths {
			for method := range item {
				if !isMethod(method) {
					continue
				}
				full := routePath
				if !strings.HasPrefix(routePath, "/internal/") {
					full = base + routePath
				}
				declarations = append(declarations, section43Declaration{
					Route: routeKey(strings.ToUpper(method), full), Source: entry.Name(),
				})
			}
		}
	}
	sort.Slice(declarations, func(i, j int) bool {
		if declarations[i].Route == declarations[j].Route {
			return declarations[i].Source < declarations[j].Source
		}
		return declarations[i].Route < declarations[j].Route
	})
	return declarations
}

func TestSection43ApprovedOpenAPIDuplicatesAreExplicit(t *testing.T) {
	actual := map[string][]string{}
	for _, declaration := range section43Declarations(t) {
		actual[declaration.Route] = append(actual[declaration.Route], declaration.Source)
	}
	for route, sources := range actual {
		if len(sources) == 1 {
			delete(actual, route)
			continue
		}
		sort.Strings(sources)
	}
	if !reflect.DeepEqual(actual, approvedSection43Duplicates) {
		actualJSON, _ := json.MarshalIndent(actual, "", "  ")
		t.Fatalf("OpenAPI duplicate declarations changed without an explicit compatibility decision:\n%s", actualJSON)
	}
}

func TestSection43EveryProductionRouteHasAKnownFamily(t *testing.T) {
	for route := range productionRoutes(t) {
		if family := section43Family(route); family == "" {
			t.Errorf("unclassified production route: %s", route)
		}
	}
}

func section43Family(route string) string {
	_, path, ok := strings.Cut(route, " ")
	if !ok {
		return ""
	}
	if isPlatformRoute(path) {
		return "Approved platform probes"
	}
	if strings.HasPrefix(path, "/internal/") || strings.HasPrefix(path, "/api/v1/internal/") {
		return "Internal Integrations"
	}
	if strings.HasPrefix(path, "/api/v1/vendor/auth") {
		return "Vendor Authentication"
	}
	if strings.HasPrefix(path, "/api/v1/vendor/crm") {
		return "Vendor CRM"
	}
	if strings.Contains(path, "/purchasing/") || strings.Contains(path, "/suppliers") || strings.Contains(path, "/goods-receipts") {
		return "Vendor Purchasing"
	}
	if strings.Contains(path, "/inventory") || strings.Contains(path, "/stocktake") || strings.Contains(path, "/stock-") || strings.Contains(path, "/lots") || strings.Contains(path, "/reorder") {
		return "Vendor Inventory"
	}
	if strings.HasPrefix(path, "/api/v1/vendor/commerce/orders") {
		return "Vendor Orders"
	}
	if strings.Contains(path, "/delivery") || strings.Contains(path, "/fulfilment") || strings.Contains(path, "/missions") {
		return "Vendor Delivery"
	}
	if strings.HasPrefix(path, "/api/v1/commerce") {
		if strings.Contains(path, "/section21/") || strings.Contains(path, "/optimization") || strings.Contains(path, "/proposals") || strings.Contains(path, "/recurring") || strings.Contains(path, "/rfq") {
			return "Advanced Commerce"
		}
		if strings.Contains(path, "/food") || strings.Contains(path, "/menus") {
			return "Customer Food"
		}
		if strings.Contains(path, "/shop") || strings.Contains(path, "/products") {
			return "Customer Shop"
		}
		return "Commerce Cart Checkout Order"
	}
	if strings.HasPrefix(path, "/api/v1/vendor") {
		if strings.Contains(path, "/food") || strings.Contains(path, "/shop") || strings.Contains(path, "/catalog") || strings.Contains(path, "/menu") || strings.Contains(path, "/products") || strings.Contains(path, "/categories") || strings.Contains(path, "/pastry") {
			return "Vendor Catalogue"
		}
		return "Vendor Business"
	}
	return ""
}

func TestSection43ParityArtifactIsCurrent(t *testing.T) {
	routes := productionRoutes(t)
	declarations := section43Declarations(t)
	documented := map[string]bool{}
	duplicates := map[string][]string{}
	for _, declaration := range declarations {
		documented[declaration.Route] = true
		duplicates[declaration.Route] = append(duplicates[declaration.Route], declaration.Source)
	}
	for route, sources := range duplicates {
		if len(sources) == 1 {
			delete(duplicates, route)
		}
	}
	registeredRoutes := sortedKeys(routes)
	openAPIRoutes := sortedKeys(documented)
	missingFromOpenAPI := difference(routes, documented)
	missingFromRouter := difference(documented, routes)
	families := map[string]int{}
	for _, route := range registeredRoutes {
		families[section43Family(route)]++
	}
	artifact := section43Artifact{
		Section: "43 - API Contract Families", ProductionRouteCount: len(routes),
		OpenAPIOperationCount: len(declarations), UniqueOpenAPIRouteCount: len(documented),
		RegisteredRoutes: registeredRoutes, OpenAPIRoutes: openAPIRoutes,
		MissingFromOpenAPI: missingFromOpenAPI, MissingFromRouter: missingFromRouter,
		Exclusions:   []string{"GET /health", "GET /ready", "GET /api/v1/vendor/meta"},
		FamilyCounts: families, DuplicateDeclarations: duplicates,
	}
	encoded, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	path := "../../docs/SECTION43_ROUTE_PARITY.json"
	if os.Getenv("UPDATE_SECTION43_ARTIFACT") == "1" {
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read parity artifact (run with UPDATE_SECTION43_ARTIFACT=1 to create it): %v", err)
	}
	if !bytes.Equal(want, encoded) {
		t.Fatal("SECTION43_ROUTE_PARITY.json is stale; run this test with UPDATE_SECTION43_ARTIFACT=1")
	}
	if len(missingFromOpenAPI) != 0 || len(missingFromRouter) != 0 {
		t.Fatalf("parity is not clean: missingFromOpenAPI=%v missingFromRouter=%v", missingFromOpenAPI, missingFromRouter)
	}
}

func sortedKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func difference(left, right map[string]bool) []string {
	var out []string
	for value := range left {
		if !right[value] {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func TestSection43CriticalWriteContractsDeclareRetryAndConcurrency(t *testing.T) {
	critical := map[string][]string{
		"POST /api/v1/commerce/checkout/{sessionId}/quotes":                                                {"Idempotency-Key"},
		"POST /api/v1/commerce/quotes/{quoteId}/orders":                                                    {"Idempotency-Key"},
		"POST /api/v1/vendor/commerce/orders/{vendorOrderId}/accept":                                       {"Idempotency-Key", "If-Match"},
		"POST /api/v1/vendor/commerce/orders/{vendorOrderId}/reject":                                       {"Idempotency-Key", "If-Match"},
		"POST /api/v1/commerce/substitution-cases/{caseId}/accept":                                         {"Idempotency-Key", "If-Match"},
		"POST /api/v1/commerce/substitution-cases/{caseId}/reject":                                         {"Idempotency-Key", "If-Match"},
		"POST /api/v1/vendor/commerce/orders/{vendorOrderId}/missions/{missionId}/handover/pickup/confirm": {"Idempotency-Key"},
	}
	found := map[string]map[string]bool{}
	for _, declaration := range section43Declarations(t) {
		if _, ok := critical[declaration.Route]; ok {
			found[declaration.Route] = openAPIParameterNames(t, declaration.Source, declaration.Route)
		}
	}
	for route, required := range critical {
		for _, parameter := range required {
			if !found[route][parameter] {
				t.Errorf("%s does not declare required %s", route, parameter)
			}
		}
	}
}

func TestSection43AllOpenAPIDocumentsAreStructurallyValid(t *testing.T) {
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatal(err)
	}
	operationIDs := map[string]string{}
	operations := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		document := loadOpenAPIMap(t, entry.Name())
		version, _ := document["openapi"].(string)
		if !strings.HasPrefix(version, "3.") {
			t.Errorf("%s has unsupported OpenAPI version %q", entry.Name(), version)
		}
		validateSection43Refs(t, document, document, entry.Name())
		paths, _ := document["paths"].(map[string]any)
		for routePath, rawItem := range paths {
			item, _ := rawItem.(map[string]any)
			for _, method := range []string{"get", "post", "put", "patch", "delete"} {
				operation, ok := item[method].(map[string]any)
				if !ok {
					continue
				}
				operations++
				id, _ := operation["operationId"].(string)
				if strings.TrimSpace(id) == "" {
					t.Errorf("%s %s %s has no operationId", entry.Name(), method, routePath)
				} else if prior, duplicate := operationIDs[id]; duplicate {
					t.Errorf("duplicate operationId %q: %s and %s %s %s", id, prior, entry.Name(), method, routePath)
				} else {
					operationIDs[id] = entry.Name() + " " + method + " " + routePath
				}
				resolved := resolveSection43Ref(t, document, operation, entry.Name())
				if responses, ok := resolved["responses"].(map[string]any); !ok || len(responses) == 0 {
					t.Errorf("%s %s %s has no responses", entry.Name(), method, routePath)
				}
				for _, parameter := range pathTemplateNames(routePath) {
					if !section43HasPathParameter(t, document, item, operation, resolved, parameter, entry.Name()) {
						t.Errorf("%s %s %s lacks required path parameter %q", entry.Name(), method, routePath, parameter)
					}
				}
				validateSection43Security(t, document, operation, resolved, entry.Name()+" "+method+" "+routePath)
			}
		}
	}
	if operations != 877 || len(operationIDs) != operations {
		t.Fatalf("OpenAPI inventory changed: operations=%d uniqueOperationIds=%d", operations, len(operationIDs))
	}
}

func loadOpenAPIMap(t *testing.T, source string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(specDir, source))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("%s: %v", source, err)
	}
	return document
}

func resolveSection43Ref(t *testing.T, document, object map[string]any, source string) map[string]any {
	t.Helper()
	ref, _ := object["$ref"].(string)
	if !strings.HasPrefix(ref, "#/") {
		return object
	}
	var current any = document
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		container, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("%s has invalid local reference %q", source, ref)
		}
		current, ok = container[part]
		if !ok {
			t.Fatalf("%s has unresolved local reference %q", source, ref)
		}
	}
	resolved, ok := current.(map[string]any)
	if !ok {
		t.Fatalf("%s local reference %q is not an object", source, ref)
	}
	return resolved
}

func validateSection43Refs(t *testing.T, document map[string]any, value any, source string) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		if ref, ok := typed["$ref"].(string); ok && strings.HasPrefix(ref, "#/") {
			_ = resolveSection43Ref(t, document, map[string]any{"$ref": ref}, source)
		}
		for _, child := range typed {
			validateSection43Refs(t, document, child, source)
		}
	case []any:
		for _, child := range typed {
			validateSection43Refs(t, document, child, source)
		}
	}
}

func pathTemplateNames(routePath string) []string {
	var names []string
	for {
		start := strings.IndexByte(routePath, '{')
		if start < 0 {
			return names
		}
		routePath = routePath[start+1:]
		end := strings.IndexByte(routePath, '}')
		if end < 0 {
			return names
		}
		names = append(names, routePath[:end])
		routePath = routePath[end+1:]
	}
}

func section43HasPathParameter(t *testing.T, document, item, operation, resolved map[string]any, name, source string) bool {
	t.Helper()
	for _, owner := range []map[string]any{item, operation, resolved} {
		parameters, _ := owner["parameters"].([]any)
		for _, rawParameter := range parameters {
			parameter, _ := rawParameter.(map[string]any)
			parameter = resolveSection43Ref(t, document, parameter, source)
			if parameter["name"] == name && parameter["in"] == "path" && parameter["required"] == true {
				return true
			}
		}
	}
	return false
}

func validateSection43Security(t *testing.T, document, operation, resolved map[string]any, location string) {
	t.Helper()
	components, _ := document["components"].(map[string]any)
	schemes, _ := components["securitySchemes"].(map[string]any)
	raw := document["security"]
	if operation["security"] != nil {
		raw = operation["security"]
	} else if resolved["security"] != nil {
		raw = resolved["security"]
	}
	requirements, _ := raw.([]any)
	for _, rawRequirement := range requirements {
		requirement, _ := rawRequirement.(map[string]any)
		for scheme := range requirement {
			if _, ok := schemes[scheme]; !ok {
				t.Errorf("%s references undefined security scheme %q", location, scheme)
			}
		}
	}
}

func openAPIParameterNames(t *testing.T, source, route string) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(specDir, source))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	method, fullPath, _ := strings.Cut(route, " ")
	base := ""
	if servers, _ := document["servers"].([]any); len(servers) != 0 {
		server, _ := servers[0].(map[string]any)
		base, _ = server["url"].(string)
		base = strings.TrimRight(base, "/")
	}
	paths, _ := document["paths"].(map[string]any)
	for routePath, rawItem := range paths {
		candidate := routePath
		if !strings.HasPrefix(routePath, "/internal/") {
			candidate = base + routePath
		}
		if candidate != fullPath {
			continue
		}
		item, _ := rawItem.(map[string]any)
		operation, _ := item[strings.ToLower(method)].(map[string]any)
		out := map[string]bool{}
		for _, owner := range []map[string]any{item, operation} {
			parameters, _ := owner["parameters"].([]any)
			for _, rawParameter := range parameters {
				parameter, _ := rawParameter.(map[string]any)
				if ref, ok := parameter["$ref"].(string); ok {
					parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
					var current any = document
					for _, part := range parts {
						current = current.(map[string]any)[part]
					}
					parameter, _ = current.(map[string]any)
				}
				if name, ok := parameter["name"].(string); ok {
					out[name] = true
				}
			}
		}
		return out
	}
	t.Fatalf("%s was not found in %s", route, source)
	return nil
}

func TestSection43ArtifactSummary(t *testing.T) {
	// This small diagnostic makes the certification counts visible in ordinary
	// test output without parsing the checked-in artifact.
	declarations := section43Declarations(t)
	unique := map[string]bool{}
	for _, declaration := range declarations {
		unique[declaration.Route] = true
	}
	t.Log(fmt.Sprintf("production=%d openapiOperations=%d uniqueOpenAPIRoutes=%d", len(productionRoutes(t)), len(declarations), len(unique)))
}
