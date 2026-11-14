package app

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	assetsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
	assetsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/repositories"
	assetsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/usecases"
	fuelrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/fuel/repositories"
	fuelusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/fuel/usecases"
	inspectionsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/inspections/repositories"
	inspectionsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/inspections/usecases"
	maintenancerepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/maintenance/repositories"
	maintenanceusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/maintenance/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// Section18Options configure the Section 18 features (assets, inspections,
// maintenance, fuel), which share one flag set and one event pipeline.
type Section18Options struct {
	Pool         *pgxpool.Pool
	Events       assetsusecases.EventPublisher
	Audit        *audit.Recorder
	Media        platformclients.MediaContract
	Alerts       assetsusecases.AlertSink
	Restrictions assetsusecases.RestrictionGate
	Queue        *jobs.Queue
	Flags        assetsmodels.Flags
	Now          func() time.Time
}

// Section18Services are the wired Section 18 use cases.
type Section18Services struct {
	Assets      *assetsusecases.Service
	Inspections *inspectionsusecases.Service
	Maintenance *maintenanceusecases.Service
	Fuel        *fuelusecases.Service
}

// BuildSection18Services wires the four Section 18 features. Each depends on
// the others only through ports declared in its own use cases: maintenance
// and inspections compose the asset safety primitives and the maintenance
// work-order writer into their transactions; assets reaches maintenance and
// inspections for repair work orders and the section's maintenance job.
func BuildSection18Services(o Section18Options) Section18Services {
	var events maintenanceusecases.EventPublisher
	if o.Events != nil {
		events = o.Events
	}
	assets := assetsusecases.NewService(assetsusecases.Options{Pool: o.Pool, Repository: assetsrepos.NewRepository(o.Pool), Events: o.Events, Audit: o.Audit, Media: o.Media, Alerts: o.Alerts, Restrictions: o.Restrictions, Queue: o.Queue, Flags: o.Flags, Now: o.Now})
	maintenance := maintenanceusecases.NewService(maintenanceusecases.Options{Pool: o.Pool, Repository: maintenancerepos.NewRepository(o.Pool), Assets: assets, Inspections: inspectionsrepos.NewRepository(o.Pool), Events: events, Audit: o.Audit, Restrictions: o.Restrictions, Flags: o.Flags, Now: o.Now})
	inspections := inspectionsusecases.NewService(inspectionsusecases.Options{Pool: o.Pool, Repository: inspectionsrepos.NewRepository(o.Pool), Assets: assets, WorkOrders: maintenance, Events: events, Audit: o.Audit, Flags: o.Flags, Now: o.Now})
	fuel := fuelusecases.NewService(fuelusecases.Options{Pool: o.Pool, Repository: fuelrepos.NewRepository(o.Pool), Assets: assets, Events: events, Audit: o.Audit, Flags: o.Flags, Now: o.Now})
	assets.SetCollaborators(assetsusecases.Collaborators{WorkOrders: maintenance, DueSchedules: maintenance, SafetyDeclines: inspections})
	out := Section18Services{Assets: assets, Inspections: inspections, Maintenance: maintenance, Fuel: fuel}
	if o.Alerts != nil {
		out.SetAlertSink(o.Alerts)
	}
	return out
}

// SetAlertSink routes the Section 18 system alerts (critical inspections,
// defects, breakdowns, overdue maintenance, reorder levels) to one sink.
func (s Section18Services) SetAlertSink(a assetsusecases.AlertSink) {
	s.Assets.SetAlertSink(a)
	s.Inspections.SetAlertSink(a)
	s.Maintenance.SetAlertSink(a)
}
