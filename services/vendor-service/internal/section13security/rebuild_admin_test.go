package section13security

// Section 13 production rebuild closure: the administration API and scale gates.
//
// The end-to-end gate next door proves the pipeline works when driven directly.
// This file proves it works when driven the way an operator actually drives it:
// an HMAC-v2 signed request to the real projection-administration endpoint,
// followed by the real workers, followed by reading the status back through the
// same API.
//
// "A rebuild endpoint that only inserts a BUILDING row is not sufficient" is the
// requirement, so the assertions follow the generation all the way to ACTIVE
// through the API rather than stopping at the 202.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
	"cosmicforge/logistics/services/vendor-service/internal/platform/servicenonce"
	"cosmicforge/logistics/services/vendor-service/internal/searchrebuild"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// adminRebuildHarness serves the real internal administration routes over a
// real generation store, with the real rebuild workers behind them.
type adminRebuildHarness struct {
	router   *gin.Engine
	pipeline *rebuildPipeline
	pool     *pgxpool.Pool
}

func newAdminRebuildHarness(t *testing.T, pool *pgxpool.Pool) *adminRebuildHarness {
	t.Helper()

	pipeline := newRebuildPipeline(t, pool)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	flags := searchprojection.Flags{SearchEnabled: true, AdminEnabled: true}
	searchprojection.RegisterInternalSearchRoutes(
		router,
		searchprojection.NewInternalHandler(searchprojection.InternalHandlerOptions{
			Food:    &recordingSearch{},
			Shop:    recordingShopSearch{inner: &recordingSearch{}},
			Ranking: searchprojection.NewPostgresRankingProfileStore(pool),
			// The REAL administration service over the REAL generation store.
			Admin:   pipeline.admin,
			Health:  searchprojection.NewPostgresProjectionHealthReader(pool, nil),
			Callers: []string{searchCaller},
			Nonces:  servicenonce.NewStore(pool),
			Flags:   flags,
		}),
		serviceauth.NewVerifier(serviceauth.Secrets{searchCaller: searchSecret}, 5*time.Minute),
		flags,
	)

	return &adminRebuildHarness{router: router, pipeline: pipeline, pool: pool}
}

func (h *adminRebuildHarness) signed(
	t *testing.T, method, target string, body []byte,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		request.Header.Set("Idempotency-Key",
			"rebuild-admin-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	}
	if err := serviceauth.SignRequest(request, searchCaller, searchSecret, body, time.Now()); err != nil {
		t.Fatalf("sign: %v", err)
	}
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, request)
	return recorder
}

// generationView is the shape the administration API returns.
type generationView struct {
	Data struct {
		Generation struct {
			NamespaceCode  string `json:"namespace_code"`
			Generation     int64  `json:"generation"`
			Status         string `json:"status"`
			BaseGeneration int64  `json:"base_generation"`
			DocumentCount  int64  `json:"document_count"`
			ProcessedCount int64  `json:"processed_count"`
			FailureCode    string `json:"failure_code"`
		} `json:"generation"`
		ActiveGeneration int64 `json:"active_generation"`
	} `json:"data"`
}

func decodeGeneration(t *testing.T, recorder *httptest.ResponseRecorder) generationView {
	t.Helper()
	var view generationView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, recorder.Body.String())
	}
	return view
}

// TestHMACRebuildRequestDrivesGenerationToActive is the administration-API
// closure gate.
func TestHMACRebuildRequestDrivesGenerationToActive(t *testing.T) {
	fixture, ctx := newRebuildFixture(t)
	harness := newAdminRebuildHarness(t, fixture.pool)

	const previousGeneration = 8
	seedActiveGeneration(t, fixture.pool, ctx,
		searchprojection.SearchCacheNamespaceFood, previousGeneration)

	// 1. Request the rebuild through the signed endpoint.
	response := harness.signed(t, http.MethodPost,
		"/internal/vendor/v1/search/projections/FOOD/rebuilds", []byte(`{"scope":"FULL"}`))
	if response.Code != http.StatusAccepted {
		t.Fatalf("rebuild request status=%d want=202 body=%s", response.Code, response.Body.String())
	}

	requested := decodeGeneration(t, response)
	generation := requested.Data.Generation.Generation
	if generation == 0 {
		t.Fatalf("rebuild response carried no generation: %s", response.Body.String())
	}
	if requested.Data.Generation.Status != string(searchprojection.SearchGenerationBuilding) {
		t.Fatalf("status=%s want=BUILDING", requested.Data.Generation.Status)
	}
	if requested.Data.Generation.BaseGeneration != previousGeneration {
		t.Fatalf("base_generation=%d want=%d",
			requested.Data.Generation.BaseGeneration, previousGeneration)
	}

	generationPath := "/internal/vendor/v1/search/projections/FOOD/generations/" +
		strconv.FormatInt(generation, 10)

	// 2. Before any worker runs, the API reports BUILDING and the ACTIVE
	//    generation is still the old one - discovery has not moved.
	status := decodeGeneration(t, harness.signed(t, http.MethodGet, generationPath, nil))
	if status.Data.Generation.Status != string(searchprojection.SearchGenerationBuilding) {
		t.Fatalf("pre-worker status=%s want=BUILDING", status.Data.Generation.Status)
	}
	active := decodeGeneration(t, harness.signed(t, http.MethodGet,
		"/internal/vendor/v1/search/projections/FOOD/active-generation", nil))
	if active.Data.ActiveGeneration != previousGeneration {
		t.Fatalf("active generation=%d during BUILDING; want %d",
			active.Data.ActiveGeneration, previousGeneration)
	}

	// 3. Run the real workers.
	harness.pipeline.drain(t, ctx, searchprojection.SearchCacheNamespaceFood, generation, 8)

	// 4. Read the outcome back through the SAME API.
	final := decodeGeneration(t, harness.signed(t, http.MethodGet, generationPath, nil))
	if final.Data.Generation.Status != string(searchprojection.SearchGenerationActive) {
		t.Fatalf("post-worker status=%s want=ACTIVE (failure=%s) body=%s",
			final.Data.Generation.Status, final.Data.Generation.FailureCode,
			harness.signed(t, http.MethodGet, generationPath, nil).Body.String())
	}
	if final.Data.Generation.DocumentCount != int64(rbFoodItems) {
		t.Fatalf("document_count=%d want=%d",
			final.Data.Generation.DocumentCount, rbFoodItems)
	}
	if final.Data.Generation.ProcessedCount != final.Data.Generation.DocumentCount {
		t.Fatalf("processed=%d document=%d",
			final.Data.Generation.ProcessedCount, final.Data.Generation.DocumentCount)
	}

	activeAfter := decodeGeneration(t, harness.signed(t, http.MethodGet,
		"/internal/vendor/v1/search/projections/FOOD/active-generation", nil))
	if activeAfter.Data.ActiveGeneration != generation {
		t.Fatalf("active generation=%d after rebuild; want the new %d",
			activeAfter.Data.ActiveGeneration, generation)
	}

	// 5. Health reports the new generation as the live one.
	health := harness.signed(t, http.MethodGet,
		"/internal/vendor/v1/search/projection-health", nil)
	if health.Code != http.StatusOK {
		t.Fatalf("health status=%d", health.Code)
	}
	var healthView struct {
		Data struct {
			Food searchprojection.NamespaceHealth `json:"food"`
		} `json:"data"`
	}
	if err := json.Unmarshal(health.Body.Bytes(), &healthView); err != nil {
		t.Fatal(err)
	}
	if healthView.Data.Food.ActiveGeneration != generation {
		t.Fatalf("health active generation=%d want=%d",
			healthView.Data.Food.ActiveGeneration, generation)
	}
	if healthView.Data.Food.DocumentCount != int64(rbFoodItems) {
		t.Fatalf("health document_count=%d want=%d",
			healthView.Data.Food.DocumentCount, rbFoodItems)
	}
}

// TestHMACRebuildRequestRequiresSignature keeps the administration surface shut
// to unsigned callers even now that it does real work.
func TestHMACRebuildRequestRequiresSignature(t *testing.T) {
	fixture, _ := newRebuildFixture(t)
	harness := newAdminRebuildHarness(t, fixture.pool)

	unsigned := httptest.NewRequest(http.MethodPost,
		"/internal/vendor/v1/search/projections/FOOD/rebuilds",
		bytes.NewReader([]byte(`{"scope":"FULL"}`)))
	unsigned.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, unsigned)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned rebuild status=%d want=401", recorder.Code)
	}

	// And nothing was queued by the attempt.
	var jobs int
	if err := fixture.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM search_projection_jobs WHERE job_type='FULL_REBUILD'`,
	).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("%d rebuild jobs were queued by an unsigned request", jobs)
	}
}

// ---------------------------------------------------------------------------
// Scale
// ---------------------------------------------------------------------------

// rebuildScaleItems is large enough to force many bounded batches through the
// real worker without repeating the 24k search-scale corpus, which measures a
// different thing.
const rebuildScaleItems = 1500

// TestRebuildAtScaleStaysBatchedAndBounded runs a real rebuild over a
// meaningfully large authoritative catalogue.
//
// What is being checked is not speed. It is that the rebuild reaches ACTIVE by
// walking bounded batches, that the batch ceiling holds, and that the document
// count matches the catalogue exactly - a rebuild that silently dropped or
// duplicated rows at a page boundary would be far more damaging than a slow one.
func TestRebuildAtScaleStaysBatchedAndBounded(t *testing.T) {
	fixture, ctx := newRebuildFixture(t)

	// Extend the Food catalogue with generate_series, in the domain rather
	// than in the projection.
	if _, err := fixture.pool.Exec(ctx, `
        INSERT INTO food_items
            (id, business_id, branch_id, menu_id, section_id, name,
             base_price_minor, dietary_codes, position, status)
        SELECT
            'fi_scale_' || lpad(i::text, 5, '0'),
            $1, $2, $3, $4,
            'Jollof Rice Scale ' || i,
            250000 + i,
            '[]'::jsonb,
            1000 + i,
            'ACTIVE'
        FROM generate_series(1, $5) AS i`,
		rbBusiness, rbBranch, rbFoodMenu, rbFoodSection, rebuildScaleItems,
	); err != nil {
		t.Fatalf("seed scale catalogue: %v", err)
	}

	total := rbFoodItems + rebuildScaleItems

	pipeline := newRebuildPipeline(t, fixture.pool)

	const previousGeneration = 9
	seedActiveGeneration(t, fixture.pool, ctx,
		searchprojection.SearchCacheNamespaceFood, previousGeneration)

	requested, err := pipeline.admin.RequestFullRebuild(
		ctx, searchprojection.SearchCacheNamespaceFood, "scale-gate")
	if err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	final := pipeline.drain(t, ctx, searchprojection.SearchCacheNamespaceFood,
		requested.Generation, 10)
	elapsed := time.Since(started)

	if final.Status != searchprojection.SearchGenerationActive {
		t.Fatalf("scale rebuild status=%s want=ACTIVE (%s %s)",
			final.Status, final.FailureCode, final.FailureReason)
	}
	if final.DocumentCount != int64(total) {
		t.Fatalf("document_count=%d want=%d; a page boundary dropped or duplicated rows",
			final.DocumentCount, total)
	}
	if final.ProcessedCount != final.DocumentCount {
		t.Fatalf("processed=%d document=%d", final.ProcessedCount, final.DocumentCount)
	}

	// The catalogue is bigger than the batch ceiling, so this cannot have been
	// one pass - which is the point.
	if total <= searchrebuild.MaxBatch {
		t.Fatalf("fixture of %d does not exceed the batch ceiling %d", total, searchrebuild.MaxBatch)
	}

	t.Logf("rebuilt %d Food documents in %s over batches of at most %d",
		final.DocumentCount, elapsed.Round(time.Millisecond), searchrebuild.MaxBatch)

	// Search over the rebuilt generation still answers, bounded.
	engine := searchprojection.NewFoodSearchEngine(
		searchprojection.NewPostgresFoodSearchBackend(fixture.pool))
	response, err := engine.Search(ctx, searchprojection.FoodEngineSearchRequest{
		Query: "jollof", Generation: requested.Generation, PageSize: 25,
	})
	if err != nil {
		t.Fatalf("search rebuilt scale generation: %v", err)
	}
	if len(response.Hits) != 25 {
		t.Fatalf("search returned %d hits; want a full bounded page of 25", len(response.Hits))
	}
}

// TestRebuildSourceQueryPlansAreIndexCapable checks the keyset scan the rebuild
// depends on can be planned through an index rather than a full table scan.
//
// A rebuild that sequentially scans the whole catalogue for every batch is
// quadratic in catalogue size: correct, and unusable on a real marketplace.
func TestRebuildSourceQueryPlansAreIndexCapable(t *testing.T) {
	fixture, ctx := newRebuildFixture(t)

	if _, err := fixture.pool.Exec(ctx, `
        INSERT INTO food_items
            (id, business_id, branch_id, menu_id, section_id, name,
             base_price_minor, dietary_codes, position, status)
        SELECT
            'fi_plan_' || lpad(i::text, 5, '0'),
            $1, $2, $3, $4, 'Plan Item ' || i, 100000 + i, '[]'::jsonb, 5000 + i, 'ACTIVE'
        FROM generate_series(1, 2000) AS i`,
		rbBusiness, rbBranch, rbFoodMenu, rbFoodSection); err != nil {
		t.Fatalf("seed plan corpus: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx, `ANALYZE food_items`); err != nil {
		t.Fatal(err)
	}

	// The keyset predicate the rebuild source pages with.
	rows, err := fixture.pool.Query(ctx, `
        EXPLAIN SELECT id FROM food_items WHERE id > $1 ORDER BY id LIMIT 200`,
		"fi_plan_00000")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	plan := ""
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan += line + "\n"
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("rebuild keyset plan:\n%s", plan)

	// The primary key makes this an index scan; a sequential scan here would
	// mean the cursor has no usable index at all.
	if !containsFold([]string{plan}, "Index") {
		t.Errorf("the rebuild keyset scan has no index-capable plan:\n%s", plan)
	}
}
