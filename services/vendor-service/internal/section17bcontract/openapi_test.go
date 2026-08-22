// Package section17bcontract pins Section 17B's HTTP surface.
//
// The gate is a three-way identity: the routes the live router registers, the
// operations the Section 17B OpenAPI document declares, and the rows in
// docs/API_ROUTE_REGISTER.md must be the same set. A route that exists but is
// undocumented fails here; so does an operation that is documented but does
// not exist.
//
// It also states Section 17B's boundaries structurally rather than only in
// prose: three customer routes, no internal HMAC surface, no vendor-facing
// surface, and nothing that quotes, charges or orders.
package section17bcontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

const (
	commerceBase = "/api/v1/commerce"
	vendorBase   = "/api/v1/vendor"
	internalBase = "/internal/vendor/v1"

	specName = "section17b-commerce-checkout.openapi.json"

	// The accepted Section 17B surface. A new route has to be an intentional
	// edit here as well.
	expectedRoutes = 3
)

// buildSection17BRouter registers Section 17B exactly as cmd/main.go does.
//
// Every dependency is nil or zero: registration attaches handlers, it does not
// run them, so the inventory stays a structural fact about the code rather
// than something that needs a database.
func buildSection17BRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	checkout.RegisterCustomerRoutes(
		router.Group(commerceBase),
		&checkout.Service{},
		sharedauth.NewTokenSigner([]byte("contract-only")),
		nil,
		true,
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

func section17BRoutes(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, route := range buildSection17BRouter(t).Routes() {
		key := routeKey(route.Method, route.Path)
		if out[key] {
			t.Errorf("route registered twice: %s", key)
		}
		out[key] = true
	}
	return out
}

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

// registeredRoutes reads the Section 17B rows of the API route register.
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
			inSection = strings.Contains(trimmed, "Section 17B")
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

func diff(left, right map[string]bool) []string {
	var out []string
	for _, key := range sortedKeys(left) {
		if !right[key] {
			out = append(out, key)
		}
	}
	return out
}

func TestSection17BRouteCountIsTheAcceptedSurface(t *testing.T) {
	routes := section17BRoutes(t)

	if len(routes) != expectedRoutes {
		t.Errorf("Section 17B registers %d routes, want %d:\n  %s",
			len(routes), expectedRoutes, strings.Join(sortedKeys(routes), "\n  "))
	}

	for key := range routes {
		path := key[strings.Index(key, " ")+1:]
		if !strings.HasPrefix(path, commerceBase+"/checkout-sessions") {
			t.Errorf("Section 17B route is mounted outside the checkout base: %s", key)
		}
	}
}

// TestSection17BRegistersNoInternalOrVendorRoute states the boundary
// explicitly. Checkout is a customer surface: no other service drives it, and
// no vendor staff member operates it.
func TestSection17BRegistersNoInternalOrVendorRoute(t *testing.T) {
	for key := range section17BRoutes(t) {
		path := key[strings.Index(key, " ")+1:]

		if strings.HasPrefix(path, internalBase) ||
			strings.Contains(path, "/internal/") {
			t.Errorf("Section 17B registered an internal route: %s", key)
		}

		if strings.HasPrefix(path, vendorBase) {
			t.Errorf("Section 17B registered a vendor-facing route: %s", key)
		}
	}
}

// TestSection17BNamesNoCustomerAnywhereInItsSurface is the isolation rule
// stated structurally. Customer identity comes only from the bearer token, so
// a customer identifier in a path would be a way to ask about someone else.
func TestSection17BNamesNoCustomerAnywhereInItsSurface(t *testing.T) {
	for key := range section17BRoutes(t) {
		lowered := strings.ToLower(key)

		if strings.Contains(lowered, "customer") {
			t.Errorf("Section 17B route names a customer: %s", key)
		}
	}
}

// TestSection17BRegistersNothingWhenDisabled proves the flag actually gates
// the surface rather than only the behaviour behind it.
func TestSection17BRegistersNothingWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	checkout.RegisterCustomerRoutes(
		router.Group(commerceBase),
		&checkout.Service{},
		sharedauth.NewTokenSigner([]byte("contract-only")),
		nil,
		false,
	)

	if routes := router.Routes(); len(routes) != 0 {
		t.Errorf("disabled Section 17B still registered %d routes", len(routes))
	}
}

func TestSection17BLiveRouterMatchesOpenAPI(t *testing.T) {
	routes := section17BRoutes(t)
	documented := documentedRoutes(t)

	if missing := diff(routes, documented); len(missing) > 0 {
		t.Errorf("live Section 17B routes missing from %s:\n  %s",
			specName, strings.Join(missing, "\n  "))
	}
	if phantom := diff(documented, routes); len(phantom) > 0 {
		t.Errorf("%s documents routes the router does not register:\n  %s",
			specName, strings.Join(phantom, "\n  "))
	}
}

func TestSection17BLiveRouterMatchesRouteRegister(t *testing.T) {
	routes := section17BRoutes(t)
	registered := registeredRoutes(t)

	if len(registered) == 0 {
		t.Fatal("API_ROUTE_REGISTER.md declares no Section 17B routes")
	}

	if missing := diff(routes, registered); len(missing) > 0 {
		t.Errorf("live Section 17B routes missing from API_ROUTE_REGISTER.md:\n  %s",
			strings.Join(missing, "\n  "))
	}
	if stale := diff(registered, routes); len(stale) > 0 {
		t.Errorf("API_ROUTE_REGISTER.md lists Section 17B routes the router does not register:\n  %s",
			strings.Join(stale, "\n  "))
	}
}

// TestSection17BDocumentsItsBoundaries keeps the claims that matter most in
// the specification itself, not only in prose elsewhere.
func TestSection17BDocumentsItsBoundaries(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("../../docs/openapi", specName))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)

	for _, needle := range []string{
		"NOT a quote",                   // the candidate is not the quote
		"no payment",                    // no money moves here
		"Food reserves nothing",         // the Food boundary
		"before the Inventory call",     // the recovery guarantee
		"never invents a conversion",    // the fail-closed measure rule
		"no hold left behind",           // multi-vendor atomicity
		"deployment configuration",      // the TTL is never hardcoded
		"Search remains discovery only", // search is not an authority
	} {
		if !strings.Contains(text, needle) {
			t.Errorf("%s never mentions %q; the boundary is undocumented",
				specName, needle)
		}
	}
}

// TestSection17BDocumentsNoQuoteOrPaymentOperation refuses a later section's
// surface structurally rather than by convention.
func TestSection17BDocumentsNoQuoteOrPaymentOperation(t *testing.T) {
	document := loadSpec(t)

	for path := range document.Paths {
		lowered := strings.ToLower(path)

		for _, forbidden := range []string{
			"quote",
			"payment",
			"pay",
			"wallet",
			"order",
			"fulfil",
			"logistics",
			"dispatch",
		} {
			if strings.Contains(lowered, forbidden) {
				t.Errorf("Section 17B documents a %q path: %s", forbidden, path)
			}
		}
	}
}
