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

	// Dispatch is the primary execution source and is always wired: its
	// URL and secret come from the top-level dispatch configuration because
	// every deployment that runs the marketplace must be able to reach it.
	dispatchOpts := platformclients.Options{
		ServiceName: config.ServiceName,
		BaseURL:     cfg.DispatchServiceURL,
		Secret:      cfg.DispatchServiceSecret,
		Timeout:     cfg.DispatchServiceTimeout,
		Metrics:     metrics,
	}
	executions := platformclients.BuildExecutionSet(dispatchOpts)

	// Hauling is optional: when the integration is enabled and the client
	// is configured, it joins the execution set. When it is not, execution
	// calls for hauling requests fail closed with "source unavailable"
	// rather than silently being routed to dispatch.
	if cfg.HaulingIntegrationEnabled && !cfg.IntegrationsKillSwitch {
		hauling := cfg.Clients["driver-hauling-service"]
		if hauling.Configured() {
			executions = platformclients.BuildExecutionSet(dispatchOpts, platformclients.Options{
				ServiceName: config.ServiceName,
				BaseURL:     hauling.BaseURL,
				Secret:      hauling.Secret,
				Timeout:     cfg.HTTPClientTimeout,
				Metrics:     metrics,
			})
		}
	}

	capabilities := cfg.ExecutionCapabilities()

	// The marketplace flags. Two distinct classes of gate live here:
	//
	//   - ROLLOUT gates (ProjectionEnabled, OrganizationAcceptEnabled,
	//     ProviderDirectAcceptEnabled, AssignmentEnabled, ExecutionStartEnabled)
	//     decide whether a capability exists at all in this deployment.
	//     They are on by default because the marketplace has shipped and
	//     every read path already depends on projection ingestion. Turning
	//     any of them off would silently break the read surface.
	//
	//   - POLICY gates (ReassignmentEnabled, AutomaticReassignmentEnabled)
	//     are operational choices, not rollout state. They stay
	//     config-driven so a deployment can change them without a build.
	//
	// Authorization is a separate layer entirely: every corporate write
	// route checks a marketplace.* permission at the HTTP boundary, and
	// every provider write route is HMAC-authenticated by the owning
	// service. A flag being on does not authorize anyone; it only makes
	// the capability reachable to callers who are already authorized.
	flags := mktusecases.Flags{
		// Projection ingestion: the read side of the whole feature. Without
		// it, no offer ever reaches a Fleet organization. Always on.
		ProjectionEnabled: true,

		// Organization-side writes. The dispatcher accepts an offer on
		// behalf of the organization, then assigns it to a provider.
		// Gated by marketplace.accept and marketplace.assign at the HTTP
		// layer; this is the feature-level switch, not the authorization
		// boundary.
		OrganizationAcceptEnabled: true,
		AssignmentEnabled:         true,

		// Provider-direct acceptance (the DELEGATED operating mode). A
		// provider may accept an offer without a Fleet dispatcher first.
		// Served by the internal HMAC routes; the owning execution service
		// authenticates the provider before relaying. On by default
		// because DELEGATED is one of the three operating modes the
		// organization can select, and an operator who selects it must
		// find it working.
		ProviderDirectAcceptEnabled: true,

		// Live execution calls. The flag says "the marketplace may ask a
		// source service to create a canonical execution"; the per-source
		// map below says which sources are actually reachable. Dispatch
		// and hauling follow their own capabilities; taxi is explicitly
		// off because its integration is not yet implemented in this
		// composition root.
		ExecutionStartEnabled: capabilities.Any(),
		EnabledSourceServices: map[string]bool{
			mktmodels.SourceDispatch: capabilities.Dispatch,
			mktmodels.SourceTaxi:     false,
			mktmodels.SourceHauling:  capabilities.Hauling,
		},

		// Operational policy. Config-driven, never hardcoded: an operator
		// must be able to turn automatic reassignment off without a
		// deployment.
		ReassignmentEnabled:          cfg.MarketplaceReassignmentEnabled,
		AutomaticReassignmentEnabled: cfg.MarketplaceAutomaticReassignmentEnabled,
	}

	return mktusecases.NewService(mktusecases.Options{
		Pool:            pool,
		Projections:     mktrepos.NewProjectionRepository(pool),
		Offers:          mktrepos.NewOfferRepository(pool),
		Assignments:     mktrepos.NewAssignmentRepository(pool),
		ExecutionStarts: mktrepos.NewExecutionStartRepository(pool),
		Trips:           mktrepos.NewTripRepository(pool),
		Commands:        mktrepos.NewCommandRepository(pool),
		Executions:      executions,
		Eligibility: mktusecases.NewEligibilityChecker(
			NewMembershipReader(providerrepos.NewRepository(pool)),
			executions,
			nil,
		),
		DurableEligibility:   eligibilityusecases.Build(cfg, pool, metrics),
		PrivateAcceptance:    NewPrivateAcceptanceReader(pool),
		PrivateEvents:        NewPrivateExecutionEventProjector(pool, publisher),
		Events:               publisher,
		Metrics:              metrics,
		CandidateMemberships: NewCandidateMembershipLister(providerrepos.NewRepository(pool)),
		MembershipHistory:    NewMembershipHistoryReader(providerrepos.NewRepository(pool)),
		Flags:                flags,
	})
}
