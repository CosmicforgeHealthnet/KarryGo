package providerassignments

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	"cosmicforge/logistics/shared/go/fleetproviderwork"
)

// Hauling mounts two different corporate surfaces on the same provider group:
// the private-assignment relay (Register) and the shared Section 10 company
// -assignment facade (RegisterFleetWork). They once claimed the same path,
// and because gin panics when two handlers are registered for one route, the
// whole service died during boot — every deployment, not only corporate ones.
// Nothing in Hauling's own suite noticed, because nothing built the router the
// way cmd/main.go does. This test does.
func TestBothCorporateSurfacesMountWithoutCollision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	authService := providerauthusecases.NewAuthService(providerauthusecases.Options{
		AccessTokenSecret: []byte("provider-token-secret")})

	engine := gin.New()
	group := engine.Group("/api/v1/hauling")

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("registering Hauling's provider corporate surfaces panicked: %v", recovered)
		}
	}()

	Register(group, authService, NewClient("http://127.0.0.1:1", "driver-hauling-service",
		[]byte("secret"), time.Second))
	RegisterFleetWork(group, authService,
		newFleetWorkClientForTest(),
		nil, true)

	// Both families must survive registration as distinct resources: the
	// relay proxies Fleet's private-assignments, the facade serves corporate
	// company-assignments. Losing either one silently is the other way this
	// regression could come back.
	want := map[string]string{
		http.MethodGet:  "/api/v1/hauling/provider/private-assignments/:assignmentId",
		http.MethodPost: "/api/v1/hauling/provider/company-assignments/:assignmentId/accept",
	}
	registered := map[string]bool{}
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for method, path := range want {
		if !registered[method+" "+path] {
			t.Fatalf("expected route %s %s to be registered", method, path)
		}
	}
	if registered[http.MethodGet+" /api/v1/hauling/provider/company-assignments"] == false {
		t.Fatal("expected the corporate company-assignment LIST route to be registered")
	}

	// A smoke request proves the relay still answers on its new path rather
	// than 404ing, which is what a half-applied rename would look like.
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/v1/hauling/provider/private-assignments/pas_1", nil))
	if response.Code == http.StatusNotFound {
		t.Fatal("the private-assignment relay is no longer routable")
	}
}

// newFleetWorkClientForTest returns whatever the shared constructor yields for
// an unconfigured Fleet. Route registration must not depend on Fleet being
// reachable — that is the whole point of the facade being mounted eagerly.
func newFleetWorkClientForTest() *fleetproviderwork.Client {
	client, _ := fleetproviderwork.NewClient(fleetproviderwork.Config{})
	return client
}
