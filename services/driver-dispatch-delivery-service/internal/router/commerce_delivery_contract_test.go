package router_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercemission"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercequote"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestCommerceDeliveryRouterMatchesOpenAPI(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{dispatchcommerce.SignatureCaller: []byte("test-secret")}, time.Minute)
	nonces := memoryNonceClaimer{}
	quoteHandler := commercequote.NewMultiStopHandler(commercequote.NewMultiStop(commercequote.MultiStopOptions{}))
	missionHandler := commercemission.NewMultiStopMissionHandler(commercemission.NewMultiStopMission(commercemission.MultiStopMissionOptions{}))
	operationalHandler := commercemission.NewOperationalHandler(commercemission.NewOperationalService(nil, []byte("handover-secret"), nil))
	commercequote.RegisterMultiStop(engine.Group("/api/v1/internal"), quoteHandler, verifier, nonces, []string{dispatchcommerce.SignatureCaller})
	commercemission.RegisterMultiStopMissions(engine.Group("/api/v1/internal"), missionHandler, verifier, nonces, []string{dispatchcommerce.SignatureCaller})
	commercemission.RegisterProviderPickupRoutes(engine, authusecases.NewTokenUsecase([]byte("test-access"), time.Hour, time.Hour), missionHandler)
	commercemission.RegisterProviderOperationalRoutes(engine, authusecases.NewTokenUsecase([]byte("test-access"), time.Hour, time.Hour), operationalHandler)
	commercemission.RegisterInternalOperationalRoutes(engine.Group("/api/v1/internal"), operationalHandler, verifier, nonces, []string{dispatchcommerce.SignatureCaller})

	actual := make([]string, 0, len(engine.Routes()))
	for _, route := range engine.Routes() {
		actual = append(actual, route.Method+" "+openAPIPath(route.Path))
	}
	sort.Strings(actual)

	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "openapi", "commerce-dispatch-delivery.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	var documented []string
	for path, operations := range document.Paths {
		for method := range operations {
			documented = append(documented, stringUpper(method)+" "+path)
		}
	}
	sort.Strings(documented)
	if len(actual) != 20 {
		t.Fatalf("commerce route count = %d, want 20: %v", len(actual), actual)
	}
	if len(documented) != len(actual) {
		t.Fatalf("OpenAPI routes = %d, router routes = %d", len(documented), len(actual))
	}
	for index := range actual {
		if actual[index] != documented[index] {
			t.Fatalf("route mismatch at %d: router=%q OpenAPI=%q", index, actual[index], documented[index])
		}
	}
}

type memoryNonceClaimer struct{}

func (memoryNonceClaimer) Claim(_ context.Context, _, _, _ string, _ time.Time) error { return nil }

var _ authority.NonceClaimer = memoryNonceClaimer{}

func stringUpper(value string) string {
	if value == "get" {
		return "GET"
	}
	return "POST"
}

func openAPIPath(path string) string {
	parts := strings.Split(path, "/")
	for index, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[index] = "{" + strings.TrimPrefix(part, ":") + "}"
		}
	}
	return strings.Join(parts, "/")
}
