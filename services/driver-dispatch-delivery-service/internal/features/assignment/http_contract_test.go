package assignment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestAssignmentRoutesMatchOpenAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	h := NewHandler(New(&memoryStore{}, nil, Policy{}))
	RegisterInternal(engine.Group("/api/v1/internal"), h, serviceauth.NewVerifier(serviceauth.Secrets{"vendor-service": []byte("secret")}, time.Minute), nil, []string{"vendor-service"})
	RegisterProvider(engine, authusecases.NewTokenUsecase([]byte("secret"), time.Hour, time.Hour), h)
	RegisterCustomer(engine, sharedauth.NewTokenSigner([]byte("customer-secret")), h)
	RegisterAdminInternal(engine.Group("/api/v1/internal"), h, serviceauth.NewVerifier(serviceauth.Secrets{"admin-backoffice-service": []byte("secret")}, time.Minute), nil, []string{"admin-backoffice-service"})
	RegisterPartnerInternal(engine.Group("/api/v1/internal"), h, serviceauth.NewVerifier(serviceauth.Secrets{"api-gateway": []byte("secret")}, time.Minute), nil, []string{"api-gateway"})
	actual := map[string]bool{}
	for _, r := range engine.Routes() {
		actual[r.Method+" "+r.Path] = true
	}
	for _, route := range []string{"POST /api/v1/internal/dispatch/v1/assignments", "GET /api/v1/provider/assignment-offers", "POST /api/v1/provider/assignment-offers/:offerId/accept", "POST /api/v1/provider/assignment-offers/:offerId/decline", "POST /api/v1/customer/dispatch/missions/:missionId/queue-options", "POST /api/v1/internal/dispatch/v1/admin-assisted-assignments", "POST /api/v1/internal/dispatch/v1/partner/missions/:missionId/queue-options"} {
		if !actual[route] {
			t.Errorf("missing route %s", route)
		}
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "openapi", "assignment-queue-multi-vendor-planning.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("openapi invalid: %v", err)
	}
}
