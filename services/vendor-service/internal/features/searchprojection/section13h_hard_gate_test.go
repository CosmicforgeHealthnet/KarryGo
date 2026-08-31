package searchprojection

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type section13HGenerationProvider struct {
	generation int64
}

func (
	provider *section13HGenerationProvider,
) CurrentActiveGeneration(
	ctx context.Context,
	namespace SearchCacheNamespace,
) (
	int64,
	error,
) {
	return provider.generation,
		nil
}

type section13HFoodOrigin struct {
	lastGeneration int64
}

func (
	origin *section13HFoodOrigin,
) Search(
	ctx context.Context,
	request FoodEngineSearchRequest,
) (
	FoodEngineSearchResponse,
	error,
) {
	origin.lastGeneration =
		request.Generation

	return FoodEngineSearchResponse{},
		nil
}

func (
	origin *section13HFoodOrigin,
) Suggest(
	ctx context.Context,
	request SuggestionRequest,
) (
	[]SearchSuggestion,
	error,
) {
	origin.lastGeneration =
		request.Generation

	return nil, nil
}

func TestSection13HQueriesUseActiveGeneration(
	t *testing.T,
) {
	provider :=
		&section13HGenerationProvider{
			generation: 4,
		}

	origin :=
		&section13HFoodOrigin{}

	engine :=
		NewGenerationAwareFoodSearch(
			origin,
			provider,
		)

	_, err :=
		engine.Search(
			context.Background(),
			FoodEngineSearchRequest{
				Query: "jollof",

				// Caller value must not override active generation.
				Generation: 999,
			},
		)
	if err != nil {
		t.Fatal(err)
	}

	if origin.lastGeneration !=
		4 {
		t.Fatalf(
			"search used generation=%d want=4",
			origin.lastGeneration,
		)
	}
}

func TestSection13HFailureRecoveryAndAtomicActivation(
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
			3*time.Minute,
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
                search_projection_jobs,
                search_projection_generation_history,
                search_projection_generations
            CASCADE;

            TRUNCATE TABLE
                food_search_documents
            CASCADE;

            INSERT INTO search_projection_generations (
                namespace_code,
                generation,
                status,
                base_generation,
                rebuild_scope,
                requested_by,
                started_at,
                completed_at,
                validated_at,
                activated_at,
                document_count,
                processed_count
            )
            VALUES (
                'FOOD',
                4,
                'ACTIVE',
                0,
                'FULL',
                'test',
                now(),
                now(),
                now(),
                now(),
                1,
                1
            );

            UPDATE search_cache_revisions
               SET revision=1
             WHERE namespace_code='FOOD';
            `,
		); err != nil {
		t.Fatal(err)
	}

	store :=
		NewPostgresSearchGenerationStore(
			pool,
		)

	generation5, err :=
		store.RequestFullRebuild(
			ctx,
			SearchCacheNamespaceFood,
			"admin-test",
		)
	if err != nil {
		t.Fatal(err)
	}

	if generation5.Generation != 5 ||
		generation5.BaseGeneration != 4 ||
		generation5.Status !=
			SearchGenerationBuilding {
		t.Fatalf(
			"generation5=%+v",
			generation5,
		)
	}

	// Simulate halfway build failure.
	if err =
		store.RecordGenerationProgress(
			ctx,
			SearchCacheNamespaceFood,
			5,
			1,
			"worker-a",
			nil,
		); err != nil {
		t.Fatal(err)
	}

	if err =
		store.MarkGenerationFailed(
			ctx,
			SearchCacheNamespaceFood,
			5,
			"SIMULATED_FAILURE",
			"halfway failure",
			"worker-a",
		); err != nil {
		t.Fatal(err)
	}

	active, err :=
		store.CurrentActiveGeneration(
			ctx,
			SearchCacheNamespaceFood,
		)
	if err != nil {
		t.Fatal(err)
	}

	if active != 4 {
		t.Fatalf(
			"failed generation damaged active generation: %d",
			active,
		)
	}

	// Retry/replay starts generation 5 cleanly.
	if err =
		store.PrepareFailedGenerationRetry(
			ctx,
			SearchCacheNamespaceFood,
			5,
		); err != nil {
		t.Fatal(err)
	}

	// Create one reconstructed generation-5 document directly.
	// Final Section 13 hard gates additionally exercise projectors.
	if _, err =
		pool.Exec(
			ctx,
			`
            INSERT INTO food_search_documents (
                generation,
                document_id,
                business_ref,
                branch_ref,
                food_item_ref,
                menu_refs,
                name,
                normalized_name,
                search_tokens,
                aliases,
                synonyms,
                canonical_taxonomy,
                provider_types,
                cuisine_terms,
                meal_periods,
                dish_types,
                food_groups,
                occasions,
                dietary_claims,
                preference_claims,
                active,
                availability_summary,
                effective_price_summary,
                serviceability_hint,
                trust_hint,
                reliability_hint,
                promotion_indicator,
                catalogue_revision,
                projection_revision,
                search_text
            )
            VALUES (
                5,
                'generation-5-food',
                'business',
                'branch',
                'food',
                '{}',
                'Jollof Rice',
                'jollof rice',
                ARRAY['jollof','rice'],
                '{}',
                '{}',
                '{}'::jsonb,
                '{}',
                '{}',
                '{}',
                '{}',
                '{}',
                '{}',
                '{}',
                '{}',
                TRUE,
                '{}'::jsonb,
                '{}'::jsonb,
                '{}'::jsonb,
                '{}'::jsonb,
                '{}'::jsonb,
                FALSE,
                1,
                1,
                'jollof rice'
            )
            `,
		); err != nil {
		t.Fatal(err)
	}

	if err =
		store.RecordGenerationProgress(
			ctx,
			SearchCacheNamespaceFood,
			5,
			1,
			"worker-b",
			nil,
		); err != nil {
		t.Fatal(err)
	}

	if err =
		store.CompleteGenerationBuild(
			ctx,
			SearchCacheNamespaceFood,
			5,
		); err != nil {
		t.Fatal(err)
	}

	if err =
		store.MarkGenerationReady(
			ctx,
			SearchCacheNamespaceFood,
			5,
		); err != nil {
		t.Fatal(err)
	}

	if err =
		store.ActivateGeneration(
			ctx,
			SearchCacheNamespaceFood,
			5,
		); err != nil {
		t.Fatal(err)
	}

	active, err =
		store.CurrentActiveGeneration(
			ctx,
			SearchCacheNamespaceFood,
		)
	if err != nil {
		t.Fatal(err)
	}

	if active != 5 {
		t.Fatalf(
			"active generation=%d want=5",
			active,
		)
	}

	var oldStatus string

	if err =
		pool.QueryRow(
			ctx,
			`
            SELECT status
              FROM search_projection_generations
             WHERE namespace_code='FOOD'
               AND generation=4
            `,
		).Scan(
			&oldStatus,
		); err != nil {
		t.Fatal(err)
	}

	if oldStatus !=
		"RETIRED" {
		t.Fatalf(
			"generation 4 status=%s",
			oldStatus,
		)
	}
}

func TestSection13HCompetingGenerationsCannotBothActivate(
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
                search_projection_jobs,
                search_projection_generation_history,
                search_projection_generations
            CASCADE;

            INSERT INTO search_projection_generations (
                namespace_code,
                generation,
                status,
                base_generation,
                rebuild_scope,
                requested_by,
                completed_at,
                validated_at,
                document_count,
                processed_count
            )
            VALUES
            (
                'FOOD',
                4,
                'ACTIVE',
                0,
                'FULL',
                'test',
                now(),
                now(),
                1,
                1
            ),
            (
                'FOOD',
                5,
                'READY',
                4,
                'FULL',
                'test',
                now(),
                now(),
                1,
                1
            ),
            (
                'FOOD',
                6,
                'READY',
                4,
                'FULL',
                'test',
                now(),
                now(),
                1,
                1
            );
            `,
		); err != nil {
		t.Fatal(err)
	}

	store :=
		NewPostgresSearchGenerationStore(
			pool,
		)

	var successCount atomic.Int32
	var conflictCount atomic.Int32

	var wait sync.WaitGroup

	for _, generation := range []int64{
		5,
		6,
	} {
		wait.Add(1)

		go func(
			target int64,
		) {
			defer wait.Done()

			err :=
				store.ActivateGeneration(
					ctx,
					SearchCacheNamespaceFood,
					target,
				)

			if err == nil {
				successCount.Add(1)
				return
			}

			if errors.Is(
				err,
				ErrGenerationActivationConflict,
			) {
				conflictCount.Add(1)
				return
			}

			t.Errorf(
				"activation %d: %v",
				target,
				err,
			)
		}(
			generation,
		)
	}

	wait.Wait()

	if successCount.Load() != 1 {
		t.Fatalf(
			"successful activations=%d want=1",
			successCount.Load(),
		)
	}

	if conflictCount.Load() != 1 {
		t.Fatalf(
			"activation conflicts=%d want=1",
			conflictCount.Load(),
		)
	}
}

func TestSection13HWorkerLeaseIsExclusive(
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
                search_projection_jobs;

            INSERT INTO search_projection_jobs (
                job_id,
                job_type,
                namespace_code,
                generation,
                scope_type,
                status,
                max_attempts,
                available_at,
                dedupe_key
            )
            VALUES (
                '13h-exclusive-job',
                'FULL_REBUILD',
                'FOOD',
                10,
                'FULL',
                'PENDING',
                5,
                now(),
                '13h-exclusive-job'
            );
            `,
		); err != nil {
		t.Fatal(err)
	}

	store :=
		NewPostgresSearchGenerationStore(
			pool,
		)

	var totalClaimed atomic.Int32

	var wait sync.WaitGroup

	for _, workerID := range []string{
		"worker-a",
		"worker-b",
	} {
		wait.Add(1)

		go func(
			id string,
		) {
			defer wait.Done()

			jobs, claimErr :=
				store.ClaimProjectionJobs(
					ctx,
					id,
					[]ProjectionJobType{
						ProjectionJobFullRebuild,
					},
					1,
					30*time.Second,
				)

			if claimErr != nil {
				t.Errorf(
					"worker %s: %v",
					id,
					claimErr,
				)

				return
			}

			totalClaimed.Add(
				int32(
					len(jobs),
				),
			)
		}(
			workerID,
		)
	}

	wait.Wait()

	if totalClaimed.Load() != 1 {
		t.Fatalf(
			"same job claimed %d times",
			totalClaimed.Load(),
		)
	}
}
