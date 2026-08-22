// Package section17fcontract pins the Section 17F commercial surface: the
// routes the production router registers when a money authority is wired must
// be exactly the routes the OpenAPI document declares, and request bodies must
// carry no financial authority at all.
package section17fcontract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

const (
	commerceBase = "/api/v1/commerce"
	specPath     = "../../docs/openapi/section17f-payment-binding.openapi.json"
)

type document struct {
	Servers []struct {
		URL string `json:"url"`
	} `json:"servers"`
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]struct {
			Required             []string                   `json:"required"`
			Properties           map[string]json.RawMessage `json:"properties"`
			AdditionalProperties any                        `json:"additionalProperties"`
			Enum                 []string                   `json:"enum"`
		} `json:"schemas"`
	} `json:"components"`
}

func load(t *testing.T) document {
	t.Helper()
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	var value document
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func key(method, path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}
	return strings.ToUpper(method) + " " + strings.Join(parts, "/")
}

// routesFor builds the customer surface with and without a money authority, so
// the difference between them is the Section 17F surface.
func routesFor(t *testing.T, paymentEnabled bool) map[string]bool {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	orders.RegisterCustomerRoutes(
		router.Group(commerceBase), &orders.Service{},
		sharedauth.NewTokenSigner([]byte("contract")),
		idempotency.NewStore(nil, time.Hour), true, paymentEnabled,
	)

	out := map[string]bool{}
	for _, route := range router.Routes() {
		out[key(route.Method, route.Path)] = true
	}
	return out
}

func paymentRoutes(t *testing.T) map[string]bool {
	t.Helper()
	base := routesFor(t, false)
	withPayment := routesFor(t, true)

	out := map[string]bool{}
	for route := range withPayment {
		if !base[route] {
			out[route] = true
		}
	}
	return out
}

func specRoutes(t *testing.T) map[string]bool {
	t.Helper()
	doc := load(t)
	if len(doc.Servers) != 1 || doc.Servers[0].URL != commerceBase {
		t.Fatalf("unexpected server base: %+v", doc.Servers)
	}
	out := map[string]bool{}
	for path, methods := range doc.Paths {
		for method := range methods {
			switch method {
			case "get", "post", "put", "patch", "delete":
				out[strings.ToUpper(method)+" "+commerceBase+path] = true
			}
		}
	}
	return out
}

func sorted(set map[string]bool) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func TestSection17FRoutesEqualTheContract(t *testing.T) {
	code, spec := paymentRoutes(t), specRoutes(t)

	if len(code) != 6 {
		t.Fatalf("Section 17F routes=%d want 6: %v", len(code), sorted(code))
	}
	if strings.Join(sorted(code), "\n") != strings.Join(sorted(spec), "\n") {
		t.Fatalf("code/spec drift\ncode=%v\nspec=%v", sorted(code), sorted(spec))
	}
}

// A deployment with no money authority must expose no payment surface at all,
// rather than routes that answer 503 and confirm the capability exists.
func TestSection17FWithoutAMoneyAuthorityThereIsNoPaymentSurface(t *testing.T) {
	for route := range routesFor(t, false) {
		if strings.Contains(strings.ToLower(route), "payment") {
			t.Errorf("payment route registered without a money authority: %s", route)
		}
	}
}

// A commercial change moves money, so it is gated on the same money authority
// as the payment surface itself. Without Wallet there is no route to probe,
// rather than one that could only ever fail after the customer committed.
func TestSection17FWithoutAMoneyAuthorityThereIsNoCommercialChangeSurface(t *testing.T) {
	for route := range routesFor(t, false) {
		if strings.Contains(route, "commercial-changes") {
			t.Errorf("commercial-change route registered without a money "+
				"authority: %s", route)
		}
	}
}

// The four commercial-change routes are part of the accepted surface, and the
// spec declares each of them: a change the customer can request must also be
// one they can read, approve and reject.
func TestSection17FCommercialChangeSurfaceIsComplete(t *testing.T) {
	code, spec := paymentRoutes(t), specRoutes(t)

	for _, route := range []string{
		"POST " + commerceBase + "/orders/{orderId}/commercial-changes",
		"GET " + commerceBase + "/orders/{orderId}/commercial-changes/{changeId}",
		"POST " + commerceBase + "/commercial-changes/{changeId}/approve",
		"POST " + commerceBase + "/commercial-changes/{changeId}/reject",
	} {
		if !code[route] {
			t.Errorf("the production router does not expose %s", route)
		}
		if !spec[route] {
			t.Errorf("the contract does not declare %s", route)
		}
	}
}

// The request body is the tamper surface. It must carry nothing financial.
func TestSection17FRequestCarriesNoFinancialAuthority(t *testing.T) {
	doc := load(t)

	request := doc.Components.Schemas["StartPaymentRequest"]
	if value, ok := request.AdditionalProperties.(bool); !ok || value {
		t.Fatal("start-payment request must reject unknown fields")
	}
	// Exactly one field: the fingerprint the client commits to.
	if len(request.Properties) != 1 || request.Properties["quote_fingerprint"] == nil {
		t.Fatalf("start-payment request properties=%v", request.Properties)
	}

	for name, schema := range doc.Components.Schemas {
		if !strings.HasSuffix(name, "Request") {
			continue
		}
		for field := range schema.Properties {
			lower := strings.ToLower(field)
			for _, forbidden := range []string{
				"amount", "minor", "kobo", "currency", "price", "total",
				"allocation", "provider", "intent", "reference", "status",
				"state", "business_id", "vendor", "customer_id", "wallet",
			} {
				if strings.Contains(lower, forbidden) {
					t.Errorf("%s accepts authority-owned field %q", name, field)
				}
			}
		}
	}
}

// The binding is the security boundary and every part of it must be declared.
func TestSection17FBindingDeclaresEveryMaterialInput(t *testing.T) {
	binding := load(t).Components.Schemas["PaymentBinding"]
	required := map[string]bool{}
	for _, field := range binding.Required {
		required[field] = true
	}
	for _, field := range []string{
		"quote_id", "quote_version", "quote_fingerprint", "quoted_total_minor",
		"max_authorized_total_minor", "currency", "allocation_fingerprint",
	} {
		if !required[field] {
			t.Errorf("PaymentBinding does not require %s", field)
		}
	}
}

// A partially-held payment must have a declared state of its own, so it can
// never be represented as a completed one.
func TestSection17FPaymentStateDeclaresThePartialAndUnwindStates(t *testing.T) {
	state := load(t).Components.Schemas["PaymentState"]
	found := map[string]bool{}
	for _, value := range state.Enum {
		found[value] = true
	}
	for _, want := range []string{
		"PENDING_PAYMENT", "AUTHORIZING", "HELD", "FAILED",
		"COMPENSATING", "COMPENSATED",
	} {
		if !found[want] {
			t.Errorf("PaymentState does not declare %s", want)
		}
	}
}

// The leg contract must keep the authority's own word distinguishable from the
// Vendor-side reading of it.
func TestSection17FLegKeepsTheAuthorityAnswerVerbatim(t *testing.T) {
	leg := load(t).Components.Schemas["PaymentLeg"]
	if leg.Properties["authority_status"] == nil {
		t.Error("PaymentLeg does not record the authority's raw status")
	}
	if leg.Properties["intent_reference"] == nil {
		t.Error("PaymentLeg does not record the authority's own reference")
	}

	scope := leg.Properties["scope"]
	if scope == nil || !strings.Contains(string(scope), "VENDOR") ||
		!strings.Contains(string(scope), "PLATFORM") {
		t.Error("PaymentLeg does not declare the vendor/platform leg split")
	}
}
