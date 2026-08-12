package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	assetsfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	dispatch "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	eligibilityusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	workforcefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce"
)

type section16CandidateSource struct {
	pool         *pgxpool.Pool
	eligibility  *eligibilityusecases.Service
	workforce    *workforcefeature.Service
	assets       *assetsfeature.Service
	restrictions interface {
		Restricted(context.Context, string, string, string) (bool, error)
	}
}

func (a section16CandidateSource) Facts(ctx context.Context, org, branch, sourceType, sourceID string) ([]dispatch.CandidateFact, error) {
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
	out := []dispatch.CandidateFact{}
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
		out = append(out, dispatch.CandidateFact{ProviderID: provider, ProviderMembershipID: membership, VehicleReference: vehicle, ExecutionService: category, MembershipActive: status == "ACTIVE", ProviderActive: status == "ACTIVE", CapabilityMatch: capability, Available: eligible, VehicleEligible: eligible && vehicle != "", CapacityOK: eligible, BranchAllowed: home == "" || home == branch, CargoAllowed: eligible, SafetyAllowed: eligible, ReservationFree: eligible, ActiveWorkFree: free, TrackingFreshness: fresh, SourceVersions: map[string]int64{"membership": version}})
	}
	return out, rows.Err()
}

func BuildSection16Service(cfg config.Config, pool *pgxpool.Pool, publisher *events.DomainPublisher, private *privateusecases.Service, eligibility *eligibilityusecases.Service, workforce *workforcefeature.Service, assets *assetsfeature.Service, restrictions interface {
	Restricted(context.Context, string, string, string) (bool, error)
}) *dispatch.Service {
	var optimizer dispatch.OptimizationPort
	ocfg := cfg.Clients["route-optimization-service"]
	if ocfg.Configured() && !cfg.IntegrationsKillSwitch {
		optimizer = dispatch.NewHTTPOptimizer(ocfg.BaseURL, ocfg.Secret, cfg.HTTPClientTimeout)
	}
	trackingConfigured := cfg.Clients["driver-dispatch-delivery-service"].Configured() || cfg.Clients["driver-hauling-service"].Configured()
	return dispatch.NewService(dispatch.Options{Pool: pool, Repository: dispatch.NewRepository(pool), PrivateAssignments: private, Candidates: section16CandidateSource{pool: pool, eligibility: eligibility, workforce: workforce, assets: assets, restrictions: restrictions}, Optimizer: optimizer, Events: publisher, Flags: dispatch.Flags{DispatchPlanning: cfg.DispatchPlanningEnabled, RoutePlanning: cfg.RoutePlanningEnabled, RouteOptimization: cfg.RouteOptimizationEnabled, RoutePublication: cfg.RoutePublicationEnabled, ControlTower: cfg.ControlTowerEnabled, TrackingProjections: cfg.TrackingProjectionsEnabled, Geofences: cfg.GeofencesEnabled, OperationalAlerts: cfg.OperationalAlertsEnabled, Workers: cfg.Section16WorkersEnabled, KillSwitch: cfg.IntegrationsKillSwitch || cfg.PrivateOperationsKillSwitch, OptimizationProviderConfigured: ocfg.Configured(), TrackingSourceConfigured: trackingConfigured}, CandidateTTL: cfg.Section16CandidateTTL, TrackingFreshAfter: cfg.Section16TrackingFreshAfter, TrackingStaleAfter: cfg.Section16TrackingStaleAfter, TrackingRetention: cfg.Section16TrackingRetention, AlertSLA: cfg.Section16AlertSLA, Now: time.Now})
}
