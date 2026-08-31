package serviceability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"github.com/gin-gonic/gin"
)

func TestRouterMatchesOpenAPI(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	RegisterRoutes(engine, sharedauth.NewTokenSigner([]byte("test-secret")), NewHandler(New(Options{})))
	if len(engine.Routes()) != 1 || engine.Routes()[0].Method != "POST" || engine.Routes()[0].Path != "/api/v1/customer/dispatch/serviceability" {
		t.Fatalf("routes=%v", engine.Routes())
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "openapi", "customer-dispatch-serviceability.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if _, ok := document.Paths["/api/v1/customer/dispatch/serviceability"]["post"]; !ok {
		t.Fatal("OpenAPI operation missing")
	}
}
