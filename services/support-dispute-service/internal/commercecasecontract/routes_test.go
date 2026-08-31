package commercecasecontract

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	notificationhttp "cosmicforge/logistics/services/support-dispute-service/internal/features/notifications/http"
	supporthttp "cosmicforge/logistics/services/support-dispute-service/internal/features/support/http"
	supportusecases "cosmicforge/logistics/services/support-dispute-service/internal/features/support/usecases"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestSupportRouteCountAndCommerceOpenAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/support-disputes")
	providers := []supporthttp.ProviderGroup{{Prefix: "/provider", Secret: []byte("a"), Role: "truck_provider", Service: "hauling"}, {Prefix: "/taxi-provider", Secret: []byte("b"), Role: "taxi_provider", Service: "taxi"}, {Prefix: "/dispatch-provider", Secret: []byte("c"), Role: "dispatch_provider", Service: "dispatch"}}
	secrets := serviceauth.Secrets{"vendor-service": []byte("v"), "fleet-partner-service": []byte("f"), "admin-backoffice-service": []byte("a")}
	service := supportusecases.NewSupportService(nil, supportusecases.Options{})
	supporthttp.RegisterRoutes(group, service, []byte("customer"), providers, secrets)
	notificationhttp.RegisterNotificationRoutes(group, notifications.Client{BaseURL: "http://notification.invalid"}, []byte("customer"), providers)
	actual := map[string]bool{}
	for _, route := range router.Routes() {
		actual[route.Method+" "+normalize(route.Path)] = true
	}
	if len(actual) != 73 {
		t.Fatalf("Support route count=%d want 73", len(actual))
	}
	raw, err := os.ReadFile("../../docs/commerce-support-cases.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	operations := 0
	for path, methods := range doc.Paths {
		for method := range methods {
			operations++
			key := strings.ToUpper(method) + " " + path
			if !actual[key] {
				t.Errorf("documented Commerce Support route missing: %s", key)
			}
		}
	}
	if operations != 4 {
		t.Fatalf("Commerce Support OpenAPI operations=%d want 4", operations)
	}
}
func normalize(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			parts[i] = "{" + strings.TrimPrefix(p, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}
