package section17dcontract

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

const (
	commerceBase = "/api/v1/commerce"
	specPath     = "../../docs/openapi/section17d-immutable-checkout-quote.openapi.json"
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

func codeRoutes(t *testing.T) map[string]bool {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	checkoutquote.RegisterCustomerRoutes(
		router.Group(commerceBase), &checkoutquote.Service{},
		sharedauth.NewTokenSigner([]byte("contract")), idempotency.NewStore(nil, time.Hour), true,
	)
	out := map[string]bool{}
	for _, route := range router.Routes() {
		out[key(route.Method, route.Path)] = true
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

func TestSection17DRoutesEqualTheContract(t *testing.T) {
	code, spec := codeRoutes(t), specRoutes(t)
	if len(code) != 3 {
		t.Fatalf("Section 17D routes=%d want 3: %v", len(code), sorted(code))
	}
	if strings.Join(sorted(code), "\n") != strings.Join(sorted(spec), "\n") {
		t.Fatalf("code/spec drift\ncode=%v\nspec=%v", sorted(code), sorted(spec))
	}
	for route := range code {
		lower := strings.ToLower(route)
		for _, forbidden := range []string{"payment", "wallet", "capture", "authorize", "orders", "refund"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("later-section route leaked into 17D: %s", route)
			}
		}
	}
}

func TestSection17DRequestsAcceptNoCommercialOverride(t *testing.T) {
	doc := load(t)
	create := doc.Components.Schemas["CreateQuoteRequest"]
	approve := doc.Components.Schemas["ApproveQuoteRequest"]
	if value, ok := create.AdditionalProperties.(bool); !ok || value {
		t.Fatal("create request must reject unknown fields")
	}
	if value, ok := approve.AdditionalProperties.(bool); !ok || value {
		t.Fatal("approval request must reject unknown fields")
	}
	if len(create.Properties) != 2 || create.Properties["fulfilment_plan_id"] == nil || create.Properties["coupon_code"] == nil {
		t.Fatalf("create request properties=%v", create.Properties)
	}
	for field := range create.Properties {
		lower := strings.ToLower(field)
		for _, forbidden := range []string{"amount", "minor", "price", "discount", "funding", "sponsor", "campaign_id", "business_id", "branch_id"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("create request accepts commercial authority %q", field)
			}
		}
	}
	if len(approve.Properties) != 1 || approve.Properties["quote_fingerprint"] == nil {
		t.Fatalf("approval request properties=%v", approve.Properties)
	}
}

func TestSection17DQuoteDeclaresRequiredAgreementEvidence(t *testing.T) {
	quote := load(t).Components.Schemas["Quote"]
	required := map[string]bool{}
	for _, field := range quote.Required {
		required[field] = true
	}
	for _, field := range []string{
		"quote_id", "quote_version", "quote_fingerprint", "cart_ref", "cart_version", "customer_ref",
		"address_snapshot", "expires_at", "quoted_total_minor", "max_authorized_total_minor",
		"selected_fulfilment_mode", "product_subtotal_minor", "delivery_subtotal_minor",
		"fee_subtotal_minor", "tax_subtotal_minor", "discount_total_minor", "final_payable_minor",
		"funding_breakdown", "price_disclosure_snapshot", "vendor_branch_groups", "lines", "logistics_quotes",
	} {
		if !required[field] {
			t.Errorf("Quote does not require %s", field)
		}
	}
}
