package shop

import (
	"cosmicforge/logistics/services/vendor-service/internal/publictrust"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"
)

// Public customer Shop discovery (Karryam V4 §14.1, B13).
//
// The Section 12 discovery projection is internal: it carries physical
// on-hand, reserved, quarantined and in-transit quantities and stock-basis
// references that exist for trusted services. The public view below keeps
// everything a customer needs to choose and order - product, variant, sell
// units, retail/wholesale prices, MOQ and order multiple, handling flags, fresh
// preference vocabulary - and replaces inventory numbers with a stock state.

// Customer stock states. Quantities never leave the service.
const (
	StockAvailable   = "AVAILABLE"
	StockUnavailable = "UNAVAILABLE"
	StockPreorder    = "PREORDER"
	StockBackorder   = "BACKORDER"
)

type CustomerSellability struct {
	Sellable          bool      `json:"sellable"`
	ReasonCode        string    `json:"reason_code,omitempty"`
	StockState        string    `json:"stock_state"`
	CatalogueRevision int64     `json:"catalogue_revision"`
	InventoryRevision int64     `json:"inventory_revision"`
	EvaluatedAt       time.Time `json:"evaluated_at"`
}

type CustomerDiscoveryItem struct {
	CategoryRef        string                    `json:"category_ref"`
	CategoryName       string                    `json:"category_name"`
	ProductRef         string                    `json:"product_ref"`
	ProductName        string                    `json:"product_name"`
	Brand              string                    `json:"brand,omitempty"`
	ProductClass       string                    `json:"product_class"`
	VariantRef         string                    `json:"variant_ref"`
	VariantName        string                    `json:"variant_name"`
	TemperatureZone    string                    `json:"temperature_zone"`
	ListingRef         string                    `json:"listing_ref"`
	OperatingModel     string                    `json:"operating_model,omitempty"`
	RetailEnabled      bool                      `json:"retail_enabled"`
	WholesaleEnabled   bool                      `json:"wholesale_enabled"`
	StartingPriceMinor *int64                    `json:"starting_price_minor"`
	Sellability        CustomerSellability       `json:"sellability"`
	SellUnits          []DiscoverySellUnit       `json:"sell_units"`
	HandlingFlags      []string                  `json:"handling_flags"`
	FreshAttributes    []DiscoveryFreshAttribute `json:"fresh_attributes"`
}

// customerStockState derives the public state from the authoritative
// sellability answer alone.
func customerStockState(result SellabilityResult) string {
	switch {
	case result.Sellable && result.Availability.PhysicalAvailable > 0:
		return StockAvailable
	case result.Sellable && result.PreorderBackorderMode == "PREORDER":
		return StockPreorder
	case result.Sellable && result.PreorderBackorderMode == "BACKORDER":
		return StockBackorder
	case result.Sellable:
		return StockAvailable
	default:
		return StockUnavailable
	}
}

// CustomerViewOf maps one internal discovery item onto the public view.
func CustomerViewOf(item DiscoveryItem) CustomerDiscoveryItem {
	return CustomerDiscoveryItem{
		CategoryRef: item.CategoryRef, CategoryName: item.CategoryName,
		ProductRef: item.ProductRef, ProductName: item.ProductName, Brand: item.Brand,
		ProductClass: item.ProductClass, VariantRef: item.VariantRef, VariantName: item.VariantName,
		TemperatureZone: item.TemperatureZone, ListingRef: item.ListingRef,
		OperatingModel: item.OperatingModel, RetailEnabled: item.RetailEnabled,
		WholesaleEnabled: item.WholesaleEnabled, StartingPriceMinor: item.StartingPriceMinor,
		Sellability: CustomerSellability{
			Sellable: item.Sellability.Sellable, ReasonCode: item.Sellability.ReasonCode,
			StockState:        customerStockState(item.Sellability),
			CatalogueRevision: item.Sellability.CatalogueRevision,
			InventoryRevision: item.Sellability.InventoryRevision,
			EvaluatedAt:       item.Sellability.EvaluatedAt,
		},
		SellUnits: item.SellUnits, HandlingFlags: item.HandlingFlags, FreshAttributes: item.FreshAttributes,
	}
}

func customerViews(items []DiscoveryItem) []CustomerDiscoveryItem {
	out := make([]CustomerDiscoveryItem, 0, len(items))
	for _, item := range items {
		out = append(out, CustomerViewOf(item))
	}
	return out
}

// publicBranch refuses any branch whose branch or business is not ACTIVE.
func (h *InventoryInternalHandler) publicBranch(c *gin.Context) {
	var branchStatus, businessStatus string
	err := h.service.db.QueryRow(c, `SELECT br.status,b.status FROM vendor_branches br
		JOIN vendor_businesses b ON b.id=br.business_id WHERE br.id=$1`, c.Param("branchId")).Scan(&branchStatus, &businessStatus)
	if err != nil || branchStatus != "ACTIVE" || businessStatus != "ACTIVE" {
		httpx.RespondError(c, errNotFound("Branch", CodeCapabilityNotSupported))
		c.Abort()
		return
	}
	c.Next()
}

func (h *InventoryInternalHandler) customerCatalogue(c *gin.Context) {
	actor, scope, err := h.actorForBranch(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	options := parseInternalDiscovery(c)
	if options.Limit <= 0 || options.Limit > 50 {
		options.Limit = 50
	}
	if options.Offset < 0 || options.Offset > 10000 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	items, more, catalogueRevision, inventoryRevision, err := h.service.DiscoverCatalogue(c, actor, scope, c.Param("branchId"), options)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	facts, err := publictrust.Read(c, h.service.db, actor.VendorID, c.Param("branchId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"trust": facts,
		"items": customerViews(items), "has_more": more,
		"catalogue_revision": catalogueRevision, "inventory_revision": inventoryRevision,
	}})
}

func (h *InventoryInternalHandler) customerProduct(c *gin.Context) {
	actor, scope, err := h.actorForBranch(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	options := parseInternalDiscovery(c)
	options.ProductID = c.Param("productId")
	if options.Limit <= 0 || options.Limit > 50 {
		options.Limit = 50
	}
	if options.Offset < 0 || options.Offset > 10000 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	items, _, catalogueRevision, inventoryRevision, err := h.service.DiscoverCatalogue(c, actor, scope, c.Param("branchId"), options)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	if len(items) == 0 {
		httpx.RespondError(c, errNotFound("Product", CodeProductNotFound))
		return
	}
	facts, err := publictrust.Read(c, h.service.db, actor.VendorID, c.Param("branchId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"trust": facts,
		"items": customerViews(items), "catalogue_revision": catalogueRevision, "inventory_revision": inventoryRevision,
	}})
}

// RegisterCustomerDiscoveryRoutes mounts the public Shop reads under the
// customer commerce group (normally /api/v1/commerce).
func RegisterCustomerDiscoveryRoutes(group *gin.RouterGroup, handler *InventoryInternalHandler, guards ...gin.HandlerFunc) {
	if group == nil || handler == nil {
		return
	}
	chain := append(append([]gin.HandlerFunc{}, guards...), handler.publicBranch)
	routes := group.Group("/shop/branches/:branchId", chain...)
	routes.GET("/catalogue", handler.customerCatalogue)
	routes.GET("/products/:productId", handler.customerProduct)
}
