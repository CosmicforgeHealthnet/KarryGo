package searchprojection

// Section 13I internal search and projection administration API.
//
// Everything here is root-mounted under /internal/vendor/v1/search and
// authenticated with serviceauth HMAC-v2, an explicit caller allowlist and
// durable nonce replay protection. None of it is reachable with a vendor
// bearer token.
//
// Why search is internal rather than public: the consumers are other KarryGo
// services (the customer app's discovery surface reaches search through them),
// and the response carries cross-vendor results. A vendor authenticated
// against their own business has no business reading a ranked list of their
// competitors, so the surface is service-to-service and the caller allowlist
// decides which services may ask.
//
// Two properties are load-bearing throughout:
//
//   - Search is derived state. Every response is read from the PostgreSQL
//     projection at the ACTIVE generation. Nothing here can reserve stock,
//     price an order, or authorise a purchase; a hit is a pointer back to the
//     authoritative Food/Shop catalogue, which revalidates at checkout.
//   - FOOD and SHOP are separate engines over separate tables with separate
//     generations. There is no shared "search" endpoint that guesses which one
//     a caller meant, because a guess is how a Shop listing ends up in a Food
//     result.

import (
	"bytes"
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/publictrust"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Bounds on every request this surface accepts. They are constants rather than
// configuration because an unbounded page size, filter list or suggestion
// count is a denial-of-service surface, and the right value for "how many
// results may one request pull out of the projection" is not an operational
// dial.
const (
	MaxSearchPageSize    = 50
	DefaultSearchPage    = 20
	MaxSuggestionLimit   = 10
	MaxRequestedFacets   = 8
	MaxFilterValues      = 32
	MaxSearchQueryLength = 128
)

// Flags gate the Section 13 search rollout.
type Flags struct {
	SearchEnabled bool
	AdminEnabled  bool
}

// NonceStore consumes an HMAC nonce exactly once.
type NonceStore interface {
	Consume(ctx context.Context, caller string, nonce string) error
}

// FoodSearchAPI and ShopSearchAPI are the generation-aware entry points this
// surface serves from. They are interfaces so the handler is wired to
// GenerationAwareFoodSearch/GenerationAwareShopSearch in production and can be
// exercised with a stub in tests without a projection.
type FoodSearchAPI interface {
	Search(ctx context.Context, request FoodEngineSearchRequest) (FoodEngineSearchResponse, error)
	Suggest(ctx context.Context, request SuggestionRequest) ([]SearchSuggestion, error)
}

type ShopSearchAPI interface {
	Search(ctx context.Context, request ShopEngineSearchRequest) (ShopEngineSearchResponse, error)
	Suggest(ctx context.Context, request SuggestionRequest) ([]SearchSuggestion, error)
}

// RankingAdminAPI is the ranking profile administration port.
type RankingAdminAPI interface {
	ListRankingProfiles(ctx context.Context, namespace SearchCacheNamespace) ([]RankingProfile, error)
	GetRankingProfile(ctx context.Context, namespace SearchCacheNamespace, profileCode string) (RankingProfile, error)
	UpdateRankingProfile(ctx context.Context, namespace SearchCacheNamespace, profileCode string, update RankingWeightUpdate) (RankingProfile, error)
}

// ProjectionHealthReader supplies the bounded operational counters the health
// endpoint reports.
type ProjectionHealthReader interface {
	GenerationHealth(ctx context.Context, namespace SearchCacheNamespace) (NamespaceHealth, error)
	CacheAvailable(ctx context.Context) bool
}

// InternalHandler serves the internal search and administration surface.
type InternalHandler struct {
	publicTrust publictrust.Reader
	food        FoodSearchAPI
	shop        ShopSearchAPI
	ranking     RankingAdminAPI
	admin       *SearchProjectionAdminService
	health      ProjectionHealthReader

	allowed map[string]bool
	nonces  NonceStore
	flags   Flags
	limiter RequestLimiter
	limit   RequestLimitPolicy
}

type RequestLimiter interface {
	Allow(context.Context, string, string, int, time.Duration) error
}

type RequestLimitPolicy struct {
	Max    int
	Window time.Duration
}

type InternalHandlerOptions struct {
	Food    FoodSearchAPI
	Shop    ShopSearchAPI
	Ranking RankingAdminAPI
	Admin   *SearchProjectionAdminService
	Health  ProjectionHealthReader

	Callers []string
	Nonces  NonceStore
	Flags   Flags
	Limiter RequestLimiter
	Limit   RequestLimitPolicy
}

func NewInternalHandler(options InternalHandlerOptions) *InternalHandler {
	allowed := map[string]bool{}
	for _, caller := range options.Callers {
		if caller = strings.TrimSpace(caller); caller != "" {
			allowed[caller] = true
		}
	}
	return &InternalHandler{
		food:    options.Food,
		shop:    options.Shop,
		ranking: options.Ranking,
		admin:   options.Admin,
		health:  options.Health,
		allowed: allowed,
		nonces:  options.Nonces,
		flags:   options.Flags,
		limiter: options.Limiter,
		limit:   options.Limit,
	}
}

func (h *InternalHandler) limitSearch(c *gin.Context) {
	if h.limiter == nil || h.limit.Max <= 0 {
		c.Next()
		return
	}
	caller := callerName(c)
	if err := h.limiter.Allow(c.Request.Context(), "marketplace_search", caller, h.limit.Max, h.limit.Window); err != nil {
		httpx.RespondError(c, err)
		c.Abort()
		return
	}
	c.Next()
}

func (h *InternalHandler) allowCaller(c *gin.Context) {
	value, _ := c.Get("service_name")
	caller, _ := value.(string)
	if !h.allowed[caller] {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	c.Next()
}

func (h *InternalHandler) requireNonce(c *gin.Context) {
	value, _ := c.Get("service_name")
	caller, _ := value.(string)
	nonce := strings.TrimSpace(c.GetHeader(serviceauth.HeaderNonce))
	if nonce == "" || h.nonces == nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if err := h.nonces.Consume(c, caller, nonce); err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}

func respond(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"success": true, "data": data})
}

// respondError maps engine and administration errors onto the platform
// envelope. A generation or profile that does not exist inside the addressed
// namespace is a 404 rather than a 422, for the same non-enumeration reason
// the taxonomy surface uses.
func respondError(c *gin.Context, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, ErrInvalidArgument), errors.Is(err, ErrInvalidCursor),
		errors.Is(err, ErrRankingProfileInvalid), errors.Is(err, ErrSearchCacheConfigInvalid):
		httpx.RespondError(c, apperrors.BadRequest("Search request is invalid.", err))
	case errors.Is(err, ErrDistanceContextUnavailable):
		httpx.RespondError(c, apperrors.BadRequest("Distance sorting requires a distance context.", err))
	case errors.Is(err, ErrGenerationNotFound), errors.Is(err, ErrRankingProfileNotFound):
		httpx.RespondError(c, apperrors.NotFound("Search record not found.", nil))
	case errors.Is(err, ErrGenerationStateConflict), errors.Is(err, ErrGenerationActivationConflict),
		errors.Is(err, ErrProjectionEventCollision):
		httpx.RespondError(c, apperrors.Conflict("This generation is not in a state that allows this.", err))
	default:
		httpx.RespondError(c, apperrors.Internal("Search request failed.", err))
	}
}

func decodeStrict(c *gin.Context, target any) error {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		return apperrors.BadRequest("Could not read request body.", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return apperrors.BadRequest("Request body is invalid.", err)
	}
	return nil
}

// namespaceParam validates the namespace named on an administration path.
func namespaceParam(c *gin.Context) (SearchCacheNamespace, bool) {
	namespace := SearchCacheNamespace(strings.ToUpper(strings.TrimSpace(c.Param("namespaceCode"))))
	if !validSearchNamespace(namespace) {
		respondError(c, ErrGenerationNotFound)
		return "", false
	}
	return namespace, true
}

func (h *InternalHandler) requireAdminEnabled(c *gin.Context) bool {
	if !h.flags.AdminEnabled {
		httpx.RespondError(c, apperrors.Unavailable("Search projection administration is temporarily unavailable.", nil))
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Request shapes
// ---------------------------------------------------------------------------

// searchRequestBody is the wire shape both Food and Shop search accept.
//
// It carries no generation field. Callers do not choose which generation they
// read: the generation-aware engine resolves the ACTIVE one server-side. Being
// able to name a generation would let a caller pin a retired or half-built
// projection and serve results the platform has already replaced.
type searchRequestBody struct {
	Query string `json:"query"`

	BranchRef string `json:"branch_ref,omitempty"`

	// ProfileCode selects a ranking profile. It is accepted only from an
	// allowlisted internal caller - which is every caller that reaches this
	// handler - and is validated against the namespace by the engine.
	ProfileCode string `json:"profile_code,omitempty"`

	Facets []string `json:"facets,omitempty"`

	Sort string `json:"sort,omitempty"`

	PageSize int    `json:"page_size,omitempty"`
	Cursor   string `json:"cursor,omitempty"`

	DistanceContext *SearchDistanceContext `json:"distance_context,omitempty"`

	FoodFilters *FoodSearchFilters `json:"food_filters,omitempty"`
	ShopFilters *ShopSearchFilters `json:"shop_filters,omitempty"`
}

// boundedPageSize clamps rather than rejects, so a caller asking for more than
// the platform will serve gets the maximum instead of an error.
func boundedPageSize(requested int) int {
	switch {
	case requested <= 0:
		return DefaultSearchPage
	case requested > MaxSearchPageSize:
		return MaxSearchPageSize
	default:
		return requested
	}
}

func boundedFacets(requested []string) []string {
	if len(requested) > MaxRequestedFacets {
		return requested[:MaxRequestedFacets]
	}
	return requested
}

// validateQuery bounds the query text before it reaches the engine.
func validateQuery(query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", ErrInvalidArgument
	}
	if len(query) > MaxSearchQueryLength {
		return "", ErrInvalidArgument
	}
	return query, nil
}

// boundFilterValues caps every multi-valued filter. A filter list is a set of
// OR terms that widen the candidate scan, so an unbounded one is an unbounded
// query.
func boundFilterValues(values []string) []string {
	if len(values) > MaxFilterValues {
		return values[:MaxFilterValues]
	}
	return values
}

func boundFoodFilters(filters FoodSearchFilters) FoodSearchFilters {
	filters.ProviderTypes = boundFilterValues(filters.ProviderTypes)
	filters.Cuisines = boundFilterValues(filters.Cuisines)
	filters.MealPeriods = boundFilterValues(filters.MealPeriods)
	filters.DishTypes = boundFilterValues(filters.DishTypes)
	filters.FoodGroups = boundFilterValues(filters.FoodGroups)
	filters.Occasions = boundFilterValues(filters.Occasions)
	filters.DietaryClaims = boundFilterValues(filters.DietaryClaims)
	filters.PreferenceClaims = boundFilterValues(filters.PreferenceClaims)
	return filters
}

func boundShopFilters(filters ShopSearchFilters) ShopSearchFilters {
	filters.CanonicalCategories = boundFilterValues(filters.CanonicalCategories)
	filters.Brands = boundFilterValues(filters.Brands)
	filters.OperatingModels = boundFilterValues(filters.OperatingModels)
	filters.ProductClasses = boundFilterValues(filters.ProductClasses)
	filters.SellUnits = boundFilterValues(filters.SellUnits)
	filters.Measurements = boundFilterValues(filters.Measurements)
	filters.FreshAttributes = boundFilterValues(filters.FreshAttributes)
	filters.TemperatureZones = boundFilterValues(filters.TemperatureZones)
	return filters
}

// foodRequest converts a wire body into an engine request. Generation is left
// at zero deliberately: GenerationAwareFoodSearch fills it from the ACTIVE
// generation, and a zero here is what guarantees it.
func (body searchRequestBody) foodRequest() (FoodEngineSearchRequest, error) {
	query, err := validateQuery(body.Query)
	if err != nil {
		return FoodEngineSearchRequest{}, err
	}
	request := FoodEngineSearchRequest{
		Query:           query,
		BranchRef:       strings.TrimSpace(body.BranchRef),
		ProfileCode:     strings.TrimSpace(body.ProfileCode),
		Facets:          boundedFacets(body.Facets),
		Sort:            SearchSort(strings.TrimSpace(body.Sort)),
		PageSize:        boundedPageSize(body.PageSize),
		Cursor:          strings.TrimSpace(body.Cursor),
		DistanceContext: body.DistanceContext,
	}
	if body.FoodFilters != nil {
		request.Filters = boundFoodFilters(*body.FoodFilters)
	}
	return request, nil
}

func (body searchRequestBody) shopRequest() (ShopEngineSearchRequest, error) {
	query, err := validateQuery(body.Query)
	if err != nil {
		return ShopEngineSearchRequest{}, err
	}
	request := ShopEngineSearchRequest{
		Query:           query,
		BranchRef:       strings.TrimSpace(body.BranchRef),
		ProfileCode:     strings.TrimSpace(body.ProfileCode),
		Facets:          boundedFacets(body.Facets),
		Sort:            SearchSort(strings.TrimSpace(body.Sort)),
		PageSize:        boundedPageSize(body.PageSize),
		Cursor:          strings.TrimSpace(body.Cursor),
		DistanceContext: body.DistanceContext,
	}
	if body.ShopFilters != nil {
		request.Filters = boundShopFilters(*body.ShopFilters)
	}
	return request, nil
}

// ---------------------------------------------------------------------------
// Food search
// ---------------------------------------------------------------------------

func (h *InternalHandler) searchFood(c *gin.Context) {
	var body searchRequestBody
	if err := decodeStrict(c, &body); err != nil {
		httpx.RespondError(c, err)
		return
	}
	// A Shop filter block on the Food path is a namespace confusion, not a
	// harmless extra field: refusing it stops a caller sliding Shop
	// vocabulary into a Food query.
	if body.ShopFilters != nil {
		respondError(c, ErrInvalidArgument)
		return
	}
	request, err := body.foodRequest()
	if err != nil {
		respondError(c, err)
		return
	}
	response, err := h.food.Search(c, request)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, foodSearchResponseView(response))
}

func (h *InternalHandler) foodFacets(c *gin.Context) {
	var body searchRequestBody
	if err := decodeStrict(c, &body); err != nil {
		httpx.RespondError(c, err)
		return
	}
	if body.ShopFilters != nil {
		respondError(c, ErrInvalidArgument)
		return
	}
	request, err := body.foodRequest()
	if err != nil {
		respondError(c, err)
		return
	}
	// Facets are computed by the same engine over the same candidate set as
	// the search itself. A second, facet-only query engine would be free to
	// disagree with the result list it is meant to describe.
	request.PageSize = 1
	if len(request.Facets) == 0 {
		request.Facets = defaultFoodFacets()
	}
	response, err := h.food.Search(c, request)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{
		"namespace_code":  SearchCacheNamespaceFood,
		"profile_code":    response.ProfileCode,
		"profile_version": response.ProfileVersion,
		"facets":          response.Facets,
	})
}

func (h *InternalHandler) foodSuggestions(c *gin.Context) {
	request, err := suggestionRequest(c)
	if err != nil {
		respondError(c, err)
		return
	}
	suggestions, err := h.food.Suggest(c, request)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{
		"namespace_code": SearchCacheNamespaceFood,
		"suggestions":    suggestions,
	})
}

// ---------------------------------------------------------------------------
// Shop search
// ---------------------------------------------------------------------------

func (h *InternalHandler) searchShop(c *gin.Context) {
	var body searchRequestBody
	if err := decodeStrict(c, &body); err != nil {
		httpx.RespondError(c, err)
		return
	}
	if body.FoodFilters != nil {
		respondError(c, ErrInvalidArgument)
		return
	}
	request, err := body.shopRequest()
	if err != nil {
		respondError(c, err)
		return
	}
	response, err := h.shop.Search(c, request)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, shopSearchResponseView(response))
}

func (h *InternalHandler) shopFacets(c *gin.Context) {
	var body searchRequestBody
	if err := decodeStrict(c, &body); err != nil {
		httpx.RespondError(c, err)
		return
	}
	if body.FoodFilters != nil {
		respondError(c, ErrInvalidArgument)
		return
	}
	request, err := body.shopRequest()
	if err != nil {
		respondError(c, err)
		return
	}
	request.PageSize = 1
	if len(request.Facets) == 0 {
		request.Facets = defaultShopFacets()
	}
	response, err := h.shop.Search(c, request)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{
		"namespace_code":  SearchCacheNamespaceShop,
		"profile_code":    response.ProfileCode,
		"profile_version": response.ProfileVersion,
		"facets":          response.Facets,
	})
}

func (h *InternalHandler) shopSuggestions(c *gin.Context) {
	request, err := suggestionRequest(c)
	if err != nil {
		respondError(c, err)
		return
	}
	suggestions, err := h.shop.Suggest(c, request)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{
		"namespace_code": SearchCacheNamespaceShop,
		"suggestions":    suggestions,
	})
}

// suggestionRequest reads and bounds an autocomplete request.
//
// There is no personalisation input here and there is nowhere to put one: no
// customer id, no session, no history. Suggestions come from the canonical
// taxonomy, aliases, synonyms, brands and projected names, and nothing else.
func suggestionRequest(c *gin.Context) (SuggestionRequest, error) {
	query, err := validateQuery(c.Query("q"))
	if err != nil {
		return SuggestionRequest{}, err
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
	if limit <= 0 || limit > MaxSuggestionLimit {
		limit = MaxSuggestionLimit
	}
	return SuggestionRequest{Query: query, Limit: limit}, nil
}

// ---------------------------------------------------------------------------
// Response views
// ---------------------------------------------------------------------------

// foodHitView is what leaves the service for one Food hit.
//
// It carries stable references and the evidence a consumer needs to reason
// about freshness (projection revision, source evidence, generation), and
// nothing else. The raw projected document - search text, token lists,
// normalised vocabulary, operational hint payloads - stays inside: it is
// internal representation, and shipping it would let a consumer build a
// shadow catalogue out of search responses.
type foodHitView struct {
	DocumentID string `json:"document_id"`

	BusinessRef string `json:"business_ref"`
	BranchRef   string `json:"branch_ref"`
	FoodItemRef string `json:"food_item_ref"`

	Name  string `json:"name"`
	Score int64  `json:"score"`

	PriceMinor     *int64 `json:"price_minor,omitempty"`
	DistanceMeters *int64 `json:"distance_meters,omitempty"`

	Sponsored     bool          `json:"sponsored"`
	PlacementType PlacementType `json:"placement_type"`

	AvailabilityStatus   string `json:"availability_status"`
	ServiceabilityStatus string `json:"serviceability_status"`

	ProjectionRevision int64                `json:"projection_revision"`
	SourceEvidence     SearchSourceEvidence `json:"source_evidence"`
}

type foodSearchView struct {
	NamespaceCode SearchCacheNamespace `json:"namespace_code"`

	ProfileCode    string `json:"profile_code"`
	ProfileVersion int    `json:"profile_version"`

	Hits   []foodHitView `json:"hits"`
	Facets []SearchFacet `json:"facets,omitempty"`

	NextCursor string `json:"next_cursor,omitempty"`

	// Derived is stated on every response so no consumer can mistake a search
	// result for an authoritative availability or price guarantee.
	Derived bool `json:"derived"`
}

func foodSearchResponseView(response FoodEngineSearchResponse) foodSearchView {
	hits := make([]foodHitView, 0, len(response.Hits))
	for _, hit := range response.Hits {
		hits = append(hits, foodHitView{
			DocumentID:           hit.DocumentID,
			BusinessRef:          hit.BusinessRef,
			BranchRef:            hit.BranchRef,
			FoodItemRef:          hit.FoodItemRef,
			Name:                 hit.Name,
			Score:                hit.Score,
			PriceMinor:           hit.PriceMinor,
			DistanceMeters:       hit.DistanceMeters,
			Sponsored:            hit.Sponsored,
			PlacementType:        hit.PlacementType,
			AvailabilityStatus:   hit.AvailabilityStatus,
			ServiceabilityStatus: hit.ServiceabilityStatus,
			ProjectionRevision:   hit.ProjectionRevision,
			SourceEvidence:       hit.SourceEvidence,
		})
	}
	return foodSearchView{
		NamespaceCode:  SearchCacheNamespaceFood,
		ProfileCode:    response.ProfileCode,
		ProfileVersion: response.ProfileVersion,
		Hits:           hits,
		Facets:         response.Facets,
		NextCursor:     response.NextCursor,
		Derived:        true,
	}
}

type shopHitView struct {
	DocumentID string `json:"document_id"`

	BusinessRef string `json:"business_ref"`
	BranchRef   string `json:"branch_ref"`

	ProductRef string `json:"product_ref"`
	VariantRef string `json:"variant_ref,omitempty"`
	ListingRef string `json:"listing_ref,omitempty"`

	Name  string `json:"name"`
	Brand string `json:"brand,omitempty"`
	Score int64  `json:"score"`

	PriceMinor     *int64 `json:"price_minor,omitempty"`
	DistanceMeters *int64 `json:"distance_meters,omitempty"`

	Sponsored     bool          `json:"sponsored"`
	PlacementType PlacementType `json:"placement_type"`

	AvailabilityStatus   string `json:"availability_status"`
	ServiceabilityStatus string `json:"serviceability_status"`

	ProjectionRevision int64                `json:"projection_revision"`
	SourceEvidence     SearchSourceEvidence `json:"source_evidence"`
}

type shopSearchView struct {
	NamespaceCode SearchCacheNamespace `json:"namespace_code"`

	ProfileCode    string `json:"profile_code"`
	ProfileVersion int    `json:"profile_version"`

	Hits   []shopHitView `json:"hits"`
	Facets []SearchFacet `json:"facets,omitempty"`

	NextCursor string `json:"next_cursor,omitempty"`

	// A Shop hit reports stock as the projection saw it. It is not a
	// reservation and confers no right to it: Section 13F's checkout guard
	// revalidates against authoritative inventory before anything is held.
	Derived bool `json:"derived"`
}

func shopSearchResponseView(response ShopEngineSearchResponse) shopSearchView {
	hits := make([]shopHitView, 0, len(response.Hits))
	for _, hit := range response.Hits {
		hits = append(hits, shopHitView{
			DocumentID:           hit.DocumentID,
			BusinessRef:          hit.BusinessRef,
			BranchRef:            hit.BranchRef,
			ProductRef:           hit.ProductRef,
			VariantRef:           hit.VariantRef,
			ListingRef:           hit.ListingRef,
			Name:                 hit.Name,
			Brand:                hit.Brand,
			Score:                hit.Score,
			PriceMinor:           hit.PriceMinor,
			DistanceMeters:       hit.DistanceMeters,
			Sponsored:            hit.Sponsored,
			PlacementType:        hit.PlacementType,
			AvailabilityStatus:   hit.AvailabilityStatus,
			ServiceabilityStatus: hit.ServiceabilityStatus,
			ProjectionRevision:   hit.ProjectionRevision,
			SourceEvidence:       hit.SourceEvidence,
		})
	}
	return shopSearchView{
		NamespaceCode:  SearchCacheNamespaceShop,
		ProfileCode:    response.ProfileCode,
		ProfileVersion: response.ProfileVersion,
		Hits:           hits,
		Facets:         response.Facets,
		NextCursor:     response.NextCursor,
		Derived:        true,
	}
}

// ---------------------------------------------------------------------------
// Ranking administration
// ---------------------------------------------------------------------------

func (h *InternalHandler) listRankingProfiles(c *gin.Context) {
	if !h.requireAdminEnabled(c) {
		return
	}
	namespace, ok := namespaceParam(c)
	if !ok {
		return
	}
	profiles, err := h.ranking.ListRankingProfiles(c, namespace)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"namespace_code": namespace, "ranking_profiles": profiles})
}

func (h *InternalHandler) getRankingProfile(c *gin.Context) {
	if !h.requireAdminEnabled(c) {
		return
	}
	namespace, ok := namespaceParam(c)
	if !ok {
		return
	}
	profile, err := h.ranking.GetRankingProfile(c, namespace, c.Param("profileCode"))
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"ranking_profile": profile})
}

func (h *InternalHandler) updateRankingProfile(c *gin.Context) {
	if !h.requireAdminEnabled(c) {
		return
	}
	var update RankingWeightUpdate
	if err := decodeStrict(c, &update); err != nil {
		httpx.RespondError(c, err)
		return
	}
	namespace, ok := namespaceParam(c)
	if !ok {
		return
	}
	profile, err := h.ranking.UpdateRankingProfile(c, namespace, c.Param("profileCode"), update)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"ranking_profile": profile})
}

// ---------------------------------------------------------------------------
// Projection generation administration
// ---------------------------------------------------------------------------

type rebuildRequest struct {
	Scope    string `json:"scope,omitempty"`
	ScopeRef string `json:"scope_ref,omitempty"`
}

func (h *InternalHandler) requestRebuild(c *gin.Context) {
	if !h.requireAdminEnabled(c) {
		return
	}
	var request rebuildRequest
	if err := decodeStrict(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	namespace, ok := namespaceParam(c)
	if !ok {
		return
	}
	requestedBy := callerName(c)

	scope := SearchRebuildScope(strings.ToUpper(strings.TrimSpace(request.Scope)))
	if scope == "" || scope == SearchRebuildScopeFull {
		generation, err := h.admin.RequestFullRebuild(c, namespace, requestedBy)
		if err != nil {
			respondError(c, err)
			return
		}
		respond(c, http.StatusAccepted, gin.H{"generation": generationView(generation)})
		return
	}

	jobID, err := h.admin.RequestScopedRebuild(c, namespace, scope, strings.TrimSpace(request.ScopeRef), requestedBy)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusAccepted, gin.H{"job_id": jobID, "scope": scope})
}

func (h *InternalHandler) getGeneration(c *gin.Context) {
	if !h.requireAdminEnabled(c) {
		return
	}
	namespace, ok := namespaceParam(c)
	if !ok {
		return
	}
	generation, err := parseGeneration(c.Param("generation"))
	if err != nil {
		respondError(c, err)
		return
	}
	record, err := h.admin.InspectGeneration(c, namespace, generation)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"generation": generationView(record)})
}

func (h *InternalHandler) activeGeneration(c *gin.Context) {
	if !h.requireAdminEnabled(c) {
		return
	}
	namespace, ok := namespaceParam(c)
	if !ok {
		return
	}
	generation, err := h.admin.InspectActiveGeneration(c, namespace)
	if err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"namespace_code": namespace, "active_generation": generation})
}

// generationCommand covers retry, activate and retire, which differ only in
// which admin-service method they call.
func (h *InternalHandler) generationCommand(
	action func(*SearchProjectionAdminService, context.Context, SearchCacheNamespace, int64) error,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.requireAdminEnabled(c) {
			return
		}
		namespace, ok := namespaceParam(c)
		if !ok {
			return
		}
		generation, err := parseGeneration(c.Param("generation"))
		if err != nil {
			respondError(c, err)
			return
		}
		if err = action(h.admin, c, namespace, generation); err != nil {
			respondError(c, err)
			return
		}
		record, err := h.admin.InspectGeneration(c, namespace, generation)
		if err != nil {
			respondError(c, err)
			return
		}
		respond(c, http.StatusOK, gin.H{"generation": generationView(record)})
	}
}

func (h *InternalHandler) replayDeadLetter(c *gin.Context) {
	if !h.requireAdminEnabled(c) {
		return
	}
	if err := h.admin.ReplayDeadLetterJob(c, strings.TrimSpace(c.Param("jobId"))); err != nil {
		respondError(c, err)
		return
	}
	respond(c, http.StatusAccepted, gin.H{"replayed": true})
}

func parseGeneration(raw string) (int64, error) {
	generation, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || generation <= 0 {
		return 0, ErrInvalidArgument
	}
	return generation, nil
}

func callerName(c *gin.Context) string {
	value, _ := c.Get("service_name")
	caller, _ := value.(string)
	return caller
}

// generationRecordView is the bounded operator view of a generation.
//
// The stored record's source_revisions is included because reconciling a
// projection against its sources is exactly what an operator is here to do.
// Lease token is not: it is a capability another worker could use to steal a
// claim, and knowing a lease exists (worker id, expiry) is enough to diagnose
// a stuck rebuild.
type generationRecordView struct {
	NamespaceCode SearchCacheNamespace   `json:"namespace_code"`
	Generation    int64                  `json:"generation"`
	Status        SearchGenerationStatus `json:"status"`

	BaseGeneration int64 `json:"base_generation"`

	Scope    SearchRebuildScope `json:"scope,omitempty"`
	ScopeRef string             `json:"scope_ref,omitempty"`

	SourceRevisions json.RawMessage `json:"source_revisions,omitempty"`

	DocumentCount  int64 `json:"document_count"`
	ProcessedCount int64 `json:"processed_count"`

	FailureCode   string `json:"failure_code,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`

	RequestedBy string `json:"requested_by,omitempty"`

	StartedAt   *string `json:"started_at,omitempty"`
	CompletedAt *string `json:"completed_at,omitempty"`
	ValidatedAt *string `json:"validated_at,omitempty"`
	ActivatedAt *string `json:"activated_at,omitempty"`
	RetiredAt   *string `json:"retired_at,omitempty"`

	WorkerID       string  `json:"worker_id,omitempty"`
	LeaseExpiresAt *string `json:"lease_expires_at,omitempty"`
	HeartbeatAt    *string `json:"heartbeat_at,omitempty"`
}

func generationView(record SearchProjectionGeneration) generationRecordView {
	return generationRecordView{
		NamespaceCode:   record.Namespace,
		Generation:      record.Generation,
		Status:          record.Status,
		BaseGeneration:  record.BaseGeneration,
		Scope:           record.Scope,
		ScopeRef:        record.ScopeRef,
		SourceRevisions: record.SourceRevisions,
		DocumentCount:   record.DocumentCount,
		ProcessedCount:  record.ProcessedCount,
		FailureCode:     record.FailureCode,
		FailureReason:   record.FailureReason,
		RequestedBy:     record.RequestedBy,
		StartedAt:       formatTime(record.StartedAt),
		CompletedAt:     formatTime(record.CompletedAt),
		ValidatedAt:     formatTime(record.ValidatedAt),
		ActivatedAt:     formatTime(record.ActivatedAt),
		RetiredAt:       formatTime(record.RetiredAt),
		WorkerID:        record.WorkerID,
		LeaseExpiresAt:  formatTime(record.LeaseExpiresAt),
		HeartbeatAt:     formatTime(record.HeartbeatAt),
	}
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

func (h *InternalHandler) projectionHealth(c *gin.Context) {
	if h.health == nil {
		httpx.RespondError(c, apperrors.Unavailable("Projection health is unavailable.", nil))
		return
	}

	food, foodErr := h.health.GenerationHealth(c, SearchCacheNamespaceFood)
	shop, shopErr := h.health.GenerationHealth(c, SearchCacheNamespaceShop)
	if foodErr != nil || shopErr != nil {
		respondError(c, firstError(foodErr, shopErr))
		return
	}

	cacheAvailable := h.health.CacheAvailable(c)

	// Cache health is reported beside search health, never folded into it.
	// Redis being down degrades latency; it does not make search incorrect,
	// because every engine reads through to the PostgreSQL projection. An
	// operator who cannot tell those two states apart will fail over the
	// wrong component.
	respond(c, http.StatusOK, gin.H{
		"food": food,
		"shop": shop,
		"cache": gin.H{
			"available":     cacheAvailable,
			"authoritative": false,
			"degraded_only": true,
		},
		"search_correctness_source": "postgresql_projection",
	})
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

// RegisterInternalSearchRoutes mounts the internal search and administration
// surface at the engine root.
func RegisterInternalSearchRoutes(
	router *gin.Engine,
	h *InternalHandler,
	verifier *serviceauth.Verifier,
	flags Flags,
) {
	if h == nil || verifier == nil || !flags.SearchEnabled {
		return
	}

	group := router.Group("/internal/vendor/v1/search")
	group.Use(serviceauth.Middleware(verifier), h.allowCaller, h.requireNonce)

	// Food and Shop are separate route families over separate engines. There
	// is deliberately no combined endpoint.
	group.POST("/food", h.limitSearch, h.searchFood)
	group.POST("/food/facets", h.limitSearch, h.foodFacets)
	group.GET("/food/suggestions", h.limitSearch, h.foodSuggestions)

	group.POST("/shop", h.limitSearch, h.searchShop)
	group.POST("/shop/facets", h.limitSearch, h.shopFacets)
	group.GET("/shop/suggestions", h.limitSearch, h.shopSuggestions)

	group.GET("/ranking-profiles/:namespaceCode", h.listRankingProfiles)
	group.GET("/ranking-profiles/:namespaceCode/:profileCode", h.getRankingProfile)
	group.PATCH("/ranking-profiles/:namespaceCode/:profileCode", h.updateRankingProfile)

	group.POST("/projections/:namespaceCode/rebuilds", h.requestRebuild)
	group.GET("/projections/:namespaceCode/active-generation", h.activeGeneration)
	group.GET("/projections/:namespaceCode/generations/:generation", h.getGeneration)
	group.POST("/projections/:namespaceCode/generations/:generation/retry",
		h.generationCommand((*SearchProjectionAdminService).RetryFailedRebuild))
	group.POST("/projections/:namespaceCode/generations/:generation/activate",
		h.generationCommand((*SearchProjectionAdminService).ActivateValidatedGeneration))
	group.POST("/projections/:namespaceCode/generations/:generation/retire",
		h.generationCommand((*SearchProjectionAdminService).RetireOldGeneration))

	group.POST("/projection-jobs/:jobId/replay", h.replayDeadLetter)
	group.GET("/projection-health", h.projectionHealth)
}
