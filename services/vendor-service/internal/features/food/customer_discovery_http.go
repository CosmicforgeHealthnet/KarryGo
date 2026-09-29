package food

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/publictrust"
	"github.com/gin-gonic/gin"
	"time"

	"cosmicforge/logistics/shared/go/httpx"
)

// Customer views are explicit allowlists. The internal catalogue includes
// operational profiles, schedule references and staff audit/version fields.
func customerSellability(s Sellability) gin.H {
	return gin.H{"sellable": s.Sellable, "reason_code": s.ReasonCode, "evaluated_at": s.EvaluatedAt, "catalogue_revision": s.CatalogueRevision}
}

func customerCatalogueView(snapshot DiscoverySnapshot) gin.H {
	menus := make([]gin.H, 0, len(snapshot.Menus))
	for _, menu := range snapshot.Menus {
		sections := make([]gin.H, 0, len(menu.Sections))
		for _, section := range menu.Sections {
			items := make([]gin.H, 0, len(section.Items))
			for _, item := range section.Items {
				variants := make([]gin.H, 0, len(item.Variants))
				for _, v := range item.Variants {
					variants = append(variants, gin.H{"id": v.ID, "name": v.Name, "price_override_minor": v.PriceOverrideMinor, "inherits_base_price": v.InheritsBasePrice, "position": v.Position})
				}
				groups := make([]gin.H, 0, len(item.ModifierGroups))
				for _, g := range item.ModifierGroups {
					modifiers := make([]gin.H, 0, len(g.Modifiers))
					for _, m := range g.Modifiers {
						modifiers = append(modifiers, gin.H{"id": m.ID, "name": m.Name, "price_delta_minor": m.PriceDeltaMinor, "position": m.Position})
					}
					groups = append(groups, gin.H{"group": gin.H{"id": g.Group.ID, "name": g.Group.Name, "description": g.Group.Description, "min_select": g.Group.MinSelect, "max_select": g.Group.MaxSelect, "required": g.Group.Required}, "modifiers": modifiers})
				}
				view := gin.H{"ref": item.Ref, "name": item.Name, "description": item.Description, "media_refs": item.MediaRefs, "dietary_codes": item.DietaryCodes, "allergen_codes": item.AllergenCodes, "variants": variants, "modifier_groups": groups, "sellability": customerSellability(item.Sellability), "prep_min_minutes": item.Prep.PrepMinMinutes, "prep_max_minutes": item.Prep.PrepMaxMinutes}
				if item.Price != nil {
					view["price"] = gin.H{"currency": item.Price.Currency, "effective_unit_price_minor": item.Price.EffectiveUnitPriceMinor}
				}
				items = append(items, view)
			}
			sections = append(sections, gin.H{"ref": section.Ref, "name": section.Name, "position": section.Position, "items": items})
		}
		menus = append(menus, gin.H{"ref": menu.Ref, "name": menu.Name, "sections": sections})
	}
	return gin.H{"business_ref": snapshot.BusinessRef, "branch_ref": snapshot.BranchRef, "capability": snapshot.Capability, "catalogue_revision": snapshot.CatalogueRevision, "generated_at": snapshot.GeneratedAt, "menus": menus, "pagination": snapshot.Pagination}
}

func (h *InternalHandler) customerCatalogue(c *gin.Context) {
	in, err := parseDiscoveryQuery(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	if in.Limit <= 0 || in.Limit > 50 {
		in.Limit = 50
	}
	if in.Offset < 0 || in.Offset > 10000 {
		c.AbortWithStatus(400)
		return
	}
	snapshot, err := h.service.DiscoverCatalogue(c, in)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	facts, err := h.service.CustomerTrust(c, snapshot.BusinessRef, snapshot.BranchRef)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	view := customerCatalogueView(snapshot)
	view["trust"] = facts
	c.Header("Cache-Control", "no-store")
	internalOK(c, view)
}

func (s *Service) CustomerTrust(ctx context.Context, business, branch string) ([]publictrust.Fact, error) {
	return publictrust.Read(ctx, s.db, business, branch)
}

func (h *InternalHandler) customerSellability(c *gin.Context) {
	businessID, err := h.branchMatch(c, c.Param("itemId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	result, err := h.service.ResolveSellability(c, Actor{VendorID: businessID}, Scope{Global: true}, c.Param("itemId"), time.Now().UTC())
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	internalOK(c, customerSellability(result))
}

// Public customer Food discovery (Karryam V4 §14.1, B12).
//
// The Section 9C projection was internal-only. These routes expose the SAME
// read-only projection - catalogue, search within a branch and item
// sellability - to an authenticated customer. Nothing here mutates, prices a
// cart or reserves stock, and no vendor staff route is reused: the caller
// supplies the gates (customer bearer, the CUSTOMER_FOOD_ENABLED contract gate
// and a per-customer rate limit).

// publicBranch refuses any branch a customer may not see: the branch AND its
// business must both be ACTIVE. The internal projection only checks the
// branch, which is enough for trusted service callers but not for the public:
// a suspended vendor must disappear from discovery.
func (h *InternalHandler) publicBranch(c *gin.Context) {
	var branchStatus, businessStatus string
	err := h.service.db.QueryRow(c, `SELECT br.status,b.status FROM vendor_branches br
		JOIN vendor_businesses b ON b.id=br.business_id WHERE br.id=$1`, c.Param("branchId")).Scan(&branchStatus, &businessStatus)
	if err != nil || branchStatus != "ACTIVE" || businessStatus != "ACTIVE" {
		httpx.RespondError(c, errNotFound("Branch", CodeMenuNotFound))
		c.Abort()
		return
	}
	c.Next()
}

// RegisterCustomerDiscoveryRoutes mounts the public Food reads under the
// customer commerce group (normally /api/v1/commerce).
func RegisterCustomerDiscoveryRoutes(group *gin.RouterGroup, handler *InternalHandler, guards ...gin.HandlerFunc) {
	if group == nil || handler == nil {
		return
	}
	chain := append(append([]gin.HandlerFunc{}, guards...), handler.publicBranch)
	routes := group.Group("/food/branches/:branchId", chain...)
	routes.GET("/catalogue", handler.customerCatalogue)
	routes.GET("/items/:itemId/sellability", handler.customerSellability)
}
