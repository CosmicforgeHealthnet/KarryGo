package section13security

// Section 13I internal search API certification.
//
// The taxonomy HMAC gate (internal/features/taxonomy/section13i_hmac_test.go)
// proves the serviceauth chain itself. This file proves the SEARCH surface is
// behind the same chain, with its own separate allowlist, and that the request
// bounds and namespace separation hold at the HTTP boundary rather than only in
// the engine.
//
// The separate allowlist is the part worth being explicit about: a service
// cleared to run a discovery query is not thereby cleared to publish taxonomy,
// and the two surfaces are configured independently
// (VENDOR_SEARCH_INTERNAL_CALLERS and VENDOR_TAXONOMY_INTERNAL_CALLERS) so that
// separation is an operational fact rather than a convention.

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

	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
	"cosmicforge/logistics/services/vendor-service/internal/platform/servicenonce"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	searchCaller       = "order-service"
	searchDeniedCaller = "marketing-service"
)

var (
	searchSecret       = []byte("section13i-search-secret")
	searchDeniedSecret = []byte("section13i-marketing-secret")
)

// recordingSearch captures what the handler actually asked the engine for, so
// the tests can assert on the request the API constructed rather than only on
// the response it returned.
type recordingSearch struct {
	lastFood searchprojection.FoodEngineSearchRequest
	lastShop searchprojection.ShopEngineSearchRequest
	lastHint searchprojection.SuggestionRequest
}

func (r *recordingSearch) Search(
	_ context.Context, request searchprojection.FoodEngineSearchRequest,
) (searchprojection.FoodEngineSearchResponse, error) {
	r.lastFood = request
	return searchprojection.FoodEngineSearchResponse{
		ProfileCode:    searchprojection.DefaultFoodRankingProfile,
		ProfileVersion: 1,
		Hits: []searchprojection.FoodEngineHit{{
			DocumentID: "food_doc_1", BusinessRef: "vb", BranchRef: "vbr",
			FoodItemRef: "fi_1", Name: "Jollof Rice", Score: 100,
			PlacementType: searchprojection.PlacementOrganic,
		}},
	}, nil
}

func (r *recordingSearch) Suggest(
	_ context.Context, request searchprojection.SuggestionRequest,
) ([]searchprojection.SearchSuggestion, error) {
	r.lastHint = request
	return []searchprojection.SearchSuggestion{{Text: "Jollof Rice", NormalizedText: "jollof rice"}}, nil
}

type recordingShopSearch struct{ inner *recordingSearch }

func (r recordingShopSearch) Search(
	_ context.Context, request searchprojection.ShopEngineSearchRequest,
) (searchprojection.ShopEngineSearchResponse, error) {
	r.inner.lastShop = request
	return searchprojection.ShopEngineSearchResponse{
		ProfileCode:    searchprojection.DefaultShopRankingProfile,
		ProfileVersion: 1,
		Hits: []searchprojection.ShopEngineHit{{
			DocumentID: "shop_doc_1", BusinessRef: "vb", BranchRef: "vbr",
			ProductRef: "sp_1", Name: "Raw Rice 50kg", Score: 90,
			PlacementType: searchprojection.PlacementOrganic,
		}},
	}, nil
}

func (r recordingShopSearch) Suggest(
	_ context.Context, request searchprojection.SuggestionRequest,
) ([]searchprojection.SearchSuggestion, error) {
	r.inner.lastHint = request
	return []searchprojection.SearchSuggestion{{Text: "Raw Rice", NormalizedText: "raw rice"}}, nil
}

type searchHarness struct {
	router   *gin.Engine
	recorder *recordingSearch
}

func newSearchHarness(t *testing.T) *searchHarness {
	t.Helper()
	pool, _ := testPool(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	recorder := &recordingSearch{}
	flags := searchprojection.Flags{SearchEnabled: true, AdminEnabled: true}

	verifier := serviceauth.NewVerifier(serviceauth.Secrets{
		searchCaller:       searchSecret,
		searchDeniedCaller: searchDeniedSecret,
	}, 5*time.Minute)

	searchprojection.RegisterInternalSearchRoutes(
		router,
		searchprojection.NewInternalHandler(searchprojection.InternalHandlerOptions{
			Food:    recorder,
			Shop:    recordingShopSearch{inner: recorder},
			Ranking: searchprojection.NewPostgresRankingProfileStore(pool),
			Health:  searchprojection.NewPostgresProjectionHealthReader(pool, nil),
			Callers: []string{searchCaller},
			Nonces:  servicenonce.NewStore(pool),
			Flags:   flags,
		}),
		verifier,
		flags,
	)

	return &searchHarness{router: router, recorder: recorder}
}

func (h *searchHarness) call(
	t *testing.T, method, target string, body []byte, caller string, secret []byte,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		request.Header.Set("Idempotency-Key",
			"section13i-search-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	}
	if err := serviceauth.SignRequest(request, caller, secret, body, time.Now()); err != nil {
		t.Fatalf("sign: %v", err)
	}
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, request)
	return recorder
}

// TestSearchSurfaceRequiresHMACAndItsOwnAllowlist proves the search surface is
// behind the same chain as taxonomy, with a separate allowlist.
func TestSearchSurfaceRequiresHMACAndItsOwnAllowlist(t *testing.T) {
	harness := newSearchHarness(t)
	body := []byte(`{"query":"jollof"}`)

	// Unsigned.
	unsigned := httptest.NewRequest(http.MethodPost, "/internal/vendor/v1/search/food", bytes.NewReader(body))
	unsigned.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, unsigned)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned search status=%d want=401", recorder.Code)
	}

	// Authentic but not on the SEARCH allowlist.
	if response := harness.call(t, http.MethodPost, "/internal/vendor/v1/search/food",
		body, searchDeniedCaller, searchDeniedSecret); response.Code != http.StatusForbidden {
		t.Fatalf("non-allowlisted caller status=%d want=403", response.Code)
	}

	// Allowlisted.
	if response := harness.call(t, http.MethodPost, "/internal/vendor/v1/search/food",
		body, searchCaller, searchSecret); response.Code != http.StatusOK {
		t.Fatalf("allowlisted caller status=%d want=200 body=%s", response.Code, response.Body.String())
	}
}

// TestSearchCallerCannotChooseTheGeneration is the derived-state guarantee at
// the API boundary: a caller must read what the platform has activated, never
// pin a retired or half-built projection.
func TestSearchCallerCannotChooseTheGeneration(t *testing.T) {
	harness := newSearchHarness(t)

	// A body naming a generation is refused outright, because the request
	// shape has no such field and unknown fields are rejected.
	if response := harness.call(t, http.MethodPost, "/internal/vendor/v1/search/food",
		[]byte(`{"query":"jollof","generation":7}`), searchCaller, searchSecret,
	); response.Code != http.StatusBadRequest {
		t.Fatalf("generation in body status=%d want=400 body=%s", response.Code, response.Body.String())
	}

	// A legitimate request leaves generation at zero, which is what makes the
	// generation-aware layer above the engine fill in the ACTIVE one.
	if response := harness.call(t, http.MethodPost, "/internal/vendor/v1/search/food",
		[]byte(`{"query":"jollof"}`), searchCaller, searchSecret); response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if harness.recorder.lastFood.Generation != 0 {
		t.Fatalf("handler set generation=%d; it must leave the choice to the generation-aware layer",
			harness.recorder.lastFood.Generation)
	}
}

// TestSearchNamespaceFiltersDoNotCross proves the API refuses a Shop filter
// block on the Food path and the reverse, rather than ignoring it.
func TestSearchNamespaceFiltersDoNotCross(t *testing.T) {
	harness := newSearchHarness(t)

	if response := harness.call(t, http.MethodPost, "/internal/vendor/v1/search/food",
		[]byte(`{"query":"rice","shop_filters":{"Brands":["dangote"]}}`),
		searchCaller, searchSecret,
	); response.Code != http.StatusBadRequest {
		t.Errorf("Shop filters on the Food path status=%d want=400", response.Code)
	}

	if response := harness.call(t, http.MethodPost, "/internal/vendor/v1/search/shop",
		[]byte(`{"query":"rice","food_filters":{"Cuisines":["yoruba"]}}`),
		searchCaller, searchSecret,
	); response.Code != http.StatusBadRequest {
		t.Errorf("Food filters on the Shop path status=%d want=400", response.Code)
	}
}

// TestSearchRequestsAreBounded proves the API clamps or refuses every
// unbounded request shape rather than passing it to the engine.
func TestSearchRequestsAreBounded(t *testing.T) {
	harness := newSearchHarness(t)

	// An over-large page size is clamped, not honoured.
	if response := harness.call(t, http.MethodPost, "/internal/vendor/v1/search/food",
		[]byte(`{"query":"rice","page_size":5000}`), searchCaller, searchSecret,
	); response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got := harness.recorder.lastFood.PageSize; got > 50 {
		t.Errorf("page size reached the engine as %d; the bound is 50", got)
	}

	// An over-long query is refused.
	long := make([]byte, 0, 400)
	for len(long) < 300 {
		long = append(long, 'a')
	}
	if response := harness.call(t, http.MethodPost, "/internal/vendor/v1/search/food",
		[]byte(`{"query":"`+string(long)+`"}`), searchCaller, searchSecret,
	); response.Code != http.StatusBadRequest {
		t.Errorf("over-long query status=%d want=400", response.Code)
	}

	// An empty query is refused rather than matching everything.
	if response := harness.call(t, http.MethodPost, "/internal/vendor/v1/search/food",
		[]byte(`{"query":"   "}`), searchCaller, searchSecret,
	); response.Code != http.StatusBadRequest {
		t.Errorf("empty query status=%d want=400", response.Code)
	}

	// Suggestion limits are clamped.
	if response := harness.call(t, http.MethodGet,
		"/internal/vendor/v1/search/food/suggestions?q=jol&limit=900",
		nil, searchCaller, searchSecret,
	); response.Code != http.StatusOK {
		t.Fatalf("suggestions status=%d body=%s", response.Code, response.Body.String())
	}
	if got := harness.recorder.lastHint.Limit; got > 10 {
		t.Errorf("suggestion limit reached the engine as %d; the bound is 10", got)
	}

	// Too many facet families are truncated to the bound before the engine
	// ever sees them.
	if response := harness.call(t, http.MethodPost, "/internal/vendor/v1/search/food/facets",
		[]byte(`{"query":"rice","facets":["CUISINE","DISH_TYPE","MEAL_PERIOD","AVAILABILITY","PROVIDER_TYPE","FOOD_GROUP","OCCASION","DIETARY","PREFERENCE","EXTRA"]}`),
		searchCaller, searchSecret,
	); response.Code != http.StatusOK {
		t.Fatalf("facets status=%d body=%s", response.Code, response.Body.String())
	}
	if got := len(harness.recorder.lastFood.Facets); got > 8 {
		t.Errorf("%d facet families reached the engine; the bound is 8", got)
	}
}

// TestSearchResponseIsMarkedDerivedAndCarriesNoInternalPayload checks what
// leaves the service.
//
// A consumer must be able to see that a hit is derived state, and must NOT be
// able to reconstruct the projected document: search text, token lists,
// normalised vocabulary and the raw operational hint payloads stay inside.
func TestSearchResponseIsMarkedDerivedAndCarriesNoInternalPayload(t *testing.T) {
	harness := newSearchHarness(t)

	response := harness.call(t, http.MethodPost, "/internal/vendor/v1/search/food",
		[]byte(`{"query":"jollof"}`), searchCaller, searchSecret)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			NamespaceCode string `json:"namespace_code"`
			Derived       bool   `json:"derived"`
			Hits          []struct {
				DocumentID    string `json:"document_id"`
				PlacementType string `json:"placement_type"`
			} `json:"hits"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !envelope.Success || !envelope.Data.Derived {
		t.Fatal("response is not marked as derived state")
	}
	if envelope.Data.NamespaceCode != string(searchprojection.SearchCacheNamespaceFood) {
		t.Fatalf("namespace_code=%q want=FOOD", envelope.Data.NamespaceCode)
	}
	if len(envelope.Data.Hits) == 0 || envelope.Data.Hits[0].PlacementType == "" {
		t.Fatal("hits do not carry an ORGANIC/SPONSORED placement")
	}

	// Fields that must never appear in a search response.
	raw := response.Body.String()
	for _, forbidden := range []string{
		"search_text", "search_tokens", "normalized_name",
		"availability_summary", "trust_hint", "reliability_hint",
		"canonical_taxonomy", "handling_metadata",
	} {
		if bytes.Contains([]byte(raw), []byte(forbidden)) {
			t.Errorf("search response exposes internal projection field %q", forbidden)
		}
	}
}

// TestSearchAdministrationRespectsItsOwnFlag proves the admin flag stands
// administration down without taking discovery offline with it.
func TestSearchAdministrationRespectsItsOwnFlag(t *testing.T) {
	pool, _ := testPool(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()

	flags := searchprojection.Flags{SearchEnabled: true, AdminEnabled: false}
	searchprojection.RegisterInternalSearchRoutes(
		router,
		searchprojection.NewInternalHandler(searchprojection.InternalHandlerOptions{
			Food:    &recordingSearch{},
			Shop:    recordingShopSearch{inner: &recordingSearch{}},
			Ranking: searchprojection.NewPostgresRankingProfileStore(pool),
			Health:  searchprojection.NewPostgresProjectionHealthReader(pool, nil),
			Callers: []string{searchCaller},
			Nonces:  servicenonce.NewStore(pool),
			Flags:   flags,
		}),
		serviceauth.NewVerifier(serviceauth.Secrets{searchCaller: searchSecret}, 5*time.Minute),
		flags,
	)

	harness := &searchHarness{router: router}

	// Discovery still answers.
	if response := harness.call(t, http.MethodPost, "/internal/vendor/v1/search/food",
		[]byte(`{"query":"jollof"}`), searchCaller, searchSecret); response.Code != http.StatusOK {
		t.Fatalf("search with administration disabled status=%d want=200", response.Code)
	}

	// Administration reports unavailable rather than forbidden: a disabled
	// rollout is not an authorization failure and must not read like one.
	if response := harness.call(t, http.MethodGet,
		"/internal/vendor/v1/search/ranking-profiles/FOOD",
		nil, searchCaller, searchSecret); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("ranking administration status=%d want=503", response.Code)
	}
}

// TestProjectionHealthSeparatesCacheFromCorrectness pins the operational
// distinction that matters during an incident: Redis being unreachable is a
// degraded cache, not unavailable search.
func TestProjectionHealthSeparatesCacheFromCorrectness(t *testing.T) {
	harness := newSearchHarness(t)

	response := harness.call(t, http.MethodGet, "/internal/vendor/v1/search/projection-health",
		nil, searchCaller, searchSecret)
	if response.Code != http.StatusOK {
		t.Fatalf("health status=%d body=%s", response.Code, response.Body.String())
	}

	var envelope struct {
		Data struct {
			Food  searchprojection.NamespaceHealth `json:"food"`
			Shop  searchprojection.NamespaceHealth `json:"shop"`
			Cache struct {
				Available     bool `json:"available"`
				Authoritative bool `json:"authoritative"`
				DegradedOnly  bool `json:"degraded_only"`
			} `json:"cache"`
			CorrectnessSource string `json:"search_correctness_source"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode health: %v", err)
	}

	// The harness passes a nil cache, so the cache reports unavailable - and
	// the health endpoint still answers 200 with both namespaces reported.
	if envelope.Data.Cache.Available {
		t.Error("a nil cache reported itself available")
	}
	if envelope.Data.Cache.Authoritative {
		t.Error("health declares the cache authoritative; Redis is never authoritative for search")
	}
	if !envelope.Data.Cache.DegradedOnly {
		t.Error("health does not mark cache loss as degradation-only")
	}
	if envelope.Data.CorrectnessSource != "postgresql_projection" {
		t.Errorf("correctness source=%q want=postgresql_projection", envelope.Data.CorrectnessSource)
	}
	if envelope.Data.Food.NamespaceCode != searchprojection.SearchCacheNamespaceFood ||
		envelope.Data.Shop.NamespaceCode != searchprojection.SearchCacheNamespaceShop {
		t.Error("health does not report both namespaces separately")
	}
}
