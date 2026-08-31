package basket

// Route and contract parity.
//
// An OpenAPI document that drifts from the router is worse than no document:
// a client reads it, implements against it, and discovers the difference in
// production. These tests make drift a build failure by comparing the routes
// the router actually registers against the operations the specification
// actually declares.
//
// The security tests are here for a related reason. "The customer comes from
// the token" is the kind of claim that stays true right up until somebody
// adds a convenience field to a DTO, so the request contracts are asserted to
// carry no customer id, no price and no vendor selection.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

func registeredRoutes(t *testing.T, enabled bool) []string {
	t.Helper()

	gin.SetMode(gin.TestMode)

	router := gin.New()

	service, err := NewService(ServiceOptions{
		Catalogue: &fakeCatalogue{},
		Quoter:    &fakeQuoter{},
	})
	if err != nil {
		t.Fatal(err)
	}

	RegisterCustomerRoutes(
		router.Group("/api/v1/commerce"),
		NewHandler(service),
		sharedauth.NewTokenSigner([]byte("test-secret")),
		enabled,
	)

	var routes []string

	for _, route := range router.Routes() {
		routes = append(routes, route.Method+" "+route.Path)
	}

	sort.Strings(routes)

	return routes
}

func specOperations(t *testing.T) []string {
	t.Helper()

	path := filepath.Join(
		"..", "..", "..", "docs", "openapi",
		"basket-optimization.openapi.json",
	)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}

	var document struct {
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`

		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}

	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("parse spec: %v", err)
	}

	if len(document.Servers) == 0 {
		t.Fatal("spec declares no server base")
	}

	base := document.Servers[0].URL

	var operations []string

	for path, methods := range document.Paths {
		for method := range methods {
			switch strings.ToUpper(method) {
			case "GET", "POST", "PUT", "PATCH", "DELETE":
			default:
				continue
			}

			// OpenAPI writes {id}; gin writes :id. Same route, two spellings.
			ginPath := strings.NewReplacer("{", ":", "}", "").Replace(path)

			operations = append(
				operations,
				strings.ToUpper(method)+" "+base+ginPath,
			)
		}
	}

	sort.Strings(operations)

	return operations
}

func TestRoutes_MatchTheSpecificationExactly(t *testing.T) {
	routes := registeredRoutes(t, true)
	operations := specOperations(t)

	if !reflect.DeepEqual(routes, operations) {
		t.Fatalf(
			"router and specification disagree\nrouter: %v\nspec:   %v",
			routes, operations,
		)
	}
}

// A deployment that has not enabled optimization has NO surface, rather than
// routes that answer 503 and confirm one exists.
func TestRoutes_DisabledFeatureRegistersNothing(t *testing.T) {
	if routes := registeredRoutes(t, false); len(routes) != 0 {
		t.Fatalf("disabled feature registered %v", routes)
	}
}

// ---------------------------------------------------------------------------
// Security: the request contracts carry nothing forgeable
// ---------------------------------------------------------------------------

// Every route authenticates from the bearer token. A request with none must
// be refused before any work is done - not answered from a customer id in the
// body, because there is no such field to read.
func TestRoutes_RefuseUnauthenticatedCallers(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	// The production error renderer, because the auth middleware reports a
	// refusal by putting an error on the context rather than by writing a
	// response itself. A test router without it would see an empty 200 and
	// conclude the route was open, which is precisely the wrong conclusion to
	// draw from a missing middleware.
	router.Use(httpx.RequestID(), httpx.ErrorHandler())

	service, err := NewService(ServiceOptions{
		Catalogue: &fakeCatalogue{},
		Quoter:    &fakeQuoter{},
	})
	if err != nil {
		t.Fatal(err)
	}

	RegisterCustomerRoutes(
		router.Group("/api/v1/commerce"),
		NewHandler(service),
		sharedauth.NewTokenSigner([]byte("test-secret")),
		true,
	)

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/commerce/basket/optimize"},
		{http.MethodPost, "/api/v1/commerce/basket/cart/optimize"},
		{http.MethodGet, "/api/v1/commerce/basket/proposals/bop_1"},
		{http.MethodPost, "/api/v1/commerce/basket/proposals/bop_1/accept"},
		{http.MethodGet, "/api/v1/commerce/basket/recurring"},
		{http.MethodPost, "/api/v1/commerce/basket/recurring"},
	} {
		recorder := httptest.NewRecorder()

		request := httptest.NewRequest(
			tc.method, tc.path, strings.NewReader(`{"customer_id":"someone_else"}`),
		)
		request.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s returned %d, want 401",
				tc.method, tc.path, recorder.Code)
		}
	}
}

// The forgeable fields are absent by construction.
//
// A customer who could put a price, a delivery fee, a vendor or a winning
// variant in a request body could approve one basket and be charged for
// another. This asserts the DTOs have no such field, which is a stronger
// guarantee than validating one away.
func TestRequestContracts_CarryNoAuthoritativeValues(t *testing.T) {
	forbidden := []string{
		"customer_id",
		"customer_ref",
		"price",
		"unit_price",
		"delivery_fee",
		"delivery_total",
		"merchandise_total",
		"final_delivered_total",
		"business_id",
		"branch_id",
		"vendor_id",
		"selected",
		"winner",
		"stock",
		"available_quantity",
		"latitude",
		"longitude",
	}

	for _, contract := range []any{
		optimizeRequest{},
		optimizeCartRequest{},
		templateRequest{},
		templateStateRequest{},
	} {
		value := reflect.TypeOf(contract)

		for index := 0; index < value.NumField(); index++ {
			tag := value.Field(index).Tag.Get("json")

			name := strings.ToLower(strings.Split(tag, ",")[0])

			for _, banned := range forbidden {
				if name == banned {
					t.Fatalf(
						"%s accepts %q from the client; every one of these "+
							"is server-resolved and must not be forgeable",
						value.Name(), name,
					)
				}
			}
		}
	}
}

// The one place a coordinate could enter is the destination, and it enters by
// REFERENCE. Pricing a delivery to arbitrary coordinates would leak
// serviceability answers about points on the map the caller has no connection
// to.
func TestOptimizeRequest_TakesAnAddressReferenceNotCoordinates(t *testing.T) {
	value := reflect.TypeOf(optimizeRequest{})

	field, ok := value.FieldByName("AddressRef")

	if !ok {
		t.Fatal("the budget request has no address reference")
	}

	if field.Type.Kind() != reflect.String {
		t.Fatalf("address reference is %s, want a string reference",
			field.Type.Kind())
	}
}
