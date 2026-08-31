// Package section13contract pins Section 13's own HTTP surface.
//
// internal/crmcontract already holds the WHOLE service against every
// specification. This package narrows the same question to Section 13 and adds
// the checks that are specific to it and that a whole-service count cannot
// express: that the vendor surface and the platform surface are mounted behind
// different doors, that no taxonomy or search route is reachable through the
// public base except the ten vendor classification routes, and that every
// internal operation is documented as HMAC-protected rather than bearer.
//
// The distinction matters more here than in any previous section. Section 13's
// internal routes can publish the vocabulary every vendor on the platform is
// classified against and can reorder every search result; a single one of them
// mounted under /api/v1/vendor would be that authority behind a public door.
package section13contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
	"cosmicforge/logistics/services/vendor-service/internal/features/taxonomy"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	vendorBase   = "/api/v1/vendor"
	internalBase = "/internal/vendor/v1"

	expectedVendorTaxonomyRoutes = 10
	expectedAdminTaxonomyRoutes  = 33
	expectedSearchRoutes         = 6
	expectedProjectionRoutes     = 11
)

// buildSection13Router registers Section 13 exactly as cmd/main.go does, with
// every flag on. No dependency is real: registration attaches handlers, it does
// not run them, so this stays a structural fact about the code.
func buildSection13Router(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	taxonomyFlags := taxonomy.Flags{TaxonomyEnabled: true, WritesEnabled: true}
	taxonomyService := taxonomy.NewService(taxonomy.NewRepository(nil), nil)
	taxonomyAccess := taxonomy.NewVendorAccess(nil, nil)

	taxonomy.RegisterRoutes(
		router.Group(vendorBase),
		taxonomy.NewHandler(taxonomyService, taxonomyAccess, taxonomyFlags),
		taxonomyFlags,
	)

	verifier := serviceauth.NewVerifier(serviceauth.Secrets{}, 5*time.Minute)
	taxonomy.RegisterInternalRoutes(
		router,
		taxonomy.NewInternalHandler(taxonomyService, []string{"admin-backoffice-service"}, nil, taxonomyFlags),
		verifier,
		taxonomyFlags,
	)

	searchFlags := searchprojection.Flags{SearchEnabled: true, AdminEnabled: true}
	searchprojection.RegisterInternalSearchRoutes(
		router,
		searchprojection.NewInternalHandler(searchprojection.InternalHandlerOptions{
			Callers: []string{"order-service"},
			Flags:   searchFlags,
		}),
		verifier,
		searchFlags,
	)

	return router
}

func routeKey(method, path string) string {
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		if strings.HasPrefix(segment, ":") {
			segments[index] = "{" + strings.TrimPrefix(segment, ":") + "}"
		}
	}
	return method + " " + strings.Join(segments, "/")
}

func section13Routes(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, route := range buildSection13Router(t).Routes() {
		out[routeKey(route.Method, route.Path)] = true
	}
	return out
}

// ---------------------------------------------------------------------------
// Specification loading
// ---------------------------------------------------------------------------

type specDocument struct {
	Servers []struct {
		URL string `json:"url"`
	} `json:"servers"`
	Security []map[string][]string                 `json:"security"`
	Paths    map[string]map[string]json.RawMessage `json:"paths"`
}

var section13Specs = []string{
	"section13-taxonomy.openapi.json",
	"section13-taxonomy-admin.openapi.json",
	"section13-search.openapi.json",
	"section13-search-projection.openapi.json",
}

func loadSpec(t *testing.T, name string) specDocument {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("../../docs/openapi", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var document specDocument
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	if len(document.Servers) == 0 {
		t.Fatalf("%s declares no server base", name)
	}
	return document
}

func isMethod(value string) bool {
	switch value {
	case "get", "post", "put", "patch", "delete":
		return true
	default:
		return false
	}
}

// documentedRoutes maps each Section 13 operation to the spec that declares it.
func documentedRoutes(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, name := range section13Specs {
		document := loadSpec(t, name)
		base := strings.TrimRight(document.Servers[0].URL, "/")
		for path, methods := range document.Paths {
			for method := range methods {
				if !isMethod(method) {
					continue
				}
				full := path
				if !strings.HasPrefix(path, "/internal/") {
					full = base + path
				}
				out[strings.ToUpper(method)+" "+full] = name
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Parity
// ---------------------------------------------------------------------------

func TestSection13RouteCountsAreTheAcceptedSurface(t *testing.T) {
	routes := section13Routes(t)

	counts := map[string]int{}
	for route := range routes {
		_, path, _ := strings.Cut(route, " ")
		switch {
		case strings.HasPrefix(path, vendorBase+"/taxonomy"):
			counts["vendor_taxonomy"]++
		case strings.HasPrefix(path, internalBase+"/taxonomy"):
			counts["admin_taxonomy"]++
		case strings.HasPrefix(path, internalBase+"/search/food"),
			strings.HasPrefix(path, internalBase+"/search/shop"):
			counts["search"]++
		case strings.HasPrefix(path, internalBase+"/search"):
			counts["projection"]++
		default:
			t.Errorf("route %s belongs to no Section 13 family", route)
		}
	}

	for name, want := range map[string]int{
		"vendor_taxonomy": expectedVendorTaxonomyRoutes,
		"admin_taxonomy":  expectedAdminTaxonomyRoutes,
		"search":          expectedSearchRoutes,
		"projection":      expectedProjectionRoutes,
	} {
		if counts[name] != want {
			t.Errorf("%s routes=%d want=%d", name, counts[name], want)
		}
	}
}

func TestSection13EveryLiveRouteIsDocumented(t *testing.T) {
	documented := documentedRoutes(t)
	var missing []string
	for route := range section13Routes(t) {
		if _, ok := documented[route]; !ok {
			missing = append(missing, route)
		}
	}
	sort.Strings(missing)
	if len(missing) != 0 {
		t.Fatalf("%d Section 13 routes appear in no specification:\n%s",
			len(missing), strings.Join(missing, "\n"))
	}
}

func TestSection13NoDocumentedOperationIsPhantom(t *testing.T) {
	routes := section13Routes(t)
	var phantom []string
	for route, spec := range documentedRoutes(t) {
		if !routes[route] {
			phantom = append(phantom, route+"  ("+spec+")")
		}
	}
	sort.Strings(phantom)
	if len(phantom) != 0 {
		t.Fatalf("%d documented Section 13 operations do not exist:\n%s",
			len(phantom), strings.Join(phantom, "\n"))
	}
}

// ---------------------------------------------------------------------------
// Authority separation
// ---------------------------------------------------------------------------

// TestSection13InternalRoutesAreNeverPublic is the check this package exists
// for. Publishing a taxonomy version or activating a projection generation
// through the gateway-forwarded base would put platform authority behind a
// vendor-authenticated door.
func TestSection13InternalRoutesAreNeverPublic(t *testing.T) {
	for route := range section13Routes(t) {
		_, path, _ := strings.Cut(route, " ")
		if strings.Contains(path, "/internal/") && strings.HasPrefix(path, vendorBase) {
			t.Errorf("internal route %s is mounted under the public vendor base", route)
		}
	}
}

// TestSection13PublicSurfaceIsClassificationOnly pins what a vendor bearer
// token can reach: reads of the published vocabulary, reads of their own
// classification, and one mapping-replacement command. Nothing that authors,
// publishes, governs, ranks or rebuilds.
func TestSection13PublicSurfaceIsClassificationOnly(t *testing.T) {
	// Matched as whole path segments, not substrings: "published-version" is a
	// legitimate vendor read and must not be mistaken for the platform
	// "versions" collection.
	forbidden := map[string]bool{
		"versions": true, "publish": true, "retire": true, "validate": true,
		"governance-cases": true, "semantic-seed": true, "seed-status": true,
		"seed-runs": true, "ranking-profiles": true, "projections": true,
		"projection-health": true, "projection-jobs": true, "facet-mappings": true,
		"authority-vocabulary": true, "publication-history": true, "clone-active": true,
	}
	for route := range section13Routes(t) {
		method, path, _ := strings.Cut(route, " ")
		if !strings.HasPrefix(path, vendorBase) {
			continue
		}
		for _, segment := range strings.Split(path, "/") {
			if forbidden[segment] {
				t.Errorf("platform capability %s is reachable on the public vendor base", route)
			}
		}
		// The only public mutation Section 13 owns is mapping replacement.
		if method != "GET" && !strings.HasSuffix(path, "/mappings") {
			t.Errorf("unexpected public Section 13 mutation %s", route)
		}
	}
}

// TestSection13SpecSecurityMatchesMountPoint checks the specifications declare
// the authority model each surface actually enforces. A spec that advertises a
// bearer token on an HMAC-only route would send an integrator at a 401 and,
// worse, imply that a vendor could reach it.
func TestSection13SpecSecurityMatchesMountPoint(t *testing.T) {
	for _, name := range section13Specs {
		document := loadSpec(t, name)
		if len(document.Security) != 1 {
			t.Fatalf("%s must declare exactly one top-level security requirement", name)
		}
		var scheme string
		for key := range document.Security[0] {
			scheme = key
		}

		base := strings.TrimRight(document.Servers[0].URL, "/")
		internal := base == internalBase
		for path := range document.Paths {
			if strings.HasPrefix(path, "/internal/") {
				internal = true
			}
		}

		switch {
		case internal && scheme != "hmacV2":
			t.Errorf("%s serves %s but declares %q rather than hmacV2", name, base, scheme)
		case !internal && scheme != "bearerAuth":
			t.Errorf("%s serves %s but declares %q rather than bearerAuth", name, base, scheme)
		}
	}
}

// TestSection13InternalOperationsDocumentTheHMACHeaders keeps the contract
// honest about what a caller must actually send. An internal operation whose
// parameters omit the nonce or signature header reads as if the request could
// be replayed, or signed with fewer bindings than the verifier requires.
func TestSection13InternalOperationsDocumentTheHMACHeaders(t *testing.T) {
	required := []string{
		"X-Service-Name", "X-Service-Timestamp",
		"X-Service-Nonce", "X-Service-Signature",
	}

	type operation struct {
		Parameters []struct {
			In   string `json:"in"`
			Name string `json:"name"`
		} `json:"parameters"`
	}

	for _, name := range section13Specs {
		document := loadSpec(t, name)
		if strings.TrimRight(document.Servers[0].URL, "/") != internalBase {
			continue
		}
		for path, methods := range document.Paths {
			for method, raw := range methods {
				if !isMethod(method) {
					continue
				}
				var op operation
				if err := json.Unmarshal(raw, &op); err != nil {
					t.Fatalf("%s %s %s: %v", name, method, path, err)
				}
				headers := map[string]bool{}
				for _, parameter := range op.Parameters {
					if parameter.In == "header" {
						headers[parameter.Name] = true
					}
				}
				for _, header := range required {
					if !headers[header] {
						t.Errorf("%s %s %s does not document %s", name, strings.ToUpper(method), path, header)
					}
				}
			}
		}
	}
}
