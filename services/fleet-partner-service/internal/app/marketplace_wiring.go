package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	eligibilityusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// BuildMarketplaceService is the one composition root for HTTP ingestion,
// reconciliation, and the common Section 26 stream consumer. Sharing the
// instance shape prevents stream delivery from falling back to a duplicate
// generic request/trip projection.
func BuildMarketplaceService(cfg config.Config, pool *pgxpool.Pool, metrics *observability.Metrics) *mktusecases.Service {
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	executions := platformclients.BuildExecutionSet(platformclients.Options{
		ServiceName: config.ServiceName, BaseURL: cfg.DispatchServiceURL,
		Secret: cfg.DispatchServiceSecret, Timeout: cfg.DispatchServiceTimeout, Metrics: metrics,
	})
	if cfg.HaulingIntegrationEnabled && !cfg.IntegrationsKillSwitch {
		hauling := cfg.Clients["driver-hauling-service"]
		executions = platformclients.BuildExecutionSet(platformclients.Options{
			ServiceName: config.ServiceName, BaseURL: cfg.DispatchServiceURL,
			Secret: cfg.DispatchServiceSecret, Timeout: cfg.DispatchServiceTimeout, Metrics: metrics,
		}, platformclients.Options{
			ServiceName: config.ServiceName, BaseURL: hauling.BaseURL,
			Secret: hauling.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics,
		})
	}
	capabilities := cfg.ExecutionCapabilities()
	return mktusecases.NewService(mktusecases.Options{
		Pool: pool, Projections: mktrepos.NewProjectionRepository(pool), Offers: mktrepos.NewOfferRepository(pool),
		Assignments: mktrepos.NewAssignmentRepository(pool), ExecutionStarts: mktrepos.NewExecutionStartRepository(pool),
		Trips: mktrepos.NewTripRepository(pool), Commands: mktrepos.NewCommandRepository(pool), Executions: executions,
		Eligibility:          mktusecases.NewEligibilityChecker(NewMembershipReader(providerrepos.NewRepository(pool)), executions, nil),
		DurableEligibility:   eligibilityusecases.Build(cfg, pool, metrics),
		PrivateAcceptance:    NewPrivateAcceptanceReader(pool),
		PrivateEvents:        NewPrivateExecutionEventProjector(pool, publisher),
		Events:               publisher,
		Metrics:              metrics,
		CandidateMemberships: NewCandidateMembershipLister(providerrepos.NewRepository(pool)),
		MembershipHistory:    NewMembershipHistoryReader(providerrepos.NewRepository(pool)),
		Flags: mktusecases.Flags{ExecutionStartEnabled: capabilities.Any(), EnabledSourceServices: map[string]bool{
			mktmodels.SourceDispatch: capabilities.Dispatch,
			mktmodels.SourceTaxi:     false,
			mktmodels.SourceHauling:  capabilities.Hauling,
		},
			ReassignmentEnabled:          cfg.MarketplaceReassignmentEnabled,
			AutomaticReassignmentEnabled: cfg.MarketplaceAutomaticReassignmentEnabled,
		},
	})
}
