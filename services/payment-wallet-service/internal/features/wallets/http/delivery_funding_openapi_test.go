package wallethttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestDeliveryFundingRoutesEqualOpenAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	RegisterRoutes(router.Group("/api/v1/payment-wallet"), nil, []byte("customer"), nil, serviceauth.Secrets{"vendor-service": []byte("secret")})
	code := map[string]bool{}
	for _, route := range router.Routes() {
		if strings.Contains(route.Path, "/internal/commerce/delivery-funding") {
			path := strings.ReplaceAll(route.Path, ":sourceReference", "{sourceReference}")
			code[route.Method+" "+path] = true
		}
	}
	raw, err := os.ReadFile("../../../../docs/openapi/commerce-delivery-funding.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for path, methods := range spec.Paths {
		for method := range methods {
			if method == "get" || method == "post" {
				documented[strings.ToUpper(method)+" "+path] = true
			}
		}
	}
	if len(code) != 2 || len(documented) != 2 {
		t.Fatalf("delivery funding route count code=%v spec=%v", code, documented)
	}
	for route := range code {
		if !documented[route] {
			t.Fatalf("route missing from OpenAPI: %s", route)
		}
	}
}

func TestDeliveryFundingRejectsForeignHMACCaller(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	router.POST("/funding", func(c *gin.Context) {
		c.Set("service_name", "order-service")
		NewHandler(nil).ApplyDeliveryFunding(c)
	})
	body := bytes.NewBufferString(`{"source_reference":"source","vendor_order_id":"order","sponsoring_vendor_id":"vendor","provider_type":"dispatch","provider_id":"provider","currency":"NGN","authoritative_delivery_minor":1,"customer_contribution_minor":1,"vendor_subsidy_minor":0,"platform_subsidy_minor":0,"idempotency_key":"key"}`)
	request := httptest.NewRequest(http.MethodPost, "/funding", body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("foreign caller status=%d body=%s", response.Code, response.Body.String())
	}
}
