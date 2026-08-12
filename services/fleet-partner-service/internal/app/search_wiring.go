package app

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	searchrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/repositories"
	searchusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// BuildSearchService assembles the Section 25 search projection service.
//
// It is always constructible. With the enablement flag off its route still
// mounts but answers not-found, matching the established Section 15
// convention: an absent capability must be observable, not indistinguishable
// from a deployment that never had it.
func BuildSearchService(cfg config.Config, pool *pgxpool.Pool, authorizer *accesscontrol.Authorizer,
	metrics *observability.Metrics) *searchusecases.Service {
	return searchusecases.NewService(searchusecases.Options{
		Pool: pool, Repository: searchrepos.NewRepository(pool), Authorizer: authorizer, Metrics: metrics,
		Flags: searchusecases.Flags{Enabled: cfg.SearchEnabled},
	})
}
