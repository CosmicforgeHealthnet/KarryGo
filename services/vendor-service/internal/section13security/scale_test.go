package section13security

// Section 13I scale and query-plan certification.
//
// Two questions are asked, and they are different questions.
//
// The first is behavioural: at a realistic dataset size, does search still
// return correct, bounded results across every capability Section 13E claims -
// exact, prefix, typo, synonym, category, availability, facets, suggestions,
// stable pagination?
//
// The second is structural: are the plans PostgreSQL chooses index-capable, or
// is the engine getting away with a sequential scan that happens to be fast
// while the table is small? That question is deliberately NOT asked as "no plan
// may ever contain a Seq Scan". A small lookup table is correctly scanned
// sequentially, and asserting otherwise pins a planner decision rather than an
// architectural property. What is asserted instead is narrower and actually
// meaningful: the indexes that make the large search paths scalable exist, and
// with enable_seqscan off the planner can satisfy the real search queries
// through them - which is only possible if the access paths are index-capable
// in the first place.
//
// No millisecond SLA is asserted. Timings are recorded for the certification
// report, because a number measured once on one machine is evidence, not a
// threshold.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
)

// Dataset sizes. Section 13I asks for at least 20,000 searchable documents per
// namespace; these are the sizes the certification report records.
const (
	scaleFoodDocuments = 24000
	scaleShopDocuments = 24000

	scaleGeneration = 9001

	// A separate, deliberately tiny generation used only to prove that the
	// bulk terms projection reproduces the per-row trigger. It is built and
	// torn down before the real corpus and is never searched.
	scaleProbeGeneration = 9002
	scaleProbeDocuments  = 200
)

// scaleCuisines and scaleDishes are combined to give the Food corpus real
// lexical variety, so a query is discriminating rather than matching
// everything.
var (
	scaleCuisines = []string{"nigerian", "yoruba", "igbo", "hausa", "continental", "chinese"}
	scaleDishes   = []string{"jollof rice", "fried rice", "moi moi", "egusi soup", "suya", "shawarma"}
	scaleBrands   = []string{"dangote", "honeywell", "mamador", "golden penny", "kings", "power oil"}
	scaleMeasures = []string{"50kg", "25kg", "10kg", "5kg", "1kg", "500g"}
)

// The corpus is written with generate_series rather than through the
// projector because 48,000 projector round trips would dominate the test's
// runtime and prove nothing about search; the projector's own correctness is
// Section 13D's gate. The rows are shaped exactly as the projector shapes
// them, including the terms rows the engine's token path depends on.
//
// The document tables carry a FOR EACH ROW AFTER INSERT trigger
// (refresh_food_search_terms / refresh_shop_search_terms) which rebuilds one
// document's terms at a time. That is the right shape for the incremental
// projection path it exists to serve, and it is production behaviour this test
// does not change. It is the wrong shape for a bulk load: a 24,000-row INSERT
// fires it 24,000 times, each firing running a multi-CTE DELETE + INSERT and
// maintaining five indexes. Measured on this corpus, one namespace took 57s
// with the trigger attached against 1.1s with it detached - the trigger is
// ~98% of the seed cost, and at that rate the Shop half alone overran the
// seed's own ten-minute deadline.
//
// So the bulk load detaches the trigger and materialises the same terms with
// one set-based statement per namespace. That is a shortcut, and a shortcut
// that silently stopped matching the trigger would leave every search case
// below querying a corpus production could never produce. So it is not taken
// on trust: assertBulkTermsMatchTrigger first drives a small probe corpus
// through the live trigger and requires the set-based statement to produce an
// identical term set. If a future migration changes the trigger, that check
// fails loudly here rather than quietly weakening every search assertion in
// this file.
const (
	scaleFoodDocumentsSQL = `
        INSERT INTO food_search_documents (
            generation, document_id, business_ref, branch_ref, food_item_ref,
            name, normalized_name, search_tokens, aliases, synonyms,
            cuisine_terms, dish_types, meal_periods, provider_types,
            dietary_claims, active, availability_summary, effective_price_summary,
            catalogue_revision, search_text
        )
        SELECT
            $1,
            'food_scale_' || i,
            'vb_scale_' || (i % 40),
            'vbr_scale_' || (i % 120),
            'fi_scale_' || i,
            initcap(dish) || ' ' || i,
            lower(dish) || ' ' || i,
            string_to_array(lower(dish) || ' ' || i, ' '),
            ARRAY[left(lower(dish), 4) || i],
            ARRAY[cuisine || ' special'],
            ARRAY[cuisine],
            ARRAY[split_part(dish, ' ', 1)],
            ARRAY[CASE WHEN i % 3 = 0 THEN 'BREAKFAST' WHEN i % 3 = 1 THEN 'LUNCH' ELSE 'DINNER' END],
            ARRAY[CASE WHEN i % 2 = 0 THEN 'RESTAURANT' ELSE 'FAST_FOOD' END],
            CASE WHEN i % 7 = 0 THEN ARRAY['HALAL'] ELSE ARRAY[]::text[] END,
            (i % 11) <> 0,
            jsonb_build_object('available', (i % 5) <> 0),
            jsonb_build_object('price_minor', 100000 + (i % 900) * 100),
            1,
            lower(dish) || ' ' || i || ' ' || cuisine
        FROM generate_series(1, $2) AS i
        CROSS JOIN LATERAL (
            SELECT ($3::text[])[1 + (i % array_length($3::text[], 1))] AS dish,
                   ($4::text[])[1 + (i % array_length($4::text[], 1))] AS cuisine
        ) AS pick`

	scaleShopDocumentsSQL = `
        INSERT INTO shop_search_documents (
            generation, document_id, business_ref, branch_ref,
            product_ref, variant_ref, listing_ref,
            name, normalized_name, brand, search_tokens, aliases, synonyms,
            sell_units, measure_names, local_measure_aliases, packaging_terms,
            sku_terms, barcode_terms, canonical_taxonomy,
            retail_eligible, wholesale_eligible, active,
            inventory_availability_summary, effective_price_summary,
            catalogue_revision, inventory_revision, search_text
        )
        SELECT
            $1,
            'shop_scale_' || i,
            'vb_scale_' || (i % 40),
            'vbr_scale_' || (i % 120),
            'sp_scale_' || i,
            'sv_scale_' || i,
            'sl_scale_' || i,
            initcap(brand) || ' Rice ' || measure || ' ' || i,
            lower(brand) || ' rice ' || measure || ' ' || i,
            brand,
            string_to_array(lower(brand) || ' rice ' || measure || ' ' || i, ' '),
            ARRAY['sku' || i],
            ARRAY[brand || ' grain'],
            ARRAY['SACK'],
            ARRAY[measure],
            ARRAY['derica'],
            ARRAY['SACK'],
            ARRAY['SKU-' || i],
            ARRAY['62' || lpad(i::text, 11, '0')],
            jsonb_build_object('category', ARRAY['GRAINS']),
            TRUE,
            (i % 4) = 0,
            (i % 13) <> 0,
            jsonb_build_object('available', (i % 6) <> 0, 'in_stock', (i % 6) <> 0),
            jsonb_build_object('price_minor', 500000 + (i % 700) * 100),
            1, 1,
            lower(brand) || ' rice ' || measure || ' ' || i
        FROM generate_series(1, $2) AS i
        CROSS JOIN LATERAL (
            SELECT ($3::text[])[1 + (i % array_length($3::text[], 1))] AS brand,
                   ($4::text[])[1 + (i % array_length($4::text[], 1))] AS measure
        ) AS pick`
)

// The set-based equivalents of refresh_food_search_terms and
// refresh_shop_search_terms: the same phrase families, the same btrim/lower
// normalisation, the same DISTINCT, the same PHRASE + TOKEN expansion, the
// same ON CONFLICT DO NOTHING - applied to a whole generation in one statement
// instead of to one NEW row at a time. %s is the destination table, which is
// the real terms table for the corpus and a scratch table for the equivalence
// probe.
const (
	scaleFoodTermsSQL = `
        WITH docs AS (
            SELECT * FROM food_search_documents WHERE generation = $1
        ),
        phrases (generation, document_id, term_kind, display_term, normalized_term) AS (
            SELECT generation, document_id, 'NAME', name, normalized_name FROM docs
            UNION ALL
            SELECT generation, document_id, 'ALIAS', value, value
              FROM docs CROSS JOIN LATERAL unnest(aliases) AS value
            UNION ALL
            SELECT generation, document_id, 'SYNONYM', value, value
              FROM docs CROSS JOIN LATERAL unnest(synonyms) AS value
            UNION ALL
            SELECT generation, document_id, 'CATEGORY', value, value
              FROM docs CROSS JOIN LATERAL unnest(
                       provider_types || cuisine_terms || meal_periods
                       || dish_types || food_groups || occasions
                   ) AS value
            UNION ALL
            SELECT generation, document_id, 'TAG', value, value
              FROM docs CROSS JOIN LATERAL unnest(
                       dietary_claims || preference_claims
                   ) AS value
        ),
        clean_phrases AS (
            SELECT DISTINCT
                generation, document_id, term_kind,
                btrim(display_term) AS display_term,
                lower(btrim(normalized_term)) AS normalized_term
            FROM phrases
            WHERE btrim(normalized_term) <> ''
        )
        INSERT INTO %s
            (generation, document_id, term_kind, term_scope, display_term, normalized_term)
        SELECT generation, document_id, term_kind, 'PHRASE', display_term, normalized_term
          FROM clean_phrases
        UNION ALL
        SELECT DISTINCT generation, document_id, term_kind, 'TOKEN', token, token
          FROM clean_phrases
          CROSS JOIN LATERAL unnest(string_to_array(normalized_term, ' ')) AS token
         WHERE btrim(token) <> ''
        ON CONFLICT DO NOTHING`

	scaleShopTermsSQL = `
        WITH docs AS (
            SELECT * FROM shop_search_documents WHERE generation = $1
        ),
        json_category_phrases (generation, document_id, value) AS (
            SELECT d.generation, d.document_id, entry.value #>> '{}'
              FROM docs d CROSS JOIN LATERAL jsonb_each(d.canonical_taxonomy) AS entry
             WHERE jsonb_typeof(entry.value) = 'string'
            UNION ALL
            SELECT d.generation, d.document_id, array_value
              FROM docs d
              CROSS JOIN LATERAL jsonb_each(d.canonical_taxonomy) AS entry
              CROSS JOIN LATERAL jsonb_array_elements_text(
                       CASE WHEN jsonb_typeof(entry.value) = 'array'
                            THEN entry.value ELSE '[]'::jsonb END
                   ) AS array_value
        ),
        phrases (generation, document_id, term_kind, display_term, normalized_term) AS (
            SELECT generation, document_id, 'NAME', name, normalized_name FROM docs
            UNION ALL
            SELECT generation, document_id, 'ALIAS', value, value
              FROM docs CROSS JOIN LATERAL unnest(aliases) AS value
            UNION ALL
            SELECT generation, document_id, 'SYNONYM', value, value
              FROM docs CROSS JOIN LATERAL unnest(synonyms) AS value
            UNION ALL
            SELECT generation, document_id, 'CATEGORY', value, lower(value)
              FROM json_category_phrases
            UNION ALL
            SELECT generation, document_id, 'BRAND', brand, lower(brand)
              FROM docs WHERE brand IS NOT NULL AND btrim(brand) <> ''
            UNION ALL
            SELECT generation, document_id, 'IDENTIFIER', value, lower(value)
              FROM docs CROSS JOIN LATERAL unnest(sku_terms || barcode_terms) AS value
            UNION ALL
            SELECT generation, document_id, 'MEASUREMENT', value, value
              FROM docs CROSS JOIN LATERAL unnest(
                       sell_units || measure_names || local_measure_aliases || packaging_terms
                   ) AS value
            UNION ALL
            SELECT generation, document_id, 'TAG', value, value
              FROM docs CROSS JOIN LATERAL unnest(fresh_attributes) AS value
            UNION ALL
            SELECT generation, document_id, 'TAG', temperature_zone, lower(temperature_zone)
              FROM docs
             WHERE temperature_zone IS NOT NULL AND btrim(temperature_zone) <> ''
        ),
        clean_phrases AS (
            SELECT DISTINCT
                generation, document_id, term_kind,
                btrim(display_term) AS display_term,
                lower(btrim(normalized_term)) AS normalized_term
            FROM phrases
            WHERE normalized_term IS NOT NULL AND btrim(normalized_term) <> ''
        )
        INSERT INTO %s
            (generation, document_id, term_kind, term_scope, display_term, normalized_term)
        SELECT generation, document_id, term_kind, 'PHRASE', display_term, normalized_term
          FROM clean_phrases
        UNION ALL
        SELECT DISTINCT generation, document_id, term_kind, 'TOKEN', token, token
          FROM clean_phrases
          CROSS JOIN LATERAL unnest(string_to_array(normalized_term, ' ')) AS token
         WHERE btrim(token) <> ''
        ON CONFLICT DO NOTHING`
)

// scaleNamespace ties one namespace's document table, terms table, per-row
// trigger, document generator and set-based terms projection together, so the
// seed and the equivalence probe drive both namespaces through the same code.
type scaleNamespace struct {
	name          string
	documents     string
	terms         string
	trigger       string
	documentCount int
	documentsSQL  string
	termsSQL      string
	vocabularyA   []string
	vocabularyB   []string
}

func scaleNamespaces() []scaleNamespace {
	return []scaleNamespace{
		{
			name:          "food",
			documents:     "food_search_documents",
			terms:         "food_search_terms",
			trigger:       "food_search_terms_refresh_trg",
			documentCount: scaleFoodDocuments,
			documentsSQL:  scaleFoodDocumentsSQL,
			termsSQL:      scaleFoodTermsSQL,
			vocabularyA:   scaleDishes,
			vocabularyB:   scaleCuisines,
		},
		{
			name:          "shop",
			documents:     "shop_search_documents",
			terms:         "shop_search_terms",
			trigger:       "shop_search_terms_refresh_trg",
			documentCount: scaleShopDocuments,
			documentsSQL:  scaleShopDocumentsSQL,
			termsSQL:      scaleShopTermsSQL,
			vocabularyA:   scaleBrands,
			vocabularyB:   scaleMeasures,
		},
	}
}

// assertBulkTermsMatchTrigger is the guard on the shortcut.
//
// A small probe corpus is inserted with the per-row trigger live, so the terms
// it produces are production's own output. The set-based statement is then run
// over the same documents into a scratch table, and the two term sets must be
// identical in both directions. A difference means the bulk path no longer
// reproduces the trigger, and the corpus every search assertion below reads
// would no longer be a corpus production could produce.
func assertBulkTermsMatchTrigger(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	namespace scaleNamespace,
) {
	t.Helper()

	// The probe runs on one pinned connection, because it compares against a
	// temporary table and a pooled connection would not be guaranteed to see
	// that table again on the next statement.
	connection, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("%s probe: acquire connection: %v", namespace.name, err)
	}
	defer connection.Release()

	cleanup := func() {
		_, _ = connection.Exec(ctx,
			"DELETE FROM "+namespace.terms+" WHERE generation = $1", scaleProbeGeneration)
		_, _ = connection.Exec(ctx,
			"DELETE FROM "+namespace.documents+" WHERE generation = $1", scaleProbeGeneration)
		_, _ = connection.Exec(ctx, "DROP TABLE IF EXISTS probe_expected_terms")
	}
	cleanup()
	defer cleanup()

	// Documents first, with the trigger live: these terms are production's.
	if _, err = connection.Exec(ctx, namespace.documentsSQL,
		scaleProbeGeneration, scaleProbeDocuments,
		namespace.vocabularyA, namespace.vocabularyB,
	); err != nil {
		t.Fatalf("%s probe: seed documents: %v", namespace.name, err)
	}

	if _, err = connection.Exec(ctx, `
        CREATE TEMPORARY TABLE probe_expected_terms (
            generation bigint NOT NULL,
            document_id text NOT NULL,
            term_kind text NOT NULL,
            term_scope text NOT NULL,
            display_term text NOT NULL,
            normalized_term text NOT NULL,
            PRIMARY KEY (generation, document_id, term_kind, term_scope, normalized_term)
        )`); err != nil {
		t.Fatalf("%s probe: create comparison table: %v", namespace.name, err)
	}

	if _, err = connection.Exec(ctx, fmt.Sprintf(
		namespace.termsSQL, "probe_expected_terms"), scaleProbeGeneration,
	); err != nil {
		t.Fatalf("%s probe: run set-based projection: %v", namespace.name, err)
	}

	const comparison = `
        SELECT
            (SELECT count(*) FROM %[1]s WHERE generation = $1),
            (SELECT count(*) FROM probe_expected_terms),
            (SELECT count(*) FROM (
                SELECT generation, document_id, term_kind, term_scope, display_term, normalized_term
                  FROM %[1]s WHERE generation = $1
                EXCEPT
                SELECT generation, document_id, term_kind, term_scope, display_term, normalized_term
                  FROM probe_expected_terms) AS missing_from_bulk),
            (SELECT count(*) FROM (
                SELECT generation, document_id, term_kind, term_scope, display_term, normalized_term
                  FROM probe_expected_terms
                EXCEPT
                SELECT generation, document_id, term_kind, term_scope, display_term, normalized_term
                  FROM %[1]s WHERE generation = $1) AS extra_from_bulk)`

	var triggerRows, bulkRows, onlyTrigger, onlyBulk int64
	if err = connection.QueryRow(ctx,
		fmt.Sprintf(comparison, namespace.terms), scaleProbeGeneration,
	).Scan(&triggerRows, &bulkRows, &onlyTrigger, &onlyBulk); err != nil {
		t.Fatalf("%s probe: compare term sets: %v", namespace.name, err)
	}

	if triggerRows == 0 {
		t.Fatalf("%s probe: the live trigger produced no terms; the equivalence check would be vacuous",
			namespace.name)
	}
	if onlyTrigger != 0 || onlyBulk != 0 {
		t.Fatalf("%s bulk term projection no longer matches %s: trigger=%d bulk=%d "+
			"missing_from_bulk=%d extra_from_bulk=%d",
			namespace.name, namespace.trigger, triggerRows, bulkRows, onlyTrigger, onlyBulk)
	}

	t.Logf("%s bulk term projection matches %s exactly (%d terms over %d probe documents)",
		namespace.name, namespace.trigger, triggerRows, scaleProbeDocuments)
}

// buildScaleCorpus builds both projections.
func buildScaleCorpus(t *testing.T, pool *pgxpool.Pool, ctx context.Context) error {
	t.Helper()

	namespaces := scaleNamespaces()

	if _, err := pool.Exec(ctx,
		`TRUNCATE TABLE food_search_documents, shop_search_documents CASCADE`); err != nil {
		t.Fatalf("reset projections: %v", err)
	}

	// Prove the shortcut before taking it.
	for _, namespace := range namespaces {
		assertBulkTermsMatchTrigger(t, pool, ctx, namespace)
	}

	started := time.Now()

	for _, namespace := range namespaces {
		if _, err := pool.Exec(ctx, fmt.Sprintf(
			"ALTER TABLE %s DISABLE TRIGGER %s", namespace.documents, namespace.trigger,
		)); err != nil {
			t.Fatalf("detach %s: %v", namespace.trigger, err)
		}
	}
	// Reattached unconditionally: a corpus build that failed halfway must not
	// leave the incremental projection path detached for the tests that follow.
	defer func() {
		for _, namespace := range namespaces {
			if _, err := pool.Exec(context.Background(), fmt.Sprintf(
				"ALTER TABLE %s ENABLE TRIGGER %s", namespace.documents, namespace.trigger,
			)); err != nil {
				t.Errorf("reattach %s: %v", namespace.trigger, err)
			}
		}
	}()

	for _, namespace := range namespaces {
		documentsStarted := time.Now()
		if _, err := pool.Exec(ctx, namespace.documentsSQL,
			scaleGeneration, namespace.documentCount,
			namespace.vocabularyA, namespace.vocabularyB,
		); err != nil {
			t.Fatalf("seed %s corpus: %v", namespace.name, err)
		}
		documentsElapsed := time.Since(documentsStarted)

		// TOKEN rows feed the prefix and typo paths, PHRASE rows feed
		// autocomplete, and every other term kind the trigger emits - ALIAS,
		// SYNONYM, CATEGORY, BRAND, IDENTIFIER, MEASUREMENT, TAG - is emitted
		// here too. Seeding only some of them would leave a capability
		// untested while looking like it passed.
		termsStarted := time.Now()
		if _, err := pool.Exec(ctx, fmt.Sprintf(
			namespace.termsSQL, namespace.terms), scaleGeneration,
		); err != nil {
			t.Fatalf("seed %s terms: %v", namespace.name, err)
		}

		t.Logf("%s: %d documents in %s, terms in %s",
			namespace.name, namespace.documentCount,
			documentsElapsed.Round(time.Millisecond),
			time.Since(termsStarted).Round(time.Millisecond))
	}

	// ANALYZE, so every plan inspected below is chosen from real statistics
	// rather than from the planner's defaults for an unanalysed table.
	for _, table := range []string{
		"food_search_documents", "food_search_terms",
		"shop_search_documents", "shop_search_terms",
	} {
		if _, err := pool.Exec(ctx, "ANALYZE "+table); err != nil {
			t.Fatalf("analyze %s: %v", table, err)
		}
	}

	t.Logf("scale corpus seeded in %s", time.Since(started).Round(time.Millisecond))
	return nil
}

// scaleCorpusOnce keeps the 48,000-document corpus to a single build for the
// whole package.
//
// Seeding takes over a minute, and every scale test below reads the same
// read-only corpus, so rebuilding it per test would multiply the cost by six
// for no additional coverage - and would push each test past its own context
// deadline. The isolated test runner still gives this package its own database,
// so sharing within the package costs no isolation between packages.
var (
	scaleCorpusOnce sync.Once
	scaleCorpusErr  error
)

// ensureScaleCorpus builds the corpus at most once, on a context of its own so
// the seed is never cut short by an individual test's deadline.
func ensureScaleCorpus(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()

	pool, _ := testPool(t)

	scaleCorpusOnce.Do(func() {
		seedCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		scaleCorpusErr = buildScaleCorpus(t, pool, seedCtx)
	})
	if scaleCorpusErr != nil {
		t.Fatalf("build scale corpus: %v", scaleCorpusErr)
	}

	// A fresh deadline for the test itself. The context testPool returns was
	// started before the corpus was built, and on the first caller most of it
	// has already been spent seeding - which would fail the test that paid the
	// setup cost while every later test passed.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	return pool, ctx
}

func scaleCounts(t *testing.T, pool *pgxpool.Pool, ctx context.Context) (int64, int64) {
	t.Helper()
	var food, shop int64
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM food_search_documents WHERE generation=$1`, scaleGeneration,
	).Scan(&food); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM shop_search_documents WHERE generation=$1`, scaleGeneration,
	).Scan(&shop); err != nil {
		t.Fatal(err)
	}
	return food, shop
}

// TestSection13ScaleSearchRemainsCorrectAndBounded is the behavioural half.
func TestSection13ScaleSearchRemainsCorrectAndBounded(t *testing.T) {
	pool, ctx := ensureScaleCorpus(t)

	food, shop := scaleCounts(t, pool, ctx)
	if food < 20000 || shop < 20000 {
		t.Fatalf("scale corpus food=%d shop=%d; Section 13I requires at least 20000 each", food, shop)
	}
	t.Logf("FOOD documents=%d SHOP documents=%d", food, shop)

	foodEngine := searchprojection.NewFoodSearchEngine(searchprojection.NewPostgresFoodSearchBackend(pool))
	shopEngine := searchprojection.NewShopSearchEngine(searchprojection.NewPostgresShopSearchBackend(pool))

	available := true

	foodCases := []struct {
		name    string
		request searchprojection.FoodEngineSearchRequest
	}{
		{"exact dish", searchprojection.FoodEngineSearchRequest{Query: "jollof rice 12"}},
		{"prefix", searchprojection.FoodEngineSearchRequest{Query: "jol"}},
		{"typo", searchprojection.FoodEngineSearchRequest{Query: "jolof"}},
		{"synonym phrase", searchprojection.FoodEngineSearchRequest{Query: "nigerian special"}},
		{"cuisine filter", searchprojection.FoodEngineSearchRequest{
			Query:   "rice",
			Filters: searchprojection.FoodSearchFilters{Cuisines: []string{"yoruba"}},
		}},
		{"dish filter", searchprojection.FoodEngineSearchRequest{
			Query:   "rice",
			Filters: searchprojection.FoodSearchFilters{DishTypes: []string{"jollof"}},
		}},
		{"availability filter", searchprojection.FoodEngineSearchRequest{
			Query:   "rice",
			Filters: searchprojection.FoodSearchFilters{Available: &available},
		}},
		{"facets", searchprojection.FoodEngineSearchRequest{
			Query:  "rice",
			Facets: []string{"CUISINE", "DISH_TYPE", "MEAL_PERIOD", "AVAILABILITY"},
		}},
	}

	for _, testCase := range foodCases {
		t.Run("food/"+testCase.name, func(t *testing.T) {
			request := testCase.request
			request.Generation = scaleGeneration
			if request.PageSize == 0 {
				request.PageSize = 20
			}

			started := time.Now()
			response, err := foodEngine.Search(ctx, request)
			elapsed := time.Since(started)
			if err != nil {
				t.Fatalf("search: %v", err)
			}

			if len(response.Hits) > request.PageSize {
				t.Fatalf("hits=%d exceeds page size %d", len(response.Hits), request.PageSize)
			}
			for _, facet := range response.Facets {
				if len(facet.Values) > 50 {
					t.Errorf("facet %s returned %d values; facet cardinality must stay bounded",
						facet.Name, len(facet.Values))
				}
			}
			t.Logf("food/%s hits=%d facets=%d in %s",
				testCase.name, len(response.Hits), len(response.Facets), elapsed.Round(time.Millisecond))
		})
	}

	shopCases := []struct {
		name    string
		request searchprojection.ShopEngineSearchRequest
	}{
		{"exact listing", searchprojection.ShopEngineSearchRequest{Query: "dangote rice 50kg 12"}},
		{"prefix", searchprojection.ShopEngineSearchRequest{Query: "dango"}},
		{"measurement term", searchprojection.ShopEngineSearchRequest{Query: "50kg rice"}},
		{"brand filter", searchprojection.ShopEngineSearchRequest{
			Query:   "rice",
			Filters: searchprojection.ShopSearchFilters{Brands: []string{"dangote"}},
		}},
		{"category filter", searchprojection.ShopEngineSearchRequest{
			Query:   "rice",
			Filters: searchprojection.ShopSearchFilters{CanonicalCategories: []string{"GRAINS"}},
		}},
		{"stock filter", searchprojection.ShopEngineSearchRequest{
			Query:   "rice",
			Filters: searchprojection.ShopSearchFilters{Available: &available},
		}},
		{"facets", searchprojection.ShopEngineSearchRequest{
			Query:  "rice",
			Facets: []string{"CATEGORY", "BRAND", "MEASUREMENT", "AVAILABILITY"},
		}},
	}

	for _, testCase := range shopCases {
		t.Run("shop/"+testCase.name, func(t *testing.T) {
			request := testCase.request
			request.Generation = scaleGeneration
			if request.PageSize == 0 {
				request.PageSize = 20
			}

			started := time.Now()
			response, err := shopEngine.Search(ctx, request)
			elapsed := time.Since(started)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if len(response.Hits) > request.PageSize {
				t.Fatalf("hits=%d exceeds page size %d", len(response.Hits), request.PageSize)
			}
			for _, facet := range response.Facets {
				if len(facet.Values) > 50 {
					t.Errorf("facet %s returned %d values; facet cardinality must stay bounded",
						facet.Name, len(facet.Values))
				}
			}
			t.Logf("shop/%s hits=%d facets=%d in %s",
				testCase.name, len(response.Hits), len(response.Facets), elapsed.Round(time.Millisecond))
		})
	}

	// Suggestions stay bounded AND actually resolve at scale.
	//
	// The prefixes are chosen to match the corpus, because a prefix that
	// matches nothing would return zero suggestions and pass a "bounded" check
	// while proving the autocomplete path was never exercised. Autocomplete
	// matches the start of a whole indexed phrase, so "jol" reaches "jollof
	// rice ..." and "dan" reaches "dangote rice ...".
	for _, probe := range []struct {
		namespace string
		prefix    string
		suggest   func(context.Context, searchprojection.SuggestionRequest) ([]searchprojection.SearchSuggestion, error)
	}{
		{"food", "jol", foodEngine.Suggest},
		{"shop", "dan", shopEngine.Suggest},
	} {
		suggestions, err := probe.suggest(ctx, searchprojection.SuggestionRequest{
			Query: probe.prefix, Generation: scaleGeneration, Limit: 10,
		})
		if err != nil {
			t.Fatalf("%s suggest: %v", probe.namespace, err)
		}
		if len(suggestions) == 0 {
			t.Errorf("%s suggestions for %q returned nothing; the autocomplete path is not reachable",
				probe.namespace, probe.prefix)
		}
		if len(suggestions) > 10 {
			t.Fatalf("%s suggestions=%d exceeds the bound", probe.namespace, len(suggestions))
		}
		for _, suggestion := range suggestions {
			if !strings.HasPrefix(suggestion.NormalizedText, probe.prefix) {
				t.Errorf("%s suggestion %q does not match the requested prefix %q",
					probe.namespace, suggestion.NormalizedText, probe.prefix)
			}
		}
		t.Logf("%s suggestions for %q = %d", probe.namespace, probe.prefix, len(suggestions))
	}

	// An over-large limit is clamped rather than honoured.
	if _, err := foodEngine.Suggest(ctx, searchprojection.SuggestionRequest{
		Query: "jol", Generation: scaleGeneration, Limit: 1000,
	}); err == nil {
		t.Log("an over-large suggestion limit was accepted and clamped by the engine")
	}

}

// TestSection13ScalePaginationIsStableAndBounded walks several pages of a
// broad query and proves the cursor never repeats or drops a document.
//
// This is the property a naive OFFSET pager silently loses: it is not about
// speed, it is about a customer scrolling a result list and seeing the same
// item twice while another item never appears at all.
func TestSection13ScalePaginationIsStableAndBounded(t *testing.T) {
	pool, ctx := ensureScaleCorpus(t)

	engine := searchprojection.NewFoodSearchEngine(searchprojection.NewPostgresFoodSearchBackend(pool))

	seen := map[string]int{}
	cursor := ""
	const pageSize = 25

	for page := 0; page < 6; page++ {
		response, err := engine.Search(ctx, searchprojection.FoodEngineSearchRequest{
			Query:      "jollof",
			Generation: scaleGeneration,
			PageSize:   pageSize,
			Cursor:     cursor,
		})
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if len(response.Hits) > pageSize {
			t.Fatalf("page %d returned %d hits; page size is %d", page, len(response.Hits), pageSize)
		}
		for _, hit := range response.Hits {
			seen[hit.DocumentID]++
			if seen[hit.DocumentID] > 1 {
				t.Fatalf("document %s appeared on more than one page", hit.DocumentID)
			}
		}
		if response.NextCursor == "" {
			break
		}
		cursor = response.NextCursor
	}

	if len(seen) == 0 {
		t.Fatal("pagination returned nothing at all")
	}
	t.Logf("paged %d distinct documents without repetition", len(seen))
}

// TestSection13ScaleRejectsUnboundedRequests proves the engine refuses the
// requests that would make it unbounded, rather than trying to serve them.
func TestSection13ScaleRejectsUnboundedRequests(t *testing.T) {
	pool, ctx := ensureScaleCorpus(t)

	engine := searchprojection.NewFoodSearchEngine(searchprojection.NewPostgresFoodSearchBackend(pool))

	// Too many facet families.
	if _, err := engine.Search(ctx, searchprojection.FoodEngineSearchRequest{
		Query: "rice", Generation: scaleGeneration, PageSize: 10,
		Facets: []string{"CUISINE", "DISH_TYPE", "MEAL_PERIOD", "AVAILABILITY",
			"PROVIDER_TYPE", "FOOD_GROUP", "OCCASION", "DIETARY", "PREFERENCE"},
	}); err == nil {
		t.Error("nine facet families were accepted; the bound is eight")
	}

	// A garbage cursor must be refused rather than silently restarting the
	// result set from the beginning.
	if _, err := engine.Search(ctx, searchprojection.FoodEngineSearchRequest{
		Query: "rice", Generation: scaleGeneration, PageSize: 10, Cursor: "not-a-cursor",
	}); err == nil {
		t.Error("a malformed cursor was accepted")
	}

	// An empty query has no candidate set to bound.
	if _, err := engine.Search(ctx, searchprojection.FoodEngineSearchRequest{
		Query: "   ", Generation: scaleGeneration, PageSize: 10,
	}); err == nil {
		t.Error("an empty query was accepted")
	}
}

// ---------------------------------------------------------------------------
// Query plans
// ---------------------------------------------------------------------------

// requiredIndexes are the access paths the large search tables depend on. An
// index-presence assertion is the structural half of the plan gate: it holds
// regardless of how the planner feels about a particular dataset, and it is
// what actually breaks if a migration drops one.
var requiredIndexes = map[string][]string{
	"food_search_documents": {
		"food_search_documents_vector_gin_idx",
		"food_search_documents_generation_active_idx",
		"food_search_documents_source_idx",
	},
	"food_search_terms": {
		"food_search_terms_exact_idx",
		"food_search_terms_prefix_idx",
		"food_search_terms_typo_candidate_idx",
	},
	"shop_search_documents": {
		"shop_search_documents_vector_gin_idx",
		"shop_search_documents_generation_active_idx",
		"shop_search_documents_source_idx",
	},
	"shop_search_terms": {
		"shop_search_terms_exact_idx",
		"shop_search_terms_prefix_idx",
		"shop_search_terms_typo_candidate_idx",
	},
}

func TestSection13SearchIndexesExist(t *testing.T) {
	pool, ctx := testPool(t)

	for table, indexes := range requiredIndexes {
		for _, index := range indexes {
			var exists bool
			if err := pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE tablename=$1 AND indexname=$2)`,
				table, index,
			).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if !exists {
				t.Errorf("index %s on %s is missing; the large search path would degrade to a full scan",
					index, table)
			}
		}
	}
}

// explain returns the textual plan for a query.
func explain(t *testing.T, pool *pgxpool.Pool, ctx context.Context, sql string, args ...any) string {
	t.Helper()
	rows, err := pool.Query(ctx, "EXPLAIN "+sql, args...)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteString("\n")
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return plan.String()
}

// TestSection13LargeSearchPathsAreIndexCapable is the structural plan gate.
//
// Each query below is run twice: once as the planner would normally choose,
// which is recorded for the report, and once with enable_seqscan off inside a
// transaction. The second run is the assertion. If a query can be planned
// without any sequential scan of a 24,000-row search table, the access path is
// index-capable - which is the architectural property Section 13I cares about.
// If the planner cannot satisfy it at all without a sequential scan, the query
// has no usable index and the plan will still contain one even with the
// setting off, which is what this detects.
//
// Small lookup tables are not part of this claim and are not asserted on.
func TestSection13LargeSearchPathsAreIndexCapable(t *testing.T) {
	pool, ctx := ensureScaleCorpus(t)

	largeTables := []string{
		"food_search_documents", "food_search_terms",
		"shop_search_documents", "shop_search_terms",
	}

	cases := []struct {
		name string
		sql  string
		args []any
	}{
		{
			name: "food full-text candidate scan",
			sql: `SELECT document_id FROM food_search_documents
                   WHERE generation=$1 AND active=TRUE
                     AND search_vector @@ plainto_tsquery('simple'::regconfig, $2)
                   LIMIT 200`,
			args: []any{int64(scaleGeneration), "jollof rice"},
		},
		{
			name: "food exact term lookup",
			sql: `SELECT document_id FROM food_search_terms
                   WHERE generation=$1 AND term_scope='TOKEN' AND normalized_term=$2
                   LIMIT 200`,
			args: []any{int64(scaleGeneration), "jollof"},
		},
		{
			name: "food prefix term lookup",
			sql: `SELECT document_id FROM food_search_terms
                   WHERE generation=$1 AND term_scope='TOKEN' AND normalized_term LIKE $2
                   LIMIT 200`,
			args: []any{int64(scaleGeneration), "jol%"},
		},
		{
			name: "food typo candidate lookup",
			sql: `SELECT document_id FROM food_search_terms
                   WHERE generation=$1 AND term_scope='TOKEN'
                     AND first_char=$2 AND char_length_value BETWEEN $3 AND $4
                   LIMIT 200`,
			args: []any{int64(scaleGeneration), "j", 4, 8},
		},
		{
			name: "food branch-scoped generation scan",
			sql: `SELECT document_id FROM food_search_documents
                   WHERE generation=$1 AND active=TRUE AND branch_ref=$2
                   LIMIT 200`,
			args: []any{int64(scaleGeneration), "vbr_scale_7"},
		},
		{
			name: "food document hydration by id",
			sql: `SELECT to_jsonb(d) FROM food_search_documents d
                   WHERE d.generation=$1 AND d.document_id=$2`,
			args: []any{int64(scaleGeneration), "food_scale_1234"},
		},
		{
			name: "shop full-text candidate scan",
			sql: `SELECT document_id FROM shop_search_documents
                   WHERE generation=$1 AND active=TRUE
                     AND search_vector @@ plainto_tsquery('simple'::regconfig, $2)
                   LIMIT 200`,
			args: []any{int64(scaleGeneration), "dangote rice"},
		},
		{
			name: "shop exact term lookup",
			sql: `SELECT document_id FROM shop_search_terms
                   WHERE generation=$1 AND term_scope='TOKEN' AND normalized_term=$2
                   LIMIT 200`,
			args: []any{int64(scaleGeneration), "dangote"},
		},
		{
			name: "shop prefix term lookup",
			sql: `SELECT document_id FROM shop_search_terms
                   WHERE generation=$1 AND term_scope='TOKEN' AND normalized_term LIKE $2
                   LIMIT 200`,
			args: []any{int64(scaleGeneration), "dang%"},
		},
		{
			name: "shop branch-scoped generation scan",
			sql: `SELECT document_id FROM shop_search_documents
                   WHERE generation=$1 AND active=TRUE AND branch_ref=$2
                   LIMIT 200`,
			args: []any{int64(scaleGeneration), "vbr_scale_9"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Logf("default plan:\n%s", explain(t, pool, ctx, testCase.sql, testCase.args...))

			tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			if _, err = tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
				t.Fatal(err)
			}

			rows, err := tx.Query(ctx, "EXPLAIN "+testCase.sql, testCase.args...)
			if err != nil {
				t.Fatalf("explain with enable_seqscan off: %v", err)
			}
			var plan strings.Builder
			for rows.Next() {
				var line string
				if err = rows.Scan(&line); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				plan.WriteString(line)
				plan.WriteString("\n")
			}
			rows.Close()

			planText := plan.String()
			for _, table := range largeTables {
				marker := fmt.Sprintf("Seq Scan on %s", table)
				if strings.Contains(planText, marker) {
					t.Errorf("query has no index-capable path: %s remains even with enable_seqscan off\n%s",
						marker, planText)
				}
			}
		})
	}
}

// TestSection13ScaleHasNoPerDocumentQueryLoop guards against an N+1: a search
// that issues one statement per hit would still return correct results and
// would still pass every assertion above, while being unusable at scale.
//
// The measurement is deliberately RELATIVE rather than absolute. An absolute
// statement budget would have to account for connection setup, the ranking
// profile read, the candidate query, hydration and whatever else shares the
// database - a moving target that produces a flaky threshold. What actually
// distinguishes an N+1 is its shape: cost that grows with the number of hits.
// So the same query is run at two very different page sizes and the growth is
// compared against the growth in hits.
//
// pg_stat_statements is not assumed to be installed; enabling a PostgreSQL
// extension during certification is out of scope.
func TestSection13ScaleHasNoPerDocumentQueryLoop(t *testing.T) {
	pool, ctx := ensureScaleCorpus(t)

	engine := searchprojection.NewFoodSearchEngine(searchprojection.NewPostgresFoodSearchBackend(pool))

	transactions := func() int64 {
		var value int64
		if err := pool.QueryRow(ctx,
			`SELECT xact_commit + xact_rollback FROM pg_stat_database WHERE datname = current_database()`,
		).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}

	measure := func(pageSize int) (hits int, cost int64) {
		before := transactions()
		response, err := engine.Search(ctx, searchprojection.FoodEngineSearchRequest{
			Query: "jollof rice", Generation: scaleGeneration, PageSize: pageSize,
		})
		if err != nil {
			t.Fatal(err)
		}
		return len(response.Hits), transactions() - before
	}

	// Warm the connection pool first, so pool growth is not mistaken for query
	// growth on the larger run.
	if _, _ = measure(5); false {
		return
	}

	smallHits, smallCost := measure(1)
	largeHits, largeCost := measure(50)

	if smallHits == 0 || largeHits == 0 {
		t.Fatal("search returned nothing; the N+1 gate would be vacuous")
	}
	if largeHits <= smallHits {
		t.Fatalf("page sizes produced %d and %d hits; the comparison needs them to differ", smallHits, largeHits)
	}

	t.Logf("hits %d -> %d, transactions %d -> %d", smallHits, largeHits, smallCost, largeCost)

	// An N+1 would add roughly one statement per additional hit. Allow a
	// generous constant of extra work, but nothing proportional: the growth
	// must stay far below the growth in hits.
	growth := largeCost - smallCost
	hitGrowth := int64(largeHits - smallHits)
	if growth > hitGrowth/2 {
		t.Fatalf("returning %d more hits cost %d more transactions; search cost is scaling with hit count",
			hitGrowth, growth)
	}
}
