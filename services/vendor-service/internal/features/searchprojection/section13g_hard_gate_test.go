package searchprojection

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type section13GProjectionSource struct {
	resolveCount int
}

func (
	source *section13GProjectionSource,
) ResolveIncrementalProjection(
	ctx context.Context,
	event SearchProjectionEvent,
) (
	[]IncrementalProjectionUpdate,
	error,
) {
	source.resolveCount++

	// No concrete document is needed for event-ledger
	// idempotency testing. A legitimate event can resolve to
	// no currently discoverable documents.
	return []IncrementalProjectionUpdate{},
		nil
}

type section13GProjectionApplier struct {
	applyCount int
}

func (
	applier *section13GProjectionApplier,
) ApplyIncrementalProjection(
	ctx context.Context,
	event SearchProjectionEvent,
	updates []IncrementalProjectionUpdate,
) error {
	applier.applyCount++

	return nil
}

type section13GCache struct {
	values map[string][]byte

	getErr error
	setErr error
	delErr error
}

func (
	cache *section13GCache,
) Get(
	ctx context.Context,
	key string,
) (
	[]byte,
	bool,
	error,
) {
	if cache.getErr != nil {
		return nil,
			false,
			cache.getErr
	}

	value, exists :=
		cache.values[key]

	if !exists {
		return nil,
			false,
			nil
	}

	return append(
			[]byte(nil),
			value...,
		),
		true,
		nil
}

func (
	cache *section13GCache,
) Set(
	ctx context.Context,
	key string,
	value []byte,
	ttl time.Duration,
) error {
	if cache.setErr != nil {
		return cache.setErr
	}

	if ttl <= 0 {
		return errors.New(
			"unbounded ttl",
		)
	}

	if cache.values == nil {
		cache.values =
			make(
				map[string][]byte,
			)
	}

	cache.values[key] =
		append(
			[]byte(nil),
			value...,
		)

	return nil
}

func (
	cache *section13GCache,
) Delete(
	ctx context.Context,
	keys ...string,
) error {
	if cache.delErr != nil {
		return cache.delErr
	}

	for _, key := range keys {
		delete(
			cache.values,
			key,
		)
	}

	return nil
}

type section13GRevisionProvider struct {
	revision int64
	err      error
}

func (
	provider *section13GRevisionProvider,
) CurrentSearchCacheRevision(
	ctx context.Context,
	namespace SearchCacheNamespace,
) (
	int64,
	error,
) {
	if provider.err != nil {
		return 0,
			provider.err
	}

	return provider.revision,
		nil
}

type section13GFoodOrigin struct {
	searchCount int
}

func (
	origin *section13GFoodOrigin,
) Search(
	ctx context.Context,
	request FoodEngineSearchRequest,
) (
	FoodEngineSearchResponse,
	error,
) {
	origin.searchCount++

	return FoodEngineSearchResponse{
		ProfileCode: DefaultFoodRankingProfile,

		ProfileVersion: 1,

		Hits: []FoodEngineHit{
			{
				DocumentID: "fresh-food-document",

				FoodItemRef: "food-jollof",

				Name: "Jollof Rice",

				PlacementType: PlacementOrganic,
			},
		},
	}, nil
}

func (
	origin *section13GFoodOrigin,
) Suggest(
	ctx context.Context,
	request SuggestionRequest,
) (
	[]SearchSuggestion,
	error,
) {
	return []SearchSuggestion{
		{
			Text: "Jollof Rice",

			NormalizedText: "jollof rice",

			SourceType: "NAME",
		},
	}, nil
}

func TestSection13GEventIdempotencyAndOrdering(
	t *testing.T,
) {
	dsn :=
		os.Getenv(
			"VENDOR_SERVICE_TEST_DATABASE_URL",
		)

	if dsn == "" {
		t.Skip(
			"VENDOR_SERVICE_TEST_DATABASE_URL not configured",
		)
	}

	ctx, cancel :=
		context.WithTimeout(
			context.Background(),
			2*time.Minute,
		)
	defer cancel()

	pool, err :=
		pgxpool.New(
			ctx,
			dsn,
		)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if _, err =
		pool.Exec(
			ctx,
			`
            TRUNCATE TABLE
                search_projection_event_ledger,
                search_projection_source_checkpoints;

            UPDATE search_cache_revisions
               SET revision=1,
                   updated_at=now();
            `,
		); err != nil {
		t.Fatal(err)
	}

	source :=
		&section13GProjectionSource{}

	applier :=
		&section13GProjectionApplier{}

	processor :=
		NewPostgresIncrementalProjectionProcessor(
			pool,
			source,
			applier,
		)

	event :=
		SearchProjectionEvent{
			EventID: "13g-food-event-2",

			Namespace: SearchCacheNamespaceFood,

			EventType: ProjectionEventFoodItemChanged,

			SourceKind: "food_item",

			SourceRef: "food-jollof",

			SourceRevision: 2,

			Generation: 1,

			Payload: json.RawMessage(
				`{"reason":"catalogue_changed"}`,
			),
		}

	first, err :=
		processor.Process(
			ctx,
			event,
		)
	if err != nil {
		t.Fatal(err)
	}

	if !first.Applied ||
		first.Duplicate ||
		first.Stale {
		t.Fatalf(
			"first result=%+v",
			first,
		)
	}

	if first.CacheRevision != 2 {
		t.Fatalf(
			"cache revision=%d want=2",
			first.CacheRevision,
		)
	}

	duplicate, err :=
		processor.Process(
			ctx,
			event,
		)
	if err != nil {
		t.Fatal(err)
	}

	if !duplicate.Duplicate ||
		duplicate.Applied {
		t.Fatalf(
			"duplicate result=%+v",
			duplicate,
		)
	}

	if applier.applyCount != 1 {
		t.Fatalf(
			"duplicate advanced projection apply count=%d",
			applier.applyCount,
		)
	}

	older :=
		event

	older.EventID =
		"13g-food-event-1"

	older.SourceRevision =
		1

	older.Payload =
		json.RawMessage(
			`{"reason":"late_old_event"}`,
		)

	stale, err :=
		processor.Process(
			ctx,
			older,
		)
	if err != nil {
		t.Fatal(err)
	}

	if !stale.Stale ||
		stale.Applied {
		t.Fatalf(
			"out-of-order result=%+v",
			stale,
		)
	}

	if stale.CacheRevision != 2 {
		t.Fatalf(
			"stale event changed cache revision=%d",
			stale.CacheRevision,
		)
	}

	if applier.applyCount != 1 {
		t.Fatalf(
			"out-of-order event applied projection count=%d",
			applier.applyCount,
		)
	}

	revisionProvider :=
		NewPostgresSearchCacheRevisionProvider(
			pool,
		)

	revision, err :=
		revisionProvider.
			CurrentSearchCacheRevision(
				ctx,
				SearchCacheNamespaceFood,
			)
	if err != nil {
		t.Fatal(err)
	}

	if revision != 2 {
		t.Fatalf(
			"FOOD cache revision=%d want=2",
			revision,
		)
	}
}

func TestSection13GRedisUnavailableFallsBackToOrigin(
	t *testing.T,
) {
	redisDown :=
		errors.New(
			"redis unavailable",
		)

	cache :=
		&section13GCache{
			values: make(
				map[string][]byte,
			),

			getErr: redisDown,

			setErr: redisDown,
		}

	revisions :=
		&section13GRevisionProvider{
			revision: 7,
		}

	origin :=
		&section13GFoodOrigin{}

	engine, err :=
		NewCachedFoodSearchEngine(
			origin,
			cache,
			revisions,
			DefaultSearchCacheTTLConfig(),
		)
	if err != nil {
		t.Fatal(err)
	}

	response, err :=
		engine.Search(
			context.Background(),
			FoodEngineSearchRequest{
				Query: "jollof",

				Generation: 1,

				PageSize: 10,
			},
		)
	if err != nil {
		t.Fatal(err)
	}

	if origin.searchCount != 1 {
		t.Fatalf(
			"origin count=%d want=1",
			origin.searchCount,
		)
	}

	if len(response.Hits) != 1 ||
		response.Hits[0].FoodItemRef !=
			"food-jollof" {
		t.Fatalf(
			"fallback response=%+v",
			response,
		)
	}
}

func TestSection13GStaleRedisRevisionCannotWin(
	t *testing.T,
) {
	cache :=
		&section13GCache{
			values: make(
				map[string][]byte,
			),
		}

	request :=
		FoodEngineSearchRequest{
			Query: "jollof",

			Generation: 1,

			PageSize: 10,
		}

	staleKey, err :=
		buildFoodSearchCacheKey(
			request,
			1,
			SearchCacheClassQuery,
		)
	if err != nil {
		t.Fatal(err)
	}

	staleResponse :=
		FoodEngineSearchResponse{
			Hits: []FoodEngineHit{
				{
					DocumentID: "stale-document",

					FoodItemRef: "stale-food",

					Name: "Stale Result",
				},
			},
		}

	raw, err :=
		json.Marshal(
			staleResponse,
		)
	if err != nil {
		t.Fatal(err)
	}

	cache.values[staleKey] =
		raw

	revisions :=
		&section13GRevisionProvider{
			// PostgreSQL revision already advanced.
			revision: 2,
		}

	origin :=
		&section13GFoodOrigin{}

	engine, err :=
		NewCachedFoodSearchEngine(
			origin,
			cache,
			revisions,
			DefaultSearchCacheTTLConfig(),
		)
	if err != nil {
		t.Fatal(err)
	}

	response, err :=
		engine.Search(
			context.Background(),
			request,
		)
	if err != nil {
		t.Fatal(err)
	}

	if origin.searchCount != 1 {
		t.Fatalf(
			"stale Redis prevented PostgreSQL-origin search count=%d",
			origin.searchCount,
		)
	}

	if len(response.Hits) != 1 ||
		response.Hits[0].DocumentID !=
			"fresh-food-document" {
		t.Fatalf(
			"stale Redis response won: %+v",
			response,
		)
	}
}

func TestSection13GFoodShopKeyIsolation(
	t *testing.T,
) {
	foodKey, err :=
		buildFoodSearchCacheKey(
			FoodEngineSearchRequest{
				Query: "rice",

				Generation: 3,

				PageSize: 20,
			},
			9,
			SearchCacheClassQuery,
		)
	if err != nil {
		t.Fatal(err)
	}

	shopKey, err :=
		buildShopSearchCacheKey(
			ShopEngineSearchRequest{
				Query: "rice",

				Generation: 3,

				PageSize: 20,
			},
			9,
			SearchCacheClassQuery,
		)
	if err != nil {
		t.Fatal(err)
	}

	if foodKey == shopKey {
		t.Fatal(
			"FOOD and SHOP cache keys collided",
		)
	}

	if !strings.HasPrefix(
		foodKey,
		"vendor:search:food:",
	) {
		t.Fatalf(
			"invalid FOOD key=%q",
			foodKey,
		)
	}

	if !strings.HasPrefix(
		shopKey,
		"vendor:search:shop:",
	) {
		t.Fatalf(
			"invalid SHOP key=%q",
			shopKey,
		)
	}
}

func TestSection13GTTLAlwaysBounded(
	t *testing.T,
) {
	config :=
		DefaultSearchCacheTTLConfig()

	if err :=
		config.Validate(); err != nil {
		t.Fatal(err)
	}

	for _, class := range []SearchCacheClass{
		SearchCacheClassQuery,
		SearchCacheClassFacet,
		SearchCacheClassSuggestion,
		SearchCacheClassTaxonomy,
	} {
		ttl, err :=
			config.TTLFor(class)

		if err != nil {
			t.Fatal(err)
		}

		if ttl <= 0 ||
			ttl > 15*time.Minute {
			t.Fatalf(
				"class=%s ttl=%s",
				class,
				ttl,
			)
		}
	}
}
