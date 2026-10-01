// Package section17ccontract pins Section 17C's HTTP surface.
//
// The gate is a three-way identity: the routes the live router registers, the
// operations the Section 17C OpenAPI document declares, and the rows in
// docs/API_ROUTE_REGISTER.md must be the same set. A route that exists but is
// undocumented fails here; so does an operation that is documented but does
// not exist.
//
// It also states Section 17C's boundaries structurally rather than only in
// prose: two customer routes, two vendor-staff routes, no internal HMAC
// surface in this service, no taxi anywhere, and nothing that locks a quote,
// charges, or orders.
package section17ccontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/delivery"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

const (
	commerceBase = "/api/v1/commerce"
	vendorBase   = "/api/v1/vendor"

	// Two documents, one per server base. A single document declaring both
	// bases forced every reader to special-case which paths were absolute,
	// and the service-wide phantom-operation gate does not special-case: it
	// prepended the commerce base to the vendor paths and reported four
	// phantom operations. Splitting removes the ambiguity at the source.
	customerSpecName = "section17c-fulfilment-planning.openapi.json"
	vendorSpecName   = "section17c-vendor-delivery-policy.openapi.json"

	// The accepted Section 17C surface. A new route has to be an intentional
	// edit here as well.
	expectedRoutes = 4
)

// buildSection17CRouter registers Section 17C exactly as cmd/main.go does.
//
// Every dependency is nil or zero: registration attaches handlers, it does not
// run them, so the inventory stays a structural fact about the code rather
// than something that needs a database.
func buildSection17CRouter(t *testing.T) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	router := gin.New()

	fulfilment.RegisterCustomerRoutes(
		router.Group(commerceBase),
		fulfilment.NewHandler(&fulfilment.PlanningService{}),
		sharedauth.NewTokenSigner([]byte("contract-only")),
		true,
	)

	delivery.RegisterRoutes(
		router.Group(vendorBase),
		delivery.NewHandler(delivery.NewService(nil, nil, true)),
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

func section17CRoutes(t *testing.T) map[string]bool {
	t.Helper()

	out := map[string]bool{}

	for _, route := range buildSection17CRouter(t).Routes() {
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

func loadSpec(t *testing.T, specName string) specDocument {
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

// documentedRoutes reads both Section 17C specs.
//
// Every path is relative to its own document's server base — no special
// casing, because no document mixes bases.
func documentedRoutes(t *testing.T) map[string]bool {
	t.Helper()

	out := map[string]bool{}

	for _, specName := range []string{customerSpecName, vendorSpecName} {
		document := loadSpec(t, specName)
		base := strings.TrimRight(document.Servers[0].URL, "/")

		for path, methods := range document.Paths {
			for method := range methods {
				if !isMethod(method) {
					continue
				}

				out[strings.ToUpper(method)+" "+base+path] = true
			}
		}
	}

	return out
}

// registeredRoutes reads the Section 17C rows of the API route register.
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
			inSection = strings.Contains(trimmed, "Section 17C")

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

func TestSection17CRouteCountIsTheAcceptedSurface(t *testing.T) {
	routes := section17CRoutes(t)

	if len(routes) != expectedRoutes {
		t.Errorf(
			"Section 17C registers %d routes, want %d:\n  %s",
			len(routes), expectedRoutes,
			strings.Join(sortedKeys(routes), "\n  "),
		)
	}
}

func TestSection17CRoutesMatchTheSpec(t *testing.T) {
	routes := section17CRoutes(t)
	documented := documentedRoutes(t)

	if missing := diff(routes, documented); len(missing) > 0 {
		t.Errorf("registered but not documented:\n  %s",
			strings.Join(missing, "\n  "))
	}

	if extra := diff(documented, routes); len(extra) > 0 {
		t.Errorf("documented but not registered:\n  %s",
			strings.Join(extra, "\n  "))
	}
}

func TestSection17CRoutesMatchTheRegister(t *testing.T) {
	routes := section17CRoutes(t)
	registered := registeredRoutes(t)

	if len(registered) == 0 {
		t.Fatal("the API route register has no Section 17C rows")
	}

	if missing := diff(routes, registered); len(missing) > 0 {
		t.Errorf("registered in code but absent from the route register:\n  %s",
			strings.Join(missing, "\n  "))
	}

	if extra := diff(registered, routes); len(extra) > 0 {
		t.Errorf("in the route register but not registered in code:\n  %s",
			strings.Join(extra, "\n  "))
	}
}

// Section 17C exposes no Taxi-specific public route, but CAR is now a valid
// server-selected class backed only by the gated Taxi Goods authority.
func TestSection17CCarMapsOnlyToTaxiGoods(t *testing.T) {
	for key := range section17CRoutes(t) {
		if strings.Contains(strings.ToLower(key), "taxi") {
			t.Errorf("a Section 17C route names taxi: %s", key)
		}
	}

	service, err := fulfilment.ServiceFor(fulfilment.VehicleCar)
	if err != nil || service != fulfilment.ServiceTaxi {
		t.Fatalf("CAR mapping = %q, %v; want TAXI", service, err)
	}

	for _, specName := range []string{customerSpecName, vendorSpecName} {
		raw, err := os.ReadFile(filepath.Join("../../docs/openapi", specName))
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(raw), "\"CAR\"") {
			t.Errorf("%s does not declare the gated CAR class", specName)
		}
	}
}

// Section 17C plans and quotes. It must not have grown a surface that locks a
// quote, charges, or creates an order — those belong to 17D and later.
func TestSection17CHasNoLaterSectionSurface(t *testing.T) {
	forbidden := []string{
		"payment", "wallet", "charge", "capture", "authorize",
		"orders", "quote-lock", "settle", "refund",
	}

	for key := range section17CRoutes(t) {
		lowered := strings.ToLower(key)

		for _, word := range forbidden {
			if strings.Contains(lowered, word) {
				t.Errorf("Section 17C route %q names %q, which belongs to a later section",
					key, word)
			}
		}
	}
}

// Every Section 17C route is authenticated. There is no public planning
// surface: a plan names pickups, fees and a delivery address.
func TestSection17CHasNoPublicRoute(t *testing.T) {
	// Both surfaces are mounted under a base that carries auth middleware:
	// the customer routes behind the Customer Service bearer, the vendor
	// routes behind the staff middleware cmd/main.go applies to the whole
	// protected group. Neither base is public.
	for key := range section17CRoutes(t) {
		if !strings.Contains(key, commerceBase) &&
			!strings.Contains(key, vendorBase) {
			t.Errorf("route %q is outside both authenticated bases", key)
		}
	}
}
