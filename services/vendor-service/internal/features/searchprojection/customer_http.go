package searchprojection

import (
	"cosmicforge/logistics/services/vendor-service/internal/publictrust"
	"net/http"

	"github.com/gin-gonic/gin"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

// Public customer search (Karryam V4 §14.1, B12/B13): vendor-first and
// product-first discovery over the SAME generation-aware engines the internal
// surface uses. Food and Shop stay separate route families. Two differences
// from the internal surface:
//
//   - a customer never selects a ranking profile; profile_code is refused,
//     so ranking stays a platform decision;
//   - the rate limit is per customer (bearer subject), not per service.

func (h *InternalHandler) limitCustomerSearch(c *gin.Context) {
	if h.limiter == nil || h.limit.Max <= 0 {
		c.Next()
		return
	}
	claims, _ := sharedauth.ClaimsFromContext(c)
	if err := h.limiter.Allow(c.Request.Context(), "customer_marketplace_search", "customer:"+claims.Subject, h.limit.Max, h.limit.Window); err != nil {
		httpx.RespondError(c, err)
		c.Abort()
		return
	}
	c.Next()
}

func (h *InternalHandler) customerBody(c *gin.Context, food bool) (searchRequestBody, bool) {
	var body searchRequestBody
	if err := decodeStrict(c, &body); err != nil {
		httpx.RespondError(c, err)
		return body, false
	}
	if body.ProfileCode != "" || (food && body.ShopFilters != nil) || (!food && body.FoodFilters != nil) {
		respondError(c, ErrInvalidArgument)
		return body, false
	}
	return body, true
}

func (h *InternalHandler) customerSearchFood(c *gin.Context) {
	body, ok := h.customerBody(c, true)
	if !ok {
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
	view := customerFoodSearchView(response)
	if err := h.addPublicTrust(c, view); err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, view)
}

func (h *InternalHandler) customerSearchShop(c *gin.Context) {
	body, ok := h.customerBody(c, false)
	if !ok {
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
	view := customerShopSearchView(response)
	if err := h.addPublicTrust(c, view); err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, view)
}

func (h *InternalHandler) SetPublicTrustReader(reader publictrust.Reader) { h.publicTrust = reader }

// Overlay current trust AFTER the search cache; badges never live in cached hits.
func (h *InternalHandler) addPublicTrust(c *gin.Context, view gin.H) error {
	c.Header("Cache-Control", "no-store")
	if h.publicTrust == nil {
		return nil
	}
	seen := map[[2]string][]publictrust.Fact{}
	for _, hit := range view["hits"].([]gin.H) {
		key := [2]string{hit["business_ref"].(string), hit["branch_ref"].(string)}
		facts, ok := seen[key]
		if !ok {
			var err error
			facts, err = h.publicTrust(c, key[0], key[1])
			if err != nil {
				return err
			}
			seen[key] = facts
		}
		hit["trust"] = facts
	}
	return nil
}

func customerFoodSearchView(response FoodEngineSearchResponse) gin.H {
	hits := make([]gin.H, 0, len(response.Hits))
	for _, h := range response.Hits {
		hits = append(hits, gin.H{"business_ref": h.BusinessRef, "branch_ref": h.BranchRef, "food_item_ref": h.FoodItemRef, "name": h.Name, "price_minor": h.PriceMinor, "distance_meters": h.DistanceMeters, "sponsored": h.Sponsored, "placement_type": h.PlacementType, "availability_status": h.AvailabilityStatus, "serviceability_status": h.ServiceabilityStatus})
	}
	return gin.H{"hits": hits, "facets": response.Facets, "next_cursor": response.NextCursor, "derived": true}
}

func customerShopSearchView(response ShopEngineSearchResponse) gin.H {
	hits := make([]gin.H, 0, len(response.Hits))
	for _, h := range response.Hits {
		hits = append(hits, gin.H{"business_ref": h.BusinessRef, "branch_ref": h.BranchRef, "product_ref": h.ProductRef, "variant_ref": h.VariantRef, "listing_ref": h.ListingRef, "name": h.Name, "brand": h.Brand, "price_minor": h.PriceMinor, "distance_meters": h.DistanceMeters, "sponsored": h.Sponsored, "placement_type": h.PlacementType, "availability_status": h.AvailabilityStatus, "serviceability_status": h.ServiceabilityStatus})
	}
	return gin.H{"hits": hits, "facets": response.Facets, "next_cursor": response.NextCursor, "derived": true}
}

func (h *InternalHandler) customerSuggestions(c *gin.Context, food bool) {
	if c.Query("profile_code") != "" {
		respondError(c, ErrInvalidArgument)
		return
	}
	request, err := suggestionRequest(c)
	if err != nil {
		respondError(c, err)
		return
	}
	var suggestions []SearchSuggestion
	if food {
		suggestions, err = h.food.Suggest(c, request)
	} else {
		suggestions, err = h.shop.Suggest(c, request)
	}
	if err != nil {
		respondError(c, err)
		return
	}
	views := make([]gin.H, 0, len(suggestions))
	for _, suggestion := range suggestions {
		views = append(views, gin.H{"text": suggestion.Text})
	}
	respond(c, http.StatusOK, gin.H{"suggestions": views})
}

// RegisterCustomerSearchRoutes mounts public search under the customer
// commerce group. foodGuards/shopGuards carry the customer bearer and the
// per-domain CUSTOMER_*_ENABLED gate.
func RegisterCustomerSearchRoutes(group *gin.RouterGroup, h *InternalHandler, flags Flags, foodGuards, shopGuards []gin.HandlerFunc) {
	if group == nil || h == nil || !flags.SearchEnabled {
		return
	}
	food := group.Group("/food/search", foodGuards...)
	food.POST("", h.limitCustomerSearch, h.customerSearchFood)
	food.GET("/suggestions", h.limitCustomerSearch, func(c *gin.Context) { h.customerSuggestions(c, true) })
	shop := group.Group("/shop/search", shopGuards...)
	shop.POST("", h.limitCustomerSearch, h.customerSearchShop)
	shop.GET("/suggestions", h.limitCustomerSearch, func(c *gin.Context) { h.customerSuggestions(c, false) })
}
