package integration_test

import (
	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	assetshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/http"
	assetsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/usecases"
	fuelhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/fuel/http"
	fuelusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/fuel/usecases"
	inspectionshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/inspections/http"
	inspectionsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/inspections/usecases"
	maintenancehttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/maintenance/http"
	maintenanceusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/maintenance/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
)

// The Section 18 features are wired by app.BuildSection18Services exactly as
// in production. section18Svc exposes the four wired use cases behind one
// value so a scenario can drive assets, inspections, maintenance and fuel
// together.
type (
	assetsSvc      = assetsusecases.Service
	inspectionsSvc = inspectionsusecases.Service
	maintenanceSvc = maintenanceusecases.Service
	fuelSvc        = fuelusecases.Service
)

type section18Svc struct {
	*assetsSvc
	*inspectionsSvc
	*maintenanceSvc
	*fuelSvc
}

func newSection18(o app.Section18Options) *section18Svc {
	s := app.BuildSection18Services(o)
	return &section18Svc{s.Assets, s.Inspections, s.Maintenance, s.Fuel}
}

// SetAlertSink routes every Section 18 feature's alerts to one sink.
func (s *section18Svc) SetAlertSink(a assetsusecases.AlertSink) {
	app.Section18Services{Assets: s.assetsSvc, Inspections: s.inspectionsSvc, Maintenance: s.maintenanceSvc, Fuel: s.fuelSvc}.SetAlertSink(a)
}

// registerSection18 mounts the four Section 18 route sets.
func registerSection18(s *section18Svc, a *accesscontrol.Authorizer, i *idempotency.Store, g *gin.RouterGroup) {
	assetshttp.NewHandler(s.assetsSvc, a, i).RegisterTenantRoutes(g)
	inspectionshttp.NewHandler(s.inspectionsSvc, a, i).RegisterTenantRoutes(g)
	maintenancehttp.NewHandler(s.maintenanceSvc, a, i).RegisterTenantRoutes(g)
	fuelhttp.NewHandler(s.fuelSvc, a, i).RegisterTenantRoutes(g)
}
