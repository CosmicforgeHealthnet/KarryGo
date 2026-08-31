package main

// Section 13 composition root.
//
// Canonical taxonomy and Food/Shop search are the largest wiring in the
// service: two HTTP surfaces per feature, a layered search stack, a ranking
// store, a projection administration service and a health reader. Assembling
// that inline would double the length of main.go's Register block, so it is
// built here and main.go asks for it in one call - the same shape adapters.go
// already uses for Section 5-12 ports.
//
// The one thing worth reading closely is buildSearchStack: the order the
// layers are composed in is a correctness property, not a preference.

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
	"cosmicforge/logistics/services/vendor-service/internal/features/taxonomy"
	"cosmicforge/logistics/services/vendor-service/internal/jobs"
	"cosmicforge/logistics/services/vendor-service/internal/platform/ratelimit"
	"cosmicforge/logistics/services/vendor-service/internal/platform/servicenonce"
	"cosmicforge/logistics/services/vendor-service/internal/searchrebuild"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// section13 is everything Section 13 contributes to the running service.
type section13 struct {
	taxonomyFlags   taxonomy.Flags
	taxonomyHandler *taxonomy.Handler
	taxonomyAdmin   *taxonomy.InternalHandler

	searchFlags   searchprojection.Flags
	searchHandler *searchprojection.InternalHandler

	// workers drive the Section 13H rebuild pipeline. They are only built when
	// search administration is enabled, because a rebuild worker with nothing
	// able to enqueue a rebuild is a timer that never fires.
	workers []jobs.Job
}

// taxonomyScopeAdapter implements taxonomy.ScopeResolver over the Section 5
// authorization evaluator.
//
// Section 13 asks the same question every other feature does - at which scope
// is this permission held - because a mapping belongs to a branch's catalogue
// item and a branch-scoped grant must never reclassify another branch's menu.
type taxonomyScopeAdapter struct {
	evaluator *staffusecases.AuthorizationEvaluator
}

func (a taxonomyScopeAdapter) ResolveScope(
	ctx context.Context,
	staffID string,
	permission string,
) (taxonomy.Scope, error) {
	grants, err := a.evaluator.EffectivePermissions(ctx, staffID)
	if err != nil {
		return taxonomy.Scope{}, err
	}
	var scope taxonomy.Scope
	for _, grant := range grants {
		if grant.PermissionCode != permission {
			continue
		}
		if grant.BranchID == nil {
			// A vendor-wide grant subsumes every branch-scoped one.
			return taxonomy.Scope{Global: true}, nil
		}
		scope.BranchIDs = append(scope.BranchIDs, *grant.BranchID)
	}
	return scope, nil
}

// buildSearchStack composes the search read path.
//
// The layering is, from the outside in:
//
//	GenerationAware -> Cached -> Engine -> PostgreSQL backend
//
// and each layer has to sit exactly where it does:
//
//   - GenerationAware is outermost because it resolves which generation is
//     ACTIVE. Nothing below it may choose a generation, and callers cannot
//     supply one.
//   - Cached sits below it so the resolved generation is part of the cache
//     key. Cache above generation resolution would let a key survive a
//     generation switch and serve documents from a retired projection.
//   - Engine resolves the ranking profile and scores. Ranking below the cache
//     means a profile change is picked up on the next cache miss rather than
//     silently applying to already-cached result pages - and the cache
//     revision that a ranking change bumps is what forces those misses.
//   - The PostgreSQL backend is the only layer that touches durable state, and
//     it is the one that stays correct when Redis is gone.
func buildSearchStack(
	db *pgxpool.Pool,
	redisClient redis.UniversalClient,
	generations *searchprojection.PostgresSearchGenerationStore,
) (*searchprojection.GenerationAwareFoodSearch, *searchprojection.GenerationAwareShopSearch, searchprojection.SearchCache, error) {
	foodEngine := searchprojection.NewFoodSearchEngine(
		searchprojection.NewPostgresFoodSearchBackend(db),
	)
	shopEngine := searchprojection.NewShopSearchEngine(
		searchprojection.NewPostgresShopSearchBackend(db),
	)

	cache := searchprojection.SearchCache(searchprojection.NewRedisSearchCache(redisClient))
	revisions := searchprojection.NewPostgresSearchCacheRevisionProvider(db)
	ttl := searchprojection.DefaultSearchCacheTTLConfig()

	cachedFood, err := searchprojection.NewCachedFoodSearchEngine(foodEngine, cache, revisions, ttl)
	if err != nil {
		return nil, nil, nil, err
	}
	cachedShop, err := searchprojection.NewCachedShopSearchEngine(shopEngine, cache, revisions, ttl)
	if err != nil {
		return nil, nil, nil, err
	}

	return searchprojection.NewGenerationAwareFoodSearch(cachedFood, generations),
		searchprojection.NewGenerationAwareShopSearch(cachedShop, generations),
		cache,
		nil
}

// searchProjectionAdminAuthorizer is the authorization port the projection
// administration service requires.
//
// It returns nil unconditionally, and that is correct rather than a stub: the
// only way to reach this service is through the root-mounted internal surface,
// which has already verified an HMAC-v2 signature, checked the caller against
// the search allowlist, and consumed the request nonce. Re-deriving a vendor
// staff permission here would be meaningless - there is no vendor staff member
// on an internal service-to-service call, and inventing one would be the exact
// confusion Section 13I exists to prevent. The authority check lives at the
// door; this records why there is nothing left to check behind it.
type searchProjectionAdminAuthorizer struct{}

func (searchProjectionAdminAuthorizer) RequireSearchProjectionAdmin(ctx context.Context) error {
	return nil
}

// buildSection13 assembles the Section 13 surfaces.
func buildSection13(
	cfg config.Config,
	db *pgxpool.Pool,
	redisClient redis.UniversalClient,
	evaluator *staffusecases.AuthorizationEvaluator,
	outboxRepo *events.OutboxRepository,
	instanceID string,
) (section13, error) {
	taxonomyFlags := taxonomy.Flags{
		TaxonomyEnabled: cfg.Flags.VendorTaxonomyEnabled,
		// The commerce kill switch can only ever withhold a write.
		WritesEnabled: cfg.Flags.VendorTaxonomyWritesEnabled && !cfg.Flags.CommerceGlobalKillSwitch,
	}
	searchFlags := searchprojection.Flags{
		SearchEnabled: cfg.Flags.VendorSearchEnabled,
		AdminEnabled:  cfg.Flags.VendorSearchAdminEnabled,
	}

	nonces := servicenonce.NewStore(db)

	taxonomyRepo := taxonomy.NewRepository(db)
	taxonomyService := taxonomy.NewService(taxonomyRepo, events.NewDomainPublisher(outboxRepo))
	taxonomyAccess := taxonomy.NewVendorAccess(db, taxonomyScopeAdapter{evaluator: evaluator})

	generations := searchprojection.NewPostgresSearchGenerationStore(db)
	foodSearch, shopSearch, cache, err := buildSearchStack(db, redisClient, generations)
	if err != nil {
		return section13{}, err
	}

	// The rebuild pipeline only runs where administration is enabled. Search
	// itself keeps answering either way: standing the pipeline down freezes
	// rebuilds, it does not take discovery offline.
	var workers []jobs.Job
	if searchFlags.SearchEnabled && searchFlags.AdminEnabled {
		if workers, err = buildProjectionWorkers(db, generations, instanceID); err != nil {
			return section13{}, err
		}
	}

	return section13{
		workers:         workers,
		taxonomyFlags:   taxonomyFlags,
		taxonomyHandler: taxonomy.NewHandler(taxonomyService, taxonomyAccess, taxonomyFlags),
		taxonomyAdmin: taxonomy.NewInternalHandler(
			taxonomyService,
			splitCallers(cfg.TaxonomyInternalCallers),
			nonces,
			taxonomyFlags,
		),
		searchFlags: searchFlags,
		searchHandler: searchprojection.NewInternalHandler(searchprojection.InternalHandlerOptions{
			Food:    foodSearch,
			Shop:    shopSearch,
			Ranking: searchprojection.NewPostgresRankingProfileStore(db),
			Admin: searchprojection.NewSearchProjectionAdminService(
				searchProjectionAdminAuthorizer{},
				generations,
			),
			Health:  searchprojection.NewPostgresProjectionHealthReader(db, cache),
			Callers: splitCallers(cfg.SearchInternalCallers),
			Nonces:  nonces,
			Flags:   searchFlags,
			Limiter: ratelimit.New(redisClient, cfg.AbuseProtection.RedisKeyPrefix),
			Limit: searchprojection.RequestLimitPolicy{
				Max:    cfg.AbuseProtection.SearchMax,
				Window: time.Duration(cfg.AbuseProtection.SearchWindowSeconds) * time.Second,
			},
		}),
	}, nil
}

// splitCallers turns a comma-separated allowlist into a slice, dropping blanks
// so a trailing comma cannot register an empty caller name.
func splitCallers(raw string) []string {
	var out []string
	for _, caller := range strings.Split(raw, ",") {
		if caller = strings.TrimSpace(caller); caller != "" {
			out = append(out, caller)
		}
	}
	return out
}

// registerVendorRoutes mounts the authenticated vendor taxonomy surface.
func (s section13) registerVendorRoutes(group *gin.RouterGroup) {
	taxonomy.RegisterRoutes(group, s.taxonomyHandler, s.taxonomyFlags)
}

// registerInternalRoutes mounts both root-mounted internal surfaces. They are
// separate registrations with separate allowlists: a service cleared to run a
// search query is not thereby cleared to publish taxonomy.
func (s section13) registerInternalRoutes(router *gin.Engine, verifier *serviceauth.Verifier) {
	taxonomy.RegisterInternalRoutes(router, s.taxonomyAdmin, verifier, s.taxonomyFlags)
	searchprojection.RegisterInternalSearchRoutes(router, s.searchHandler, verifier, s.searchFlags)
}

// buildProjectionWorkers assembles the two Section 13H workers that turn a
// requested rebuild into an ACTIVE generation.
//
// They are two workers rather than one because ProjectionWorker takes a single
// handler for the job types it claims, and the two stages are genuinely
// different work: FULL_REBUILD streams the authoritative catalogue for minutes,
// GENERATION_VALIDATE_ACTIVATE performs one short guarded transition. Giving
// them separate lease durations and claim batches means a long rebuild holding
// its lease never delays the activation of a generation that has already
// finished.
//
// Both are safe to run on every instance: jobs are claimed under a lease with
// SKIP LOCKED, so two workers never take the same job, and a worker that dies
// lets its lease lapse for another to adopt with its checkpoint intact.
func buildProjectionWorkers(
	db *pgxpool.Pool,
	generations *searchprojection.PostgresSearchGenerationStore,
	instanceID string,
) ([]jobs.Job, error) {
	// The rebuild reads the authoritative Food and Shop domains. It never
	// reads the search projection it is rebuilding, and never reads Redis.
	source := searchrebuild.NewSource(db)

	applier := searchprojection.NewDefaultSearchRebuildApplier(
		searchprojection.NewFoodProjector(searchprojection.NewPostgresFoodSearchBackend(db)),
		searchprojection.NewShopProjector(searchprojection.NewPostgresShopSearchBackend(db)),
		searchprojection.NewPostgresFoodSearchBackend(db),
		searchprojection.NewPostgresShopSearchBackend(db),
	)

	rebuildWorker, err := searchprojection.NewProjectionWorker(
		generations,
		"vendor-search-rebuild-"+instanceID,
		[]searchprojection.ProjectionJobType{
			searchprojection.ProjectionJobFullRebuild,
			searchprojection.ProjectionJobScopedRebuild,
		},
		searchprojection.NewFullRebuildJobHandler(source, applier, generations),
		// One rebuild at a time per worker: a rebuild pass streams an entire
		// catalogue, and claiming several at once would hold several leases
		// while making progress on one.
		1,
		2*time.Minute,
	)
	if err != nil {
		return nil, err
	}

	activateWorker, err := searchprojection.NewProjectionWorker(
		generations,
		"vendor-search-activate-"+instanceID,
		[]searchprojection.ProjectionJobType{
			searchprojection.ProjectionJobValidateActivate,
		},
		searchprojection.NewGenerationValidateActivateHandler(generations),
		5,
		30*time.Second,
	)
	if err != nil {
		return nil, err
	}

	return []jobs.Job{
		jobs.SearchProjectionWorkerJob(
			"search-projection-rebuild",
			searchRebuildPollInterval,
			rebuildWorker,
		),
		jobs.SearchProjectionWorkerJob(
			"search-projection-validate-activate",
			searchActivatePollInterval,
			activateWorker,
		),
	}, nil
}

// Poll intervals. Rebuilds are rare and long, so polling for one often buys
// nothing; activation is short and is what an operator is waiting on, so it is
// checked more frequently.
const (
	searchRebuildPollInterval  = 10 * time.Second
	searchActivatePollInterval = 5 * time.Second
)

// Workers returns the Section 13 jobs for the service runtime to start.
func (s section13) Workers() []jobs.Job {
	return s.workers
}
