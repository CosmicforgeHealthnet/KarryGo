package wallethttp

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	walletusecases "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/usecases"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestCommerceCallerAllowlistFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		caller  string
		allowed bool
	}{{"vendor-service", true}, {"driver-dispatch-delivery-service", false}, {"attacker-service", false}, {"", false}} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("service_name", test.caller)
		if got := requireCaller(ctx, "vendor-service"); got != test.allowed {
			t.Fatalf("caller %q allowed=%v", test.caller, got)
		}
		if !test.allowed && !ctx.IsAborted() {
			t.Fatalf("caller %q was not aborted", test.caller)
		}
	}
}

func TestDeliveryReleaseAuthorityIsBoundToAuthenticatedService(t *testing.T) {
	for _, test := range []struct {
		caller, requested, providerType string
		trusted                         bool
	}{
		{"vendor-service", "", "", false},
		{"dispatch-delivery-service", "attacker_type", "dispatch_provider", true},
		{"driver-dispatch-delivery-service", "truck_provider", "dispatch_provider", true},
		{"driver-hauling-service", "dispatch_provider", "truck_provider", true},
	} {
		providerType, trusted := deliveryReleaseAuthority(test.caller, test.requested)
		if providerType != test.providerType || trusted != test.trusted {
			t.Fatalf("caller=%s provider=%s trusted=%v", test.caller, providerType, trusted)
		}
	}
}

func TestCommerceAndVendorFinanceInternalRouteSurface(t *testing.T) {
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/payment-wallet"), &walletusecases.WalletService{}, []byte("customer-secret"), nil, serviceauth.Secrets{"vendor-service": []byte("vendor-secret")})
	want := map[string]bool{
		"POST /api/v1/payment-wallet/internal/commerce/payment-intents":                             true,
		"POST /api/v1/payment-wallet/internal/commerce/allocations/:vendorOrderId/vendor-release":   true,
		"POST /api/v1/payment-wallet/internal/commerce/allocations/:vendorOrderId/delivery-release": true,
		"POST /api/v1/payment-wallet/internal/commerce/allocations/:vendorOrderId/refunds":          true,
		"GET /api/v1/payment-wallet/internal/vendor-finance/:businessId/summary":                    true,
		"POST /api/v1/payment-wallet/internal/vendor-finance/:businessId/withdrawals":               true,
	}
	for _, route := range router.Routes() {
		delete(want, route.Method+" "+route.Path)
	}
	if len(want) > 0 {
		t.Fatalf("missing routes: %v", want)
	}
}

func TestCommerceWalletOpenAPIContractsAreValid(t *testing.T) {
	for _, name := range []string{"commerce-payment-allocation.openapi.json", "vendor-payable-withdrawal.openapi.json"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "openapi", name))
		if err != nil {
			t.Fatal(err)
		}
		var document struct {
			OpenAPI    string         `json:"openapi"`
			Paths      map[string]any `json:"paths"`
			Components struct {
				Schemas map[string]any `json:"schemas"`
			} `json:"components"`
		}
		if err = json.Unmarshal(raw, &document); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if document.OpenAPI == "" || len(document.Paths) == 0 {
			t.Fatalf("%s is incomplete", name)
		}
		if name == "commerce-payment-allocation.openapi.json" {
			if _, ok := document.Components.Schemas["CommerceDeliveryReleaseRequest"]; !ok {
				t.Fatal("commerce delivery release request contract is missing")
			}
			if _, ok := document.Components.Schemas["CommerceAllocationRefundRequest"]; !ok {
				t.Fatal("commerce allocation refund request contract is missing")
			}
		}
	}
}
