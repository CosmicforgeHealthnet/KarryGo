package shop

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type InventoryInternalHandler struct {
	service *Service
	allowed map[string]bool
}

func NewInventoryInternalHandler(service *Service, callers []string) *InventoryInternalHandler {
	allowed := map[string]bool{}
	for _, caller := range callers {
		caller = strings.TrimSpace(caller)
		if caller != "" {
			allowed[caller] = true
		}
	}
	return &InventoryInternalHandler{service: service, allowed: allowed}
}

func (h *InventoryInternalHandler) allowCaller(c *gin.Context) {
	value, _ := c.Get("service_name")
	caller, _ := value.(string)
	if !h.allowed[caller] {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	c.Next()
}
func (h *InventoryInternalHandler) requireNonce(c *gin.Context) {
	value, _ := c.Get("service_name")
	nonce := strings.TrimSpace(c.GetHeader(serviceauth.HeaderNonce))
	if nonce == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	tag, err := h.service.db.Exec(c, `INSERT INTO vendor_service_nonces(caller,nonce,expires_at) VALUES($1,$2,now()+interval '10 minutes') ON CONFLICT DO NOTHING`, value, nonce)
	if err != nil || tag.RowsAffected() != 1 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}
func internalInventoryOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}
func (h *InventoryInternalHandler) actorForBranch(c *gin.Context) (Actor, Scope, error) {
	var businessID string
	err := h.service.db.QueryRow(c, `SELECT business_id FROM vendor_branches WHERE id=$1`, c.Param("branchId")).Scan(&businessID)
	if err != nil {
		return Actor{}, Scope{}, errNotFound("Branch", CodeCapabilityNotSupported)
	}
	return Actor{VendorID: businessID, CorrelationID: httpx.GetRequestID(c)}, Scope{Global: true}, nil
}
func internalDecode(c *gin.Context, target any) error {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return apperrors.BadRequest("Request body is invalid.", err)
	}
	return nil
}

func parseInternalDiscovery(c *gin.Context) DiscoveryOptions {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	return DiscoveryOptions{Search: c.Query("q"), CategoryID: c.Query("category_id"), ProductID: c.Query("product_id"), Brand: c.Query("brand"), ProductClass: c.Query("product_class"), TemperatureZone: c.Query("temperature_zone"), OperatingModel: c.Query("operating_model"), InStockOnly: strings.EqualFold(c.Query("in_stock"), "true"), WholesaleOnly: strings.EqualFold(c.Query("wholesale"), "true"), Limit: limit, Offset: offset}
}
func (h *InventoryInternalHandler) catalogue(c *gin.Context) {
	actor, scope, err := h.actorForBranch(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items, more, catalogueRevision, inventoryRevision, err := h.service.DiscoverCatalogue(c, actor, scope, c.Param("branchId"), parseInternalDiscovery(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	unchanged := false
	if sinceCatalogue, _ := strconv.ParseInt(c.Query("since_catalogue_revision"), 10, 64); sinceCatalogue > 0 && sinceCatalogue == catalogueRevision {
		if sinceInventory, _ := strconv.ParseInt(c.Query("since_inventory_revision"), 10, 64); sinceInventory == inventoryRevision {
			items = nil
			unchanged = true
		}
	}
	internalInventoryOK(c, gin.H{"items": items, "has_more": more, "catalogue_revision": catalogueRevision, "inventory_revision": inventoryRevision, "unchanged": unchanged})
}
func (h *InventoryInternalHandler) product(c *gin.Context) {
	actor, scope, err := h.actorForBranch(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	options := parseInternalDiscovery(c)
	options.ProductID = c.Param("productId")
	items, _, catalogueRevision, inventoryRevision, err := h.service.DiscoverCatalogue(c, actor, scope, c.Param("branchId"), options)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	if len(items) == 0 {
		httpx.RespondError(c, errNotFound("Product", CodeProductNotFound))
		return
	}
	internalInventoryOK(c, gin.H{"items": items, "catalogue_revision": catalogueRevision, "inventory_revision": inventoryRevision})
}
func (h *InventoryInternalHandler) availability(c *gin.Context) {
	actor, scope, err := h.actorForBranch(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	item, err := h.service.EffectiveAvailability(c, actor, scope, c.Param("branchId"), c.Param("variantId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	internalInventoryOK(c, item)
}
func (h *InventoryInternalHandler) sellability(c *gin.Context) {
	actor, scope, err := h.actorForBranch(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	quantity, _ := strconv.ParseInt(c.DefaultQuery("quantity", "1"), 10, 64)
	item, err := h.service.ResolveSellability(c, actor, scope, c.Param("branchId"), c.Param("variantId"), c.Query("sell_unit_id"), quantity, strings.EqualFold(c.Query("channel"), "wholesale"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	internalInventoryOK(c, item)
}

type internalShopPriceRequest struct {
	ListingRef  string `json:"listing_ref"`
	VariantRef  string `json:"variant_ref"`
	SellUnitRef string `json:"sell_unit_ref"`
	Quantity    int64  `json:"quantity"`
	SalesMode   string `json:"sales_mode"`
}

func (h *InventoryInternalHandler) price(c *gin.Context) {
	actor, scope, err := h.actorForBranch(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	var in internalShopPriceRequest
	if err = internalDecode(c, &in); err != nil {
		httpx.RespondError(c, err)
		return
	}
	var branch string
	if err = h.service.db.QueryRow(c, `SELECT branch_id FROM shop_branch_listings WHERE id=$1 AND business_id=$2 AND variant_id=$3`, in.ListingRef, actor.VendorID, in.VariantRef).Scan(&branch); err != nil || branch != c.Param("branchId") {
		httpx.RespondError(c, errNotFound("Listing", CodeListingNotFound))
		return
	}
	price, err := h.service.ResolvePrice(c, actor, scope, in.ListingRef, in.SellUnitRef, in.Quantity, strings.EqualFold(in.SalesMode, "WHOLESALE"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	catalogueRevision, _, _ := h.service.CatalogueRevision(c, actor, scope, branch)
	inventoryRevision, _, _ := h.service.InventoryRevision(c, actor, scope, branch)
	internalInventoryOK(c, gin.H{"price": ResolvedPriceViewOf(price), "catalogue_revision": catalogueRevision, "inventory_revision": inventoryRevision})
}
func (h *InventoryInternalHandler) revision(c *gin.Context) {
	actor, scope, err := h.actorForBranch(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	catalogue, catAt, err := h.service.CatalogueRevision(c, actor, scope, c.Param("branchId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	inventory, invAt, err := h.service.InventoryRevision(c, actor, scope, c.Param("branchId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	internalInventoryOK(c, gin.H{"catalogue_revision": catalogue, "catalogue_updated_at": catAt, "inventory_revision": inventory, "inventory_updated_at": invAt})
}
func (h *InventoryInternalHandler) reserve(c *gin.Context) {
	actor, scope, err := h.actorForBranch(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	var in ReserveInventoryInput
	if err = internalDecode(c, &in); err != nil {
		httpx.RespondError(c, err)
		return
	}
	for _, line := range in.Lines {
		var basisBranch string
		if err = h.service.db.QueryRow(c, `SELECT branch_id FROM shop_inventory_stock_bases WHERE id=$1 AND business_id=$2`, line.StockBasisID, actor.VendorID).Scan(&basisBranch); err != nil || basisBranch != c.Param("branchId") {
			httpx.RespondError(c, errNotFound("Stock basis", CodeStockBasisNotFound))
			return
		}
	}
	item, err := h.service.ReserveInventory(c, actor, scope, in)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": item})
}
func (h *InventoryInternalHandler) transition(target string) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, scope, err := h.actorForBranch(c)
		if err != nil {
			httpx.RespondError(c, err)
			return
		}
		existing, err := h.service.GetInventoryReservation(c, actor, scope, c.Param("reservationId"))
		if err != nil {
			httpx.RespondError(c, err)
			return
		}
		for _, line := range existing.Lines {
			if line.BranchRef != c.Param("branchId") {
				httpx.RespondError(c, errNotFound("Reservation", CodeReservationNotFound))
				return
			}
		}
		var item InventoryReservation
		switch target {
		case ReservationConfirmed:
			item, err = h.service.ConfirmReservation(c, actor, scope, c.Param("reservationId"))
		case ReservationCommitted:
			item, err = h.service.CommitReservation(c, actor, scope, c.Param("reservationId"))
		default:
			item, err = h.service.ReleaseReservation(c, actor, scope, c.Param("reservationId"))
		}
		if err != nil {
			httpx.RespondError(c, err)
			return
		}
		internalInventoryOK(c, item)
	}
}

func RegisterInternalInventoryRoutes(router *gin.Engine, h *InventoryInternalHandler, verifier *serviceauth.Verifier, flags Flags) {
	if h == nil || verifier == nil || !flags.ShopEnabled {
		return
	}
	group := router.Group("/internal/vendor/v1/shop")
	group.Use(serviceauth.Middleware(verifier), h.allowCaller, h.requireNonce)
	branches := group.Group("/branches/:branchId")
	branches.GET("/catalogue", h.catalogue)
	branches.GET("/search", h.catalogue)
	branches.GET("/products/:productId", h.product)
	branches.GET("/variants/:variantId/availability", h.availability)
	branches.GET("/variants/:variantId/sellability", h.sellability)
	branches.POST("/pricing/resolve", h.price)
	branches.GET("/revision", h.revision)
	branches.POST("/inventory/reservations", h.reserve)
	branches.POST("/inventory/reservations/:reservationId/confirm", h.transition(ReservationConfirmed))
	branches.POST("/inventory/reservations/:reservationId/commit", h.transition(ReservationCommitted))
	branches.POST("/inventory/reservations/:reservationId/release", h.transition(ReservationReleased))
}
