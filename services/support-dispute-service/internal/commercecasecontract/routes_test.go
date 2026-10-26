package commercecasecontract

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/support-dispute-service/internal/features/admincommands"
	"cosmicforge/logistics/services/support-dispute-service/internal/features/adminfeed"
	"cosmicforge/logistics/services/support-dispute-service/internal/features/adminresources"
	notificationhttp "cosmicforge/logistics/services/support-dispute-service/internal/features/notifications/http"
	supporthttp "cosmicforge/logistics/services/support-dispute-service/internal/features/support/http"
	supportusecases "cosmicforge/logistics/services/support-dispute-service/internal/features/support/usecases"
	"cosmicforge/logistics/shared/go/admincommand"
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
	// Section 16 Secure Admin owner routes, registered exactly as cmd/main.go
	// does. With every switch off they exist but answer 503.
	ownerAdmin, err := adminresources.Runtime(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := admincommand.Runtime(ownerAdmin, admincommands.Spec(), admincommands.Executor{}, adminfeed.Prefix)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := adminfeed.SnapshotFromEnvironment(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ownerAdmin.RegisterOn(router.Group(""))
	commands.Register(router)
	snapshot.Register(router)
	actual := map[string]bool{}
	for _, route := range router.Routes() {
		actual[route.Method+" "+normalize(route.Path)] = true
	}
	if len(actual) != 81 {
		t.Fatalf("Support route count=%d want 81 (73 product + 8 Secure Admin)", len(actual))
	}
	for _, route := range []string{
		"GET /internal/support/v1/admin/resources/{kind}", "GET /internal/support/v1/admin/resources/{kind}/{id}",
		"POST /internal/support/v1/admin/authorization-contexts", "GET /internal/support/v1/admin/authorization-contexts/bindings/{kind}/{id}",
		"POST /internal/support/v1/admin/commands", "GET /internal/support/v1/admin/commands/{kind}/{id}",
		"GET /internal/support/v1/admin/commands/{kind}/{id}/{key}", "GET /internal/support/v1/admin-events/snapshot",
	} {
		if !actual[route] {
			t.Errorf("Secure Admin route missing: %s", route)
		}
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
