// Package section15contract pins Section 15's HTTP surface.
//
// The gate is a three-way identity: the routes the live router registers, the
// operations the Section 15 OpenAPI document declares, and the rows in
// docs/API_ROUTE_REGISTER.md must be the same set.
//
// Section 15 had none of this before 15D. Its 56 routes were live in
// cmd/main.go, absent from every specification, absent from the route register,
// and absent from the service-wide contract router — so the test whose job is
// catching undocumented routes could not see any of them.
package section15contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/purchasing"
)

const (
	vendorBase   = "/api/v1/vendor"
	internalBase = "/internal/vendor/v1"

	specName = "section15-vendor-purchasing.openapi.json"

	// The accepted Section 15 surface. A new route has to be an intentional
	// edit here as well.
	expectedRoutes = 56
)

// buildSection15Router registers Section 15 exactly as cmd/main.go does.
// No dependency is real: registration attaches handlers, it does not run them.
func buildSection15Router(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	group := router.Group(vendorBase)
	base := purchasing.NewHandler(nil, nil)

	purchasing.RegisterRoutes(group, base, true)
	purchasing.RegisterPurchaseOrderRoutes(
		group, purchasing.NewPurchaseOrderHandler(nil, base, nil), true)
	purchasing.RegisterGoodsReceivingRoutes(
		group, purchasing.NewGoodsReceivingHandler(nil, nil, nil), true)
	purchasing.RegisterOperationsRoutes(
		group, purchasing.NewOperationsHandler(nil, nil), true)

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

func section15Routes(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, route := range buildSection15Router(t).Routes() {
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

// registeredRoutes reads the Section 15 rows of the API route register.
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
			inSection = strings.Contains(trimmed, "Section 15")
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

func TestSection15RouteCountIsTheAcceptedSurface(t *testing.T) {
	routes := section15Routes(t)

	if len(routes) != expectedRoutes {
		t.Errorf("Section 15 registers %d routes, want %d:\n  %s",
			len(routes), expectedRoutes, strings.Join(sortedKeys(routes), "\n  "))
	}

	for key := range routes {
		path := key[strings.Index(key, " ")+1:]
		if !strings.HasPrefix(path, vendorBase+"/purchasing") {
			t.Errorf("Section 15 route is mounted outside the purchasing base: %s", key)
		}
	}
}

// TestSection15RegistersNoInternalRoute states the boundary explicitly: no
// other service drives purchasing, so purchasing exposes no HMAC surface. If
// one is added later, it must be a deliberate change here too.
func TestSection15RegistersNoInternalRoute(t *testing.T) {
	for key := range section15Routes(t) {
		path := key[strings.Index(key, " ")+1:]
		if strings.HasPrefix(path, internalBase) || strings.Contains(path, "/internal/") {
			t.Errorf("Section 15 registered an internal route: %s", key)
		}
	}
}

func TestSection15LiveRouterMatchesOpenAPI(t *testing.T) {
	routes := section15Routes(t)
	documented := documentedRoutes(t)

	if missing := diff(routes, documented); len(missing) > 0 {
		t.Errorf("live Section 15 routes missing from %s:\n  %s",
			specName, strings.Join(missing, "\n  "))
	}
	if phantom := diff(documented, routes); len(phantom) > 0 {
		t.Errorf("%s documents routes the router does not register:\n  %s",
			specName, strings.Join(phantom, "\n  "))
	}
}

func TestSection15LiveRouterMatchesRouteRegister(t *testing.T) {
	routes := section15Routes(t)
	registered := registeredRoutes(t)

	if len(registered) == 0 {
		t.Fatal("API_ROUTE_REGISTER.md declares no Section 15 routes")
	}

	if missing := diff(routes, registered); len(missing) > 0 {
		t.Errorf("live Section 15 routes missing from API_ROUTE_REGISTER.md:\n  %s",
			strings.Join(missing, "\n  "))
	}
	if stale := diff(registered, routes); len(stale) > 0 {
		t.Errorf("API_ROUTE_REGISTER.md lists Section 15 routes the router does not register:\n  %s",
			strings.Join(stale, "\n  "))
	}
}

// TestSection15DocumentsItsBoundaries keeps the two claims that matter most in
// the specification itself, not only in prose elsewhere.
func TestSection15DocumentsItsBoundaries(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("../../docs/openapi", specName))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)

	for _, needle := range []string{
		"accepted",             // only accepted stock is posted
		"never moves money",    // no wallet mutation
		"INSUFFICIENT_HISTORY", // honest supplier performance
		"exactly-once",         // one posting per goods receipt
	} {
		if !strings.Contains(text, needle) {
			t.Errorf("%s never mentions %q; the boundary is undocumented",
				specName, needle)
		}
	}
}
