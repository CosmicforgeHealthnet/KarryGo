package searchprojection

// Section 13I projection health.
//
// The single most important thing this file gets right is that cache health
// and search health are two different questions.
//
// Redis is a latency cache in front of the PostgreSQL projection, and every
// engine reads through to PostgreSQL when the cache misses or is unreachable.
// So "Redis is down" means search is slower; it does not mean search is wrong
// or unavailable. Folding the two into one status would hand an operator a red
// light on the wrong component during an incident and invite them to fail over
// a search tier that is answering correctly.
//
// Everything reported here is a counter, a status or a timestamp. No projected
// document, no cached value, no catalogue payload, no customer data and no
// credential material passes through this endpoint.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// defaultFoodFacets and defaultShopFacets are the facet families a caller gets
// when it asks for facets without naming any. Both lists sit inside
// maxEngineFacets, so the default can never trip the engine's own bound.
func defaultFoodFacets() []string {
	return []string{"PROVIDER_TYPE", "CUISINE", "DISH_TYPE", "MEAL_PERIOD", "DIETARY", "AVAILABILITY"}
}

func defaultShopFacets() []string {
	return []string{"CATEGORY", "BRAND", "MEASUREMENT", "SELL_UNIT", "AVAILABILITY"}
}

// formatTime renders an optional timestamp as RFC3339, or omits it.
func formatTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}

// NamespaceHealth is the bounded operational picture for one search namespace.
type NamespaceHealth struct {
	NamespaceCode SearchCacheNamespace `json:"namespace_code"`

	ActiveGeneration int64                  `json:"active_generation"`
	ActiveStatus     SearchGenerationStatus `json:"active_status,omitempty"`

	// CacheRevision is the identity every cache key for this namespace is
	// built from. A cached entry stamped with an older revision can never be
	// served, which is what makes a stale Redis harmless.
	CacheRevision int64 `json:"cache_revision"`

	DocumentCount  int64 `json:"document_count"`
	ProcessedCount int64 `json:"processed_count"`

	BuildingGenerations int64 `json:"building_generations"`
	ReadyGenerations    int64 `json:"ready_generations"`
	FailedGenerations   int64 `json:"failed_generations"`

	PendingJobs    int64 `json:"pending_jobs"`
	FailedJobs     int64 `json:"failed_jobs"`
	DeadLetterJobs int64 `json:"dead_letter_jobs"`

	ActivatedAt *string `json:"activated_at,omitempty"`
}

// PostgresProjectionHealthReader answers health from PostgreSQL, and reports
// cache reachability separately.
type PostgresProjectionHealthReader struct {
	pool  *pgxpool.Pool
	cache SearchCache
}

func NewPostgresProjectionHealthReader(pool *pgxpool.Pool, cache SearchCache) *PostgresProjectionHealthReader {
	return &PostgresProjectionHealthReader{pool: pool, cache: cache}
}

// GenerationHealth reports the namespace's active generation and the bounded
// backlog counters around it.
func (reader *PostgresProjectionHealthReader) GenerationHealth(
	ctx context.Context,
	namespace SearchCacheNamespace,
) (NamespaceHealth, error) {
	if reader == nil || reader.pool == nil || !validSearchNamespace(namespace) {
		return NamespaceHealth{}, ErrInvalidArgument
	}

	health := NamespaceHealth{NamespaceCode: namespace}

	var (
		activeStatus string
		activatedAt  *time.Time
	)
	err := reader.pool.QueryRow(
		ctx,
		`SELECT generation, status, document_count, processed_count, activated_at
           FROM search_projection_generations
          WHERE namespace_code=$1
            AND status='ACTIVE'`,
		string(namespace),
	).Scan(
		&health.ActiveGeneration,
		&activeStatus,
		&health.DocumentCount,
		&health.ProcessedCount,
		&activatedAt,
	)
	switch {
	case err == pgx.ErrNoRows:
		// A namespace with no ACTIVE generation is a real, reportable state
		// (nothing has been built and activated yet), not a failure.
		health.ActiveGeneration = 0
	case err != nil:
		return NamespaceHealth{}, err
	default:
		health.ActiveStatus = SearchGenerationStatus(activeStatus)
		health.ActivatedAt = formatTime(activatedAt)
	}

	if err = reader.pool.QueryRow(
		ctx,
		`SELECT
             count(*) FILTER (WHERE status='BUILDING'),
             count(*) FILTER (WHERE status='READY'),
             count(*) FILTER (WHERE status='FAILED')
           FROM search_projection_generations
          WHERE namespace_code=$1`,
		string(namespace),
	).Scan(
		&health.BuildingGenerations,
		&health.ReadyGenerations,
		&health.FailedGenerations,
	); err != nil {
		return NamespaceHealth{}, err
	}

	if err = reader.pool.QueryRow(
		ctx,
		`SELECT
             count(*) FILTER (WHERE status='PENDING'),
             count(*) FILTER (WHERE status='FAILED'),
             count(*) FILTER (WHERE status='DEAD_LETTER')
           FROM search_projection_jobs
          WHERE namespace_code=$1`,
		string(namespace),
	).Scan(
		&health.PendingJobs,
		&health.FailedJobs,
		&health.DeadLetterJobs,
	); err != nil {
		return NamespaceHealth{}, err
	}

	if err = reader.pool.QueryRow(
		ctx,
		`SELECT revision FROM search_cache_revisions WHERE namespace_code=$1`,
		string(namespace),
	).Scan(&health.CacheRevision); err != nil && err != pgx.ErrNoRows {
		return NamespaceHealth{}, err
	}

	return health, nil
}

// CacheAvailable probes the cache without failing the health report.
//
// It returns a boolean rather than an error precisely because an unreachable
// cache is not an error condition for search: the caller records it as a
// degraded-cache signal beside a search tier that is still correct.
func (reader *PostgresProjectionHealthReader) CacheAvailable(ctx context.Context) bool {
	if reader == nil || reader.cache == nil {
		return false
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// A miss is a healthy answer: it proves the cache responded. Only a
	// transport error means unavailable.
	_, _, err := reader.cache.Get(probeCtx, "vendor:search:health:probe")
	return err == nil
}
