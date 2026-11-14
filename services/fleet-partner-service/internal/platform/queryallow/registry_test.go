package queryallow

import (
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/filters"
)

// webAppParameters is what the Fleet web app's service layer can put on each
// list request (web/fleet/src/lib/services/*.ts, every URLSearchParams.set
// and axios `params` object). Every one of them must stay accepted.
var webAppParameters = map[string][]string{
	"/alerts":         {"branchId", "status", "alertType", "limit", "offset"},
	"/assets":         {"branchId", "status", "limit", "offset"},
	"/control-tower":  {"branchId", "status", "service", "providerId", "vehicleOrTruckReference", "limit", "offset"},
	"/tracking":       {"providerId", "vehicleOrTruckReference", "limit", "offset"},
	"/dispatch/plans": {"branchId", "status", "limit", "offset"},
	"/geofences":      {"branchId", "limit", "offset"},
	"/routes":         {"branchId", "status", "service", "providerId", "vehicleOrTruckReference", "limit", "offset"},

	"/marketplace/offers":      {"status", "branchId", "limit", "cursorSortValue", "cursorLastId"},
	"/marketplace/assignments": {"status", "branchId", "limit", "cursorSortValue", "cursorLastId"},

	"/workforce/profiles":   {"branchId", "status", "limit", "offset"},
	"/shifts":               {"branchId", "status", "limit", "offset"},
	"/shift-templates":      {"branchId", "status", "limit", "offset"},
	"/leave":                {"status", "limit", "offset"},
	"/training/enrollments": {"status", "limit", "offset", "membershipId"},
	"/departments":          {"branchId", "includeArchived"},

	"/finance/operations":           {"status", "operationType", "subjectId", "limit"},
	"/finance/projections":          {"projectionType", "freshness", "subjectId", "limit"},
	"/finance/reconciliation-cases": {"projectionType", "subjectId", "limit"},

	"/customers": {"status", "riskStatus", "customerType", "branchId", "accountOwnerUserId",
		"createdAfter", "createdBefore", "search", "cursor", "limit"},
	"/customers/:customerId/history":                {"cursor", "limit"},
	"/customers/:customerId/contacts":               {"status", "contactPurpose", "cursor", "limit"},
	"/customers/:customerId/locations":              {"locationType", "status", "cursor", "limit"},
	"/customers/:customerId/credit-profile/history": {"cursor", "limit"},
	"/sales/opportunities": {"stage", "customerId", "branchId", "accountOwnerUserId",
		"createdAfter", "createdBefore", "cursor", "limit"},
	"/sales/opportunities/:opportunityId/history": {"cursor", "limit"},
	"/quotations": {"status", "customerId", "opportunityId", "branchId", "createdAfter",
		"createdBefore", "cursor", "limit"},
	"/quotations/:quotationId/versions":             {"limit"},
	"/quotations/:quotationId/history":              {"cursor", "limit"},
	"/contracts":                                    {"status", "customerId", "branchId", "createdAfter", "createdBefore", "cursor", "limit"},
	"/contracts/:contractId/versions":               {"limit"},
	"/contracts/:contractId/history":                {"cursor", "limit"},
	"/service-level-agreements":                     {"status", "customerId", "contractId", "metricCode", "cursor", "limit"},
	"/service-level-agreements/:slaId/observations": {"outcome", "cursor", "limit"},
	"/service-level-agreements/:slaId/history":      {"cursor", "limit"},
	"/crm/notification-operations":                  {"status", "subjectId", "limit"},
}

// webTypedButNeverApplied are fields present on a shared web filter type that
// the endpoint never applied server-side and that no web page sends today.
// They used to be accepted and silently ignored; they are now refused, which
// is the spec 6 behaviour. Pinned so the decision cannot drift unnoticed.
var webTypedButNeverApplied = map[string][]string{
	"/tracking":             {"branchId", "status", "service"},
	"/leave":                {"branchId"},
	"/training/enrollments": {"branchId"},
}

func sampleValue(field filters.Field) string {
	switch field.Kind {
	case filters.KindLimit:
		return "50"
	case filters.KindOffset:
		return "0"
	case filters.KindBool:
		return "true"
	default:
		return "x"
	}
}

func TestWebAppParametersStayAccepted(t *testing.T) {
	for path, params := range webAppParameters {
		schema, ok := Lookup("GET", TenantPrefix+path)
		if !ok {
			t.Fatalf("%s is not declared", path)
		}
		query := url.Values{}
		for _, name := range params {
			field, declared := schema.Fields[name]
			if !declared {
				t.Fatalf("%s: web parameter %q is not allowlisted", path, name)
			}
			query.Set(name, sampleValue(field))
		}
		if err := filters.Validate(query, schema); err != nil {
			t.Fatalf("%s: web query refused: %v", path, err)
		}
	}
	for path, params := range webTypedButNeverApplied {
		schema, _ := Lookup("GET", TenantPrefix+path)
		for _, name := range params {
			if _, declared := schema.Fields[name]; declared {
				t.Fatalf("%s: %q is declared but the endpoint never applies it", path, name)
			}
		}
	}
}

func TestPagingStylesAreExclusive(t *testing.T) {
	for _, key := range Declared() {
		schema, _ := Lookup("GET", strings.TrimPrefix(key, "GET "))
		_, offset := schema.Fields["offset"]
		_, cursor := schema.Fields["cursor"]
		if offset && cursor {
			t.Fatalf("%s declares both offset and cursor paging", key)
		}
		if offset && schema.Fields["offset"].Kind != filters.KindOffset {
			t.Fatalf("%s: offset is not validated as an offset", key)
		}
		if field, ok := schema.Fields["limit"]; ok && field.Kind != filters.KindLimit {
			t.Fatalf("%s: limit is not validated as a limit", key)
		}
	}
}

// queryRead matches c.Query("name") and its siblings, plus CRM's
// parseTimeQuery(c, "name") helper.
var queryRead = regexp.MustCompile(`(?:c\.(?:Query|DefaultQuery|GetQuery|QueryArray)\(|parseTimeQuery\(c, )"([A-Za-z]+)"`)

// Files that read query parameters on routes outside the corporate tenant
// surface (internal service routes, the customer secure-link route, and the
// organization API's guard against a caller-supplied organization id).
var nonTenantFiles = map[string]bool{
	"backgroundops/http/internal.go":        true,
	"marketplace/http/internal_handlers.go": true,
	"crm/http/public.go":                    true,
	"finance/http.go":                       true,
	"integrations/auth.go":                  true,
}

// TestRegistryCoversEveryQueryNameReadByHandlers is the drift guard: a handler
// that starts reading a new query parameter must declare it here, or the
// production router (middleware.QueryAllowlist) would refuse it with 422.
func TestRegistryCoversEveryQueryNameReadByHandlers(t *testing.T) {
	root := filepath.Join("..", "..", "features")
	declared := Names()
	missing := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if nonTenantFiles[filepath.ToSlash(rel)] {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range queryRead.FindAllStringSubmatch(string(source), -1) {
			if !declared[match[1]] {
				missing[match[1]] = append(missing[match[1]], filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for name, files := range missing {
			names = append(names, name+" ("+strings.Join(files, ", ")+")")
		}
		sort.Strings(names)
		t.Fatalf("query parameters read by handlers but declared by no route: %v", names)
	}
}
