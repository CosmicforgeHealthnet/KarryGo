package finance_projection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFinanceRoutesAreCapabilityNamedAndBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/vendor"), NewHandler(nil), true)
	want := map[string]bool{
		"GET /api/v1/vendor/finance/overview":               true,
		"GET /api/v1/vendor/finance/transactions":           true,
		"GET /api/v1/vendor/finance/bank-accounts":          true,
		"GET /api/v1/vendor/finance/withdrawals":            true,
		"GET /api/v1/vendor/finance/withdrawals/:reference": true,
		"POST /api/v1/vendor/finance/withdrawals":           true,
	}
	for _, route := range router.Routes() {
		delete(want, route.Method+" "+route.Path)
	}
	if len(want) != 0 {
		t.Fatalf("missing routes: %v", want)
	}
}

func TestVendorFinanceOpenAPIIsValid(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "openapi", "vendor-finance.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		OpenAPI string         `json:"openapi"`
		Paths   map[string]any `json:"paths"`
	}
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if document.OpenAPI == "" || len(document.Paths) != 5 {
		t.Fatalf("unexpected Vendor Finance contract: %+v", document)
	}
}
