// Package section14contract pins Section 14's own HTTP surface.
//
// The gate is a three-way identity: the routes the live router registers, the
// operations the Section 14 OpenAPI document declares, and the rows in
// docs/API_ROUTE_REGISTER.md must be the same set. Any one of them drifting is
// a documentation defect that this package fails on rather than a discrepancy
// someone discovers against production.
//
// Section 14 makes the internal half matter more than usual. The internal
// Inventory routes can reserve, commit and release another service's stock; a
// single one of them mounted under the public vendor base would be that
// authority behind a bearer-token door instead of an HMAC one.
package section14contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	vendorBase   = "/api/v1/vendor"
	internalBase = "/internal/vendor/v1"

	specName = "section14-vendor-inventory.openapi.json"

	// The accepted Section 14 surface. These are assertions, not derived
	// numbers: a new route has to be an intentional edit here as well.
	expectedPublicRoutes   = 27
	expectedInternalRoutes = 7
)

// buildSection14Router registers Section 14 exactly as cmd/main.go does, with
// every flag on. No dependency is real: registration attaches handlers, it does
// not run them, so this stays a structural fact about the code.
func buildSection14Router(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	flags := inventory.Flags{Enabled: true, WritesEnabled: true}

	handler := inventory.NewHandler(nil, nil)

	protected := router.Group(vendorBase)
	inventory.RegisterRoutes(protected, handler, flags)
	inventory.RegisterReservationRoutes(protected, handler, flags)
	inventory.RegisterOperationalRoutes(protected, handler, flags)

	verifier := serviceauth.NewVerifier(serviceauth.Secrets{}, 5*time.Minute)
	inventory.RegisterInternalRoutes(
		router,
		inventory.NewInternalInventoryHandler(nil, []string{"order-service"}),
		verifier,
		flags,
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

func section14Routes(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, route := range buildSection14Router(t).Routes() {
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
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

func loadSpec(t *testing.T) specDocument {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("../../docs/openapi", specName))
	if err != nil {
		t.Fatalf("read %s: %v", specName, err)
	}
	var document specDocument
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("parse %s: %v", specName, err)
	}
	if len(document.Servers) == 0 {
		t.Fatalf("%s declares no server base", specName)
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

// documentedRoutes returns every operation the Section 14 spec declares.
// Internal paths are absolute; public paths hang off the declared server base.
func documentedRoutes(t *testing.T) map[string]bool {
	t.Helper()
	document := loadSpec(t)
	base := strings.TrimRight(document.Servers[0].URL, "/")

	out := map[string]bool{}
	for path, methods := range document.Paths {
		for method := range methods {
			if !isMethod(method) {
				continue
			}
			full := path
			if !strings.HasPrefix(path, "/internal/") {
				full = base + path
			}
			out[strings.ToUpper(method)+" "+full] = true
		}
	}
	return out
}

// registeredRoutes returns the Section 14 rows of the API route register.
func registeredRoutes(t *testing.T) map[string]bool {
	t.Helper()

	raw, err := os.ReadFile("../../docs/API_ROUTE_REGISTER.md")
	if err != nil {
		t.Fatal(err)
	}

	out := map[string]bool{}
	inSection := false

	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## ") {
			inSection = strings.Contains(trimmed, "Section 14")
			continue
		}
		if !inSection || !strings.HasPrefix(trimmed, "|") {
			continue
		}

		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		if len(cells) < 2 {
			continue
		}

		method := strings.TrimSpace(cells[0])
		path := strings.TrimSpace(cells[1])

		if !isMethod(strings.ToLower(method)) {
			continue
		}
		out[strings.ToUpper(method)+" "+path] = true
	}

	return out
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// diff reports members of left that are absent from right.
func diff(left, right map[string]bool) []string {
	var out []string
	for _, key := range sortedKeys(left) {
		if !right[key] {
			out = append(out, key)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Parity
// ---------------------------------------------------------------------------

func TestSection14RouteCountsAreTheAcceptedSurface(t *testing.T) {
	routes := section14Routes(t)

	var public, internal int
	for key := range routes {
		path := key[strings.Index(key, " ")+1:]
		switch {
		case strings.HasPrefix(path, internalBase):
			internal++
		case strings.HasPrefix(path, vendorBase):
			public++
		default:
			t.Errorf("Section 14 route is mounted outside both accepted bases: %s", key)
		}
	}

	if public != expectedPublicRoutes {
		t.Errorf("public Section 14 routes = %d, want %d:\n  %s",
			public, expectedPublicRoutes, strings.Join(sortedKeys(routes), "\n  "))
	}
	if internal != expectedInternalRoutes {
		t.Errorf("internal Section 14 routes = %d, want %d", internal, expectedInternalRoutes)
	}
}

// TestSection14InternalAuthorityIsNotPubliclyMounted is the security half of
// parity. Stock mutation on behalf of another service must never be reachable
// through the bearer-authenticated vendor base.
func TestSection14InternalAuthorityIsNotPubliclyMounted(t *testing.T) {
	for key := range section14Routes(t) {
		path := key[strings.Index(key, " ")+1:]

		if strings.HasPrefix(path, vendorBase) &&
			strings.Contains(path, "/internal/") {
			t.Errorf("internal Inventory authority is mounted under the public base: %s", key)
		}
	}
}

func TestSection14LiveRouterMatchesOpenAPI(t *testing.T) {
	routes := section14Routes(t)
	documented := documentedRoutes(t)

	if missing := diff(routes, documented); len(missing) > 0 {
		t.Errorf("live Section 14 routes missing from %s:\n  %s",
			specName, strings.Join(missing, "\n  "))
	}
	if phantom := diff(documented, routes); len(phantom) > 0 {
		t.Errorf("%s documents routes the router does not register:\n  %s",
			specName, strings.Join(phantom, "\n  "))
	}
}

func TestSection14LiveRouterMatchesRouteRegister(t *testing.T) {
	routes := section14Routes(t)
	registered := registeredRoutes(t)

	if len(registered) == 0 {
		t.Fatal("API_ROUTE_REGISTER.md declares no Section 14 routes")
	}

	if missing := diff(routes, registered); len(missing) > 0 {
		t.Errorf("live Section 14 routes missing from API_ROUTE_REGISTER.md:\n  %s",
			strings.Join(missing, "\n  "))
	}
	if stale := diff(registered, routes); len(stale) > 0 {
		t.Errorf("API_ROUTE_REGISTER.md lists Section 14 routes the router does not register:\n  %s",
			strings.Join(stale, "\n  "))
	}
}

// TestSection14InternalOperationsAreDocumentedAsHMAC keeps the two doors
// distinguishable in the specification itself, not only in the code.
func TestSection14InternalOperationsAreDocumentedAsHMAC(t *testing.T) {
	document := loadSpec(t)

	var internalPaths int
	for path := range document.Paths {
		if strings.HasPrefix(path, internalBase) {
			internalPaths++
		}
	}

	if internalPaths == 0 {
		t.Fatalf("%s documents no internal Inventory operations", specName)
	}

	raw, err := os.ReadFile(filepath.Join("../../docs/openapi", specName))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)

	for _, needle := range []string{"hmacV2", "X-Service-Nonce"} {
		if !strings.Contains(text, needle) {
			t.Errorf("%s never mentions %q; the internal door is undocumented", specName, needle)
		}
	}
}
