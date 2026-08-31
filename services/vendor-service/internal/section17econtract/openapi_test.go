// Package section17econtract pins the Section 17E surface: the routes the
// production router registers and the routes the OpenAPI document declares
// must be the same set, and the request bodies must accept no commercial or
// payment authority.
package section17econtract

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
	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	apiBase                 = "/api/v1"
	commerceBase            = "/api/v1/commerce"
	vendorBase              = "/api/v1/vendor"
	specPath                = "../../docs/openapi/section17e-parent-vendor-orders.openapi.json"
	section18SpecPath       = "../../docs/openapi/section18-vendor-order-lifecycle.openapi.json"
	commerceSupportSpecPath = "../../docs/openapi/commerce-support-cases.openapi.json"
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
			Description          string                     `json:"description"`
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

// codeRoutes mirrors cmd/main.go: the customer surface is mounted at the
// commerce base and the vendor surface under the vendor base.
func codeRoutes(t *testing.T) map[string]bool {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	service := &orders.Service{}
	orders.RegisterCustomerRoutes(
		router.Group(commerceBase), service,
		sharedauth.NewTokenSigner([]byte("contract")),
		// Payment is deliberately off here: the Section 17E surface is what
		// exists without a money authority, and Section 17F pins its own.
		idempotency.NewStore(nil, time.Hour), true, false,
	)
	orders.RegisterVendorRoutes(router.Group(vendorBase), service, true)
	orders.RegisterInternalLogisticsRoutes(router, service, serviceauth.NewVerifier(serviceauth.Secrets{}, time.Minute))

	out := map[string]bool{}
	for _, route := range router.Routes() {
		out[key(route.Method, route.Path)] = true
	}
	return out
}

func specRoutes(t *testing.T) map[string]bool {
	t.Helper()
	doc := load(t)
	if len(doc.Servers) != 1 || doc.Servers[0].URL != apiBase {
		t.Fatalf("unexpected server base: %+v", doc.Servers)
	}
	out := map[string]bool{}
	for path, methods := range doc.Paths {
		for method := range methods {
			switch method {
			case "get", "post", "put", "patch", "delete":
				out[strings.ToUpper(method)+" "+apiBase+path] = true
			}
		}
	}
	// Section 18 deliberately extends the Section 17 order surface. Keeping
	// its operations in a separate document avoids rewriting the historical
	// Section 17 schema while this guard still checks the complete router.
	raw, err := os.ReadFile(section18SpecPath)
	if err != nil {
		t.Fatal(err)
	}
	var section18 document
	if err = json.Unmarshal(raw, &section18); err != nil {
		t.Fatal(err)
	}
	for path, methods := range section18.Paths {
		for method := range methods {
			if method == "get" || method == "post" || method == "put" || method == "patch" || method == "delete" {
				fullPath := path
				if !strings.HasPrefix(fullPath, apiBase+"/") && !strings.HasPrefix(fullPath, "/internal/") {
					fullPath = apiBase + fullPath
				}
				out[strings.ToUpper(method)+" "+fullPath] = true
			}
		}
	}
	// Later Commerce support integration extends the same shared customer and
	// Vendor order route helpers with five public case operations. Its private
	// resolution callback has a separate root registration and is intentionally
	// excluded from this historical helper inventory.
	raw, err = os.ReadFile(commerceSupportSpecPath)
	if err != nil {
		t.Fatal(err)
	}
	var support document
	if err = json.Unmarshal(raw, &support); err != nil {
		t.Fatal(err)
	}
	for path, methods := range support.Paths {
		if strings.HasPrefix(path, "/api/v1/internal/") {
			continue
		}
		for method := range methods {
			if method == "get" || method == "post" || method == "put" || method == "patch" || method == "delete" {
				out[strings.ToUpper(method)+" "+path] = true
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

func TestSection17ERoutesEqualTheContract(t *testing.T) {
	code, spec := codeRoutes(t), specRoutes(t)

	// Section 17's routes plus Section 18 lifecycle and Shop picking evidence
	// routes, including the root-mounted authenticated logistics ingress.
	if len(code) != 43 {
		t.Fatalf("shared order routes=%d want 43: %v", len(code), sorted(code))
	}
	if strings.Join(sorted(code), "\n") != strings.Join(sorted(spec), "\n") {
		t.Fatalf("code/spec drift\ncode=%v\nspec=%v", sorted(code), sorted(spec))
	}
}

// Section 17E creates orders. Payment execution and mission execution belong to
// later sections, so no route here may name them.
func TestSection17ERegistersNoPaymentOrMissionRoute(t *testing.T) {
	for route := range codeRoutes(t) {
		lower := strings.ToLower(route)
		for _, forbidden := range []string{
			"payment", "wallet", "capture", "authorize", "charge",
			"settle", "payout", "mission", "dispatch",
		} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("later-section route leaked into 17E: %s", route)
			}
		}
	}
}

// Every lifecycle command exposed by the domain must have exactly one route, so
// a new transition cannot be added to the state machine without a surface, or
// to the surface without the state machine.
func TestSection17EEveryTransitionHasExactlyOneRoute(t *testing.T) {
	code := codeRoutes(t)
	for _, transition := range orders.AllTransitions {
		verb := strings.ToLower(string(transition))
		want := "POST " + vendorBase + "/commerce/orders/{vendorOrderId}/" + verb
		if !code[want] {
			t.Errorf("transition %s has no route (%s)", transition, want)
		}
	}
}

func TestSection17ERequestsAcceptNoCommercialOrPaymentAuthority(t *testing.T) {
	doc := load(t)

	create := doc.Components.Schemas["CreateOrderRequest"]
	if value, ok := create.AdditionalProperties.(bool); !ok || value {
		t.Fatal("create request must reject unknown fields")
	}
	// Exactly one field: the fingerprint the client commits to. Anything else
	// would be a client asserting authority the server owns.
	if len(create.Properties) != 1 || create.Properties["quote_fingerprint"] == nil {
		t.Fatalf("create request properties=%v", create.Properties)
	}

	transition := doc.Components.Schemas["TransitionRequest"]
	if value, ok := transition.AdditionalProperties.(bool); !ok || value {
		t.Fatal("transition request must reject unknown fields")
	}
	if len(transition.Properties) != 1 || transition.Properties["reason_code"] == nil {
		t.Fatalf("transition request properties=%v", transition.Properties)
	}

	// No request schema may carry money, funding, allocation, vendor identity
	// or a payment reference.
	for name, schema := range doc.Components.Schemas {
		if !strings.HasSuffix(name, "Request") {
			continue
		}
		for field := range schema.Properties {
			lower := strings.ToLower(field)
			for _, forbidden := range []string{
				"minor", "amount", "price", "fee", "tax", "discount", "funding",
				"allocation", "business_id", "branch_id", "customer_id",
				"payment", "settlement", "refund", "status", "state",
			} {
				if strings.Contains(lower, forbidden) {
					t.Errorf("%s accepts server-owned authority %q", name, field)
				}
			}
		}
	}
}

// A mixed multi-vendor outcome must be a declared, reachable state.
func TestSection17EParentStateDeclaresPartialCompletion(t *testing.T) {
	state := load(t).Components.Schemas["ParentOrderState"]
	found := map[string]bool{}
	for _, value := range state.Enum {
		found[value] = true
	}
	for _, want := range []string{
		"CREATED", "IN_PROGRESS", "COMPLETED", "PARTIALLY_COMPLETED", "CANCELLED",
	} {
		if !found[want] {
			t.Errorf("ParentOrderState does not declare %s", want)
		}
	}
}

// The VendorOrder contract must expose its own isolated money, stock,
// fulfilment and prepared-allocation fields, because those are exactly what
// make one vendor's allocation unusable for another.
func TestSection17EVendorOrderDeclaresIsolationEvidence(t *testing.T) {
	vendorOrder := load(t).Components.Schemas["VendorOrder"]
	required := map[string]bool{}
	for _, field := range vendorOrder.Required {
		required[field] = true
	}
	for _, field := range []string{
		"vendor_order_id", "parent_order_id", "business_id", "branch_id",
		"quote_group_ref", "product_subtotal_minor",
		"delivery_responsibility", "delivery_fee_beneficiary",
		"customer_payable_minor", "vendor_payable_minor",
		"reservation_required", "vendor_own_delivery_state", "status",
		"refund_allocated_minor", "settlement_allocated_minor", "version",
	} {
		if !required[field] {
			t.Errorf("VendorOrder does not require %s", field)
		}
	}
}

func TestSection17EParentOrderDeclaresImmutableSnapshotEvidence(t *testing.T) {
	parent := load(t).Components.Schemas["ParentOrder"]
	required := map[string]bool{}
	for _, field := range parent.Required {
		required[field] = true
	}
	for _, field := range []string{
		"parent_order_id", "quote_ref", "quote_version", "quote_fingerprint",
		"checkout_session_id", "cart_ref", "cart_version", "fulfilment_plan_ref",
		"address_snapshot", "has_receiver", "customer_approved_total_minor",
		"max_authorized_total_minor", "funding_breakdown",
		"price_disclosure_snapshot", "state", "vendor_order_count",
		"refund_prepared_minor", "settlement_prepared_minor",
	} {
		if !required[field] {
			t.Errorf("ParentOrder does not require %s", field)
		}
	}
}

// The order line must repeat Section 17D's own online-price disclosure and must
// never be able to claim parity with a vendor's physical-store price.
func TestSection17EOrderLineKeepsOnlinePriceDisclosure(t *testing.T) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `"enum": ["VENDOR_ONLINE_PRICE"]`) {
		t.Error("order line does not pin the online price disclosure")
	}
	if !strings.Contains(text, `"store_price_parity_claimed": { "type": "boolean", "enum": [false] }`) {
		t.Error("order line does not forbid a physical-store parity claim")
	}
}
