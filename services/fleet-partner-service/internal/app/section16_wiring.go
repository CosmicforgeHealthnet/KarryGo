package app

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	dispatchhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/http"
	routeshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/routes/http"
	trackinghttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/tracking/http"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	assetsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/usecases"
	dispatchmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	dispatchrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/repositories"
	dispatchusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/usecases"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	eligibilityusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	routesclients "cosmicforge/logistics/services/fleet-partner-service/internal/features/routes/clients"
	routesrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/routes/repositories"
	routesusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/routes/usecases"
	trackingrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/tracking/repositories"
	trackingusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/tracking/usecases"
	workforceusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type section16CandidateSource struct {
	pool         *pgxpool.Pool
	eligibility  *eligibilityusecases.Service
	workforce    *workforceusecases.Service
	assets       *assetsusecases.Service
	restrictions interface {
		Restricted(context.Context, string, string, string) (bool, error)
	}
}

func (a section16CandidateSource) Facts(ctx context.Context, org, branch, sourceType, sourceID string) ([]dispatchmodels.CandidateFact, error) {
	if a.eligibility == nil {
		return nil, errors.New("eligibility service unavailable")
	}
	service, vehicle := "DISPATCH", ""
	if sourceType == "ROUTE" {
		if e := a.pool.QueryRow(ctx, `SELECT execution_service,vehicle_or_truck_reference FROM fleet_partner_route_plans WHERE organization_id=$1 AND id=$2 AND branch_id=$3`, org, sourceID, branch).Scan(&service, &vehicle); e != nil {
			return nil, e
		}
	}
	rows, e := a.pool.Query(ctx, `SELECT m.id,COALESCE(m.provider_id,''),m.service_category,m.status,COALESCE(m.home_branch_id,''),m.version,EXISTS(SELECT 1 FROM fleet_partner_provider_capabilities c WHERE c.organization_id=m.organization_id AND c.membership_id=m.id AND c.status='ACTIVE' AND (c.valid_until IS NULL OR c.valid_until>now())),NOT EXISTS(SELECT 1 FROM fleet_partner_private_assignments pa WHERE pa.organization_id=m.organization_id AND pa.provider_membership_id=m.id AND pa.status IN('ACCEPTED','READY_FOR_EXECUTION','IN_PROGRESS')) FROM fleet_partner_organization_provider_memberships m WHERE m.organization_id=$1 AND m.service_category=$2 AND m.status IN('ACTIVE','SUSPENDED') ORDER BY m.id`, org, service)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []dispatchmodels.CandidateFact{}
	for rows.Next() {
		var membership, provider, category, status, home string
		var version int64
		var capability, free bool
		if e = rows.Scan(&membership, &provider, &category, &status, &home, &version, &capability, &free); e != nil {
			return nil, e
		}
		decision, x := a.eligibility.Evaluate(ctx, eligibilityusecases.EvaluateInput{Actor: eligibilityusecases.Actor{OrganizationID: org, ActorType: eligibilitymodels.ActorSystem, ActorID: "section16-candidate-gate"}, OperationType: eligibilitymodels.OperationAssignment, SubjectType: eligibilitymodels.SubjectProvider, SubjectID: membership, MembershipID: membership, ServiceCategory: service, AssetReference: vehicle, CorrelationID: sourceID})
		eligible := x == nil && decision != nil && decision.Status == eligibilitymodels.StatusEligible
		if eligible {
			if a.restrictions == nil {
				eligible = false
			} else {
				for _, subject := range [][2]string{{"ORGANIZATION", org}, {"PROVIDER", membership}, {"ASSET", vehicle}} {
					if subject[1] == "" {
						continue
					}
					restricted, gateErr := a.restrictions.Restricted(ctx, org, subject[0], subject[1])
					if gateErr != nil || restricted {
						eligible = false
						break
					}
				}
			}
		}
		if eligible && a.workforce != nil {
			v, gateErr := a.workforce.Evaluate(ctx, org, membership, service, "", time.Now())
			eligible = gateErr == nil && v.Eligible
		}
		if eligible && vehicle != "" && a.assets != nil {
			v, gateErr := a.assets.Evaluate(ctx, org, vehicle, membership, "", 0, service, time.Now())
			eligible = gateErr == nil && v.Eligible
		}
		fresh := "UNKNOWN"
		if eligible {
			fresh = "FRESH"
		}
		out = append(out, dispatchmodels.CandidateFact{ProviderID: provider, ProviderMembershipID: membership, VehicleReference: vehicle, ExecutionService: category, MembershipActive: status == "ACTIVE", ProviderActive: status == "ACTIVE", CapabilityMatch: capability, Available: eligible, VehicleEligible: eligible && vehicle != "", CapacityOK: eligible, BranchAllowed: home == "" || home == branch, CargoAllowed: eligible, SafetyAllowed: eligible, ReservationFree: eligible, ActiveWorkFree: free, TrackingFreshness: fresh, SourceVersions: map[string]int64{"membership": version}})
	}
	return out, rows.Err()
}

// Section16 is the Section 16 composition: dispatch planning, route
// planning and tracking projections, wired to each other through their
// ports. It registers every Section 16 worker and runs the maintenance sweep.
type Section16 struct {
	Dispatch *dispatchusecases.Service
	Routes   *routesusecases.Service
	Tracking *trackingusecases.Service
}

// section16 returns the Section 16 composition, or an empty one when the
// components were built without it.
func (c *Components) section16() Section16 {
	if c.DispatchControl == nil {
		return Section16{}
	}
	return *c.DispatchControl
}

// RegisterTenantRoutes mounts the corporate dispatch, route-plan and
// tracking routes.
func (s Section16) RegisterTenantRoutes(g *gin.RouterGroup, a *accesscontrol.Authorizer, idem *idempotency.Store) {
	dispatchhttp.NewHandler(s.Dispatch, a, idem).RegisterTenantRoutes(g)
	routeshttp.NewHandler(s.Routes, a, idem).RegisterTenantRoutes(g)
	trackinghttp.NewHandler(s.Tracking, a).RegisterTenantRoutes(g)
}

// RegisterWorkers binds every Section 16 job to the durable worker.
func (s *Section16) RegisterWorkers(w *jobs.Worker) {
	s.Dispatch.RegisterWorkers(w)
	s.Routes.RegisterWorkers(w)
	s.Tracking.RegisterWorkers(w)
}

// RunMaintenance runs the periodic Section 16 sweep.
func (s *Section16) RunMaintenance(ctx context.Context) error { return s.Dispatch.RunMaintenance(ctx) }

// Section16Options configures NewSection16.
type Section16Options struct {
	Pool                                                                              *pgxpool.Pool
	PrivateAssignments                                                                routesusecases.PrivateAssignmentCreator
	Rescue                                                                            dispatchusecases.RescueAssignmentPort
	Candidates                                                                        section16Candidates
	Optimizer                                                                         routesusecases.OptimizationPort
	Events                                                                            *events.DomainPublisher
	Audit                                                                             *audit.Recorder
	Queue                                                                             *jobs.Queue
	Flags                                                                             dispatchmodels.Flags
	CandidateTTL, TrackingFreshAfter, TrackingStaleAfter, TrackingRetention, AlertSLA time.Duration
	Now                                                                               func() time.Time
}

// section16Candidates is the candidate source shared by dispatch and routes.
type section16Candidates interface {
	Facts(context.Context, string, string, string, string) ([]dispatchmodels.CandidateFact, error)
}

// section16Alerts lets tracking raise alerts through dispatch, which is
// constructed after it.
type section16Alerts struct{ dispatch *dispatchusecases.Service }

func (a *section16Alerts) RaiseAlert(ctx context.Context, tx pgx.Tx, t tenant.Context, req dispatchmodels.AlertRequest) error {
	return a.dispatch.RaiseAlert(ctx, tx, t, req)
}

// section16Recorder lets routes record audit and outbox entries through
// dispatch, which is constructed after it.
type section16Recorder struct{ dispatch *dispatchusecases.Service }

func (r *section16Recorder) Record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, version int64, payload any) error {
	return r.dispatch.Record(ctx, tx, t, event, kind, id, version, payload)
}

// section16Plans lets routes resolve dispatch plans.
type section16Plans struct{ dispatch *dispatchusecases.Service }

func (p *section16Plans) PlanBranch(ctx context.Context, q database.Querier, org, planID string) (string, error) {
	return p.dispatch.PlanBranch(ctx, q, org, planID)
}

// NewSection16 wires the dispatch, routes and tracking services together.
func NewSection16(o Section16Options) *Section16 {
	alerts, recorder, plans := &section16Alerts{}, &section16Recorder{}, &section16Plans{}
	var events dispatchusecases.EventPublisher
	var routeEvents routesusecases.EventPublisher
	var trackingEvents trackingusecases.EventPublisher
	if o.Events != nil {
		events, routeEvents, trackingEvents = o.Events, o.Events, o.Events
	}
	var candidates dispatchusecases.CandidateSource
	var routeCandidates routesusecases.CandidateSource
	if o.Candidates != nil {
		candidates, routeCandidates = o.Candidates, o.Candidates
	}
	routes := routesusecases.NewService(routesusecases.Options{Pool: o.Pool, Repository: routesrepos.NewRepository(o.Pool), Recorder: recorder, Events: routeEvents, PrivateAssignments: o.PrivateAssignments, Candidates: routeCandidates, Plans: plans, Optimizer: o.Optimizer, Queue: o.Queue, Flags: o.Flags, CandidateTTL: o.CandidateTTL, Now: o.Now})
	tracking := trackingusecases.NewService(trackingusecases.Options{Pool: o.Pool, Repository: trackingrepos.NewRepository(o.Pool), Events: trackingEvents, Alerts: alerts, Routes: routes, Queue: o.Queue, Flags: o.Flags, TrackingFreshAfter: o.TrackingFreshAfter, TrackingStaleAfter: o.TrackingStaleAfter, TrackingRetention: o.TrackingRetention, Now: o.Now})
	dispatch := dispatchusecases.NewService(dispatchusecases.Options{Pool: o.Pool, Repository: dispatchrepos.NewRepository(o.Pool), Routes: routes, Tracking: tracking, Rescue: o.Rescue, Candidates: candidates, Events: events, Audit: o.Audit, Queue: o.Queue, Flags: o.Flags, CandidateTTL: o.CandidateTTL, TrackingFreshAfter: o.TrackingFreshAfter, TrackingStaleAfter: o.TrackingStaleAfter, AlertSLA: o.AlertSLA, Now: o.Now})
	alerts.dispatch, recorder.dispatch, plans.dispatch = dispatch, dispatch, dispatch
	return &Section16{Dispatch: dispatch, Routes: routes, Tracking: tracking}
}

func BuildSection16Service(cfg config.Config, pool *pgxpool.Pool, publisher *events.DomainPublisher, private *privateusecases.Service, eligibility *eligibilityusecases.Service, workforce *workforceusecases.Service, assets *assetsusecases.Service, restrictions interface {
	Restricted(context.Context, string, string, string) (bool, error)
}) *Section16 {
	var optimizer routesusecases.OptimizationPort
	ocfg := cfg.Clients["route-optimization-service"]
	if ocfg.Configured() && !cfg.IntegrationsKillSwitch {
		optimizer = routesclients.NewHTTPOptimizer(ocfg.BaseURL, ocfg.Secret, cfg.HTTPClientTimeout)
	}
	trackingConfigured := cfg.Clients["driver-dispatch-delivery-service"].Configured() || cfg.Clients["driver-hauling-service"].Configured()
	o := Section16Options{Pool: pool, Candidates: section16CandidateSource{pool: pool, eligibility: eligibility, workforce: workforce, assets: assets, restrictions: restrictions}, Optimizer: optimizer, Events: publisher, Flags: dispatchmodels.Flags{DispatchPlanning: cfg.DispatchPlanningEnabled, RoutePlanning: cfg.RoutePlanningEnabled, RouteOptimization: cfg.RouteOptimizationEnabled, RoutePublication: cfg.RoutePublicationEnabled, ControlTower: cfg.ControlTowerEnabled, TrackingProjections: cfg.TrackingProjectionsEnabled, Geofences: cfg.GeofencesEnabled, OperationalAlerts: cfg.OperationalAlertsEnabled, Workers: cfg.Section16WorkersEnabled, KillSwitch: cfg.IntegrationsKillSwitch || cfg.PrivateOperationsKillSwitch, OptimizationProviderConfigured: ocfg.Configured(), TrackingSourceConfigured: trackingConfigured}, CandidateTTL: cfg.Section16CandidateTTL, TrackingFreshAfter: cfg.Section16TrackingFreshAfter, TrackingStaleAfter: cfg.Section16TrackingStaleAfter, TrackingRetention: cfg.Section16TrackingRetention, AlertSLA: cfg.Section16AlertSLA, Now: time.Now}
	if private != nil {
		o.PrivateAssignments, o.Rescue = private, private
	}
	return NewSection16(o)
}

// BuildMarketplaceExpiryAlertSink returns the Section 16 operational alert
// service, configured only with what raising an alert needs, for the
// marketplace assignment expiry worker (FLEET-11.11c). The worker runs in its
// own loop without the full composition root, so it gets the same alert
// service, flags, SLA and escalation queue rather than a second alerting
// mechanism.
func BuildMarketplaceExpiryAlertSink(cfg config.Config, pool *pgxpool.Pool) *dispatchusecases.Service {
	return dispatchusecases.NewService(dispatchusecases.Options{
		Pool:       pool,
		Repository: dispatchrepos.NewRepository(pool),
		Events:     events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Flags: dispatchmodels.Flags{
			OperationalAlerts: cfg.OperationalAlertsEnabled,
			Workers:           cfg.Section16WorkersEnabled,
			KillSwitch:        cfg.IntegrationsKillSwitch || cfg.PrivateOperationsKillSwitch,
		},
		AlertSLA: cfg.Section16AlertSLA,
		Now:      time.Now,
	})
}
