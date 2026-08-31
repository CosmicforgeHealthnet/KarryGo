package missionplanning

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

type contractStore struct{}

func (contractStore) Create(context.Context, PlanRequest, Plan, string) (Plan, error) {
	return Plan{}, nil
}
func (contractStore) Replan(context.Context, string, PlanRequest, Plan, string) (Plan, error) {
	return Plan{}, nil
}
func (contractStore) Read(context.Context, string) (Plan, error)                { return Plan{}, nil }
func (contractStore) ReadBySource(context.Context, string, int64) (Plan, error) { return Plan{}, nil }
func (contractStore) ActivateManifestCustody(context.Context, string, CustodyCommand, string) error {
	return nil
}
func (contractStore) ReleaseManifestCustody(context.Context, string, CustodyCommand, string) error {
	return nil
}
func TestPlanningRoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	Register(engine.Group("/api/v1/internal"), NewHandler(New(contractStore{}, NewPlanner(), true)), serviceauth.NewVerifier(serviceauth.Secrets{"vendor-service": []byte("secret")}, time.Minute), nil, []string{"vendor-service"})
	actual := map[string]bool{}
	for _, r := range engine.Routes() {
		actual[r.Method+" "+r.Path] = true
	}
	for _, route := range []string{"POST /api/v1/internal/dispatch/v1/delivery-plans", "GET /api/v1/internal/dispatch/v1/delivery-plans/by-source/:sourceRef", "GET /api/v1/internal/dispatch/v1/delivery-plans/:groupRef", "POST /api/v1/internal/dispatch/v1/delivery-plans/:groupRef/replans", "POST /api/v1/internal/dispatch/v1/delivery-plans/manifests/:manifestRef/custody/activate", "POST /api/v1/internal/dispatch/v1/delivery-plans/manifests/:manifestRef/custody/release"} {
		if !actual[route] {
			t.Errorf("missing route %s", route)
		}
	}
}
