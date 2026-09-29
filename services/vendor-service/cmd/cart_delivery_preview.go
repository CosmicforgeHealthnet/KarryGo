package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type previewCartReader interface {
	GetCart(context.Context, string, string) (cart.Cart, error)
}
type previewQuoter interface {
	QuotePlan(context.Context, fulfilment.QuoteOnlyRequest) (fulfilment.QuoteOnlyResult, error)
}

type cartPreview struct {
	carts   previewCartReader
	food    checkout.FoodAuthority
	shop    checkout.ShopAuthority
	quotes  previewQuoter
	enabled func(cart.Domain) bool
}

// Preview re-reads and revalidates current evidence on EVERY request. It has
// no store, reservation, mission or payment port and never persists a quote.
// The second version read rejects a cart mutation concurrent with quoting.
func (p cartPreview) preview(ctx context.Context, customer, id string) (gin.H, error) {
	current, err := p.carts.GetCart(ctx, customer, id)
	if err != nil {
		return nil, err
	}
	if !p.enabled(current.Domain) {
		return nil, cart.ErrDomainDisabled
	}
	if current.Status != cart.StatusActive || !current.ExpiresAt.After(time.Now()) {
		return nil, cart.ErrCartNotMutable
	}
	if current.Address == nil || len(current.Items) == 0 {
		return nil, cart.ErrInvalidAddress
	}
	request := fulfilment.QuoteOnlyRequest{CustomerID: customer, Destination: fulfilment.Destination{Address: current.Address.Address, Latitude: current.Address.Latitude, Longitude: current.Address.Longitude}}
	groups := map[string]*fulfilment.QuoteOnlyGroup{}
	at := time.Now().UTC()
	for _, item := range current.Items {
		line := checkout.LineRequest{CartItemID: item.ID, Domain: item.Domain, ItemRef: item.ItemRef, VariantRef: item.VariantRef, MenuRef: item.MenuRef, ListingRef: item.ListingRef, SellUnitRef: item.SellUnitRef, SizeRef: item.SizeRef, Quantity: item.Quantity, PricingBasis: item.PricingBasis, RequestedMeasureRef: item.RequestedMeasureRef, ExpectedMeasureQuantity: item.ExpectedMeasureQuantity, At: at}
		for _, option := range item.Options {
			line.Options = append(line.Options, checkout.OptionRequest{GroupRef: option.GroupRef, OptionRef: option.OptionRef, Quantity: option.Quantity})
		}
		var decision checkout.LineDecision
		switch current.Domain {
		case cart.DomainFood:
			decision, err = p.food.RevalidateFoodLine(ctx, line)
		case cart.DomainShop:
			decision, err = p.shop.RevalidateShopLine(ctx, line)
		default:
			return nil, cart.ErrDomainConflict
		}
		if err != nil {
			return nil, err
		}
		if !decision.Sellable || item.Domain != current.Domain || decision.BusinessID != item.BusinessID || decision.BranchID != item.BranchID {
			return nil, cart.ErrInvalidCatalogueItem
		}
		if item.Quantity <= 0 || decision.UnitPriceMinor < 0 || decision.UnitPriceMinor > math.MaxInt64/item.Quantity {
			return nil, cart.ErrInvalidQuantity
		}
		if request.Currency == "" {
			request.Currency = decision.Currency
		}
		if request.Currency == "" || request.Currency != decision.Currency {
			return nil, cart.ErrInvalidCatalogueItem
		}
		key := decision.BusinessID + ":" + decision.BranchID
		group := groups[key]
		if group == nil {
			group = &fulfilment.QuoteOnlyGroup{Ref: key, BusinessID: decision.BusinessID, BranchID: decision.BranchID, Currency: decision.Currency}
			groups[key] = group
		}
		value, _, totalErr := checkout.LineTotals(decision.UnitPriceMinor, item.Quantity, item.PricingBasis, decision.MeasureOverfillBps)
		if totalErr != nil {
			return nil, totalErr
		}
		if group.MerchandiseMinor > math.MaxInt64-value {
			return nil, cart.ErrInvalidQuantity
		}
		group.MerchandiseMinor += value
		var measure int64
		if item.PricingBasis == cart.PricingBasisVariableMeasure {
			if item.ExpectedMeasureQuantity <= 0 || item.ExpectedMeasureQuantity > math.MaxInt64/item.Quantity {
				return nil, cart.ErrInvalidQuantity
			}
			measure = item.ExpectedMeasureQuantity * item.Quantity
		}
		group.Lines = append(group.Lines, fulfilment.LineLogistics{CheckoutLineID: item.ID, Domain: string(item.Domain), BusinessID: decision.BusinessID, BranchID: decision.BranchID, ItemRef: item.ItemRef, VariantRef: item.VariantRef, ListingRef: item.ListingRef, SellUnitRef: item.SellUnitRef, Quantity: item.Quantity, MeasureQuantity: measure})
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		request.Groups = append(request.Groups, *groups[key])
	}
	result, err := p.quotes.QuotePlan(ctx, request)
	if err != nil {
		return nil, err
	}
	latest, err := p.carts.GetCart(ctx, customer, id)
	if err != nil {
		return nil, err
	}
	// Hash includes receiver, address, lines, grouping and versions. The planner
	// resolves live branch/logistics policies each call; its evidence fingerprint
	// is included, so physical/handling changes also produce a fresh answer.
	before, _ := json.Marshal(current)
	after, _ := json.Marshal(latest)
	if string(before) != string(after) {
		return nil, cart.ErrVersionConflict
	}
	evidence, _ := json.Marshal(struct {
		Cart   cart.Cart
		Result fulfilment.QuoteOnlyResult
	}{current, result})
	digest := sha256.Sum256(evidence)
	return gin.H{"cart_id": current.ID, "cart_version": current.Version, "preview": true, "guaranteed": false, "fingerprint": hex.EncodeToString(digest[:]), "evaluated_at": at,
		"serviceable": result.Serviceable, "reason_code": result.ReasonCode, "currency": result.Currency, "delivery_customer_minor": result.DeliveryCustomerMinor, "completion_minutes": result.CompletionMinutes, "expires_at": result.EarliestQuoteExpiry, "groups": result.Groups}, nil
}

func registerCartDeliveryPreview(router *gin.Engine, cfg config.Config, carts previewCartReader, food checkout.FoodAuthority, shop checkout.ShopAuthority, quotes *fulfilment.PlanningService, limiter customerDiscoveryLimiter) {
	if carts == nil || quotes == nil || !cfg.Flags.CommerceFulfilmentPlanningEnabled {
		return
	}
	p := cartPreview{carts: carts, food: food, shop: shop, quotes: quotes, enabled: customerDomainGate(cfg)}
	cart.RegisterDeliveryPreviewRoute(router.Group("/api/v1/commerce"), sharedauth.NewTokenSigner(cfg.CustomerAccessTokenSecret), func(c *gin.Context) {
		claims, _ := sharedauth.ClaimsFromContext(c)
		if limiter == nil {
			httpx.RespondError(c, apperrors.Unavailable("Delivery preview is temporarily unavailable.", nil))
			return
		}
		if err := limiter.Allow(c, "customer_cart_delivery_preview", claims.Subject, 20, time.Minute); err != nil {
			httpx.RespondError(c, err)
			return
		}
		view, err := p.preview(c, claims.Subject, c.Param("cartId"))
		if err != nil {
			switch err {
			case cart.ErrCartNotFound, cart.ErrDomainDisabled:
				httpx.RespondError(c, apperrors.NotFound("Cart is unavailable.", nil))
			case cart.ErrVersionConflict:
				c.JSON(409, gin.H{"code": "CART_VERSION_CONFLICT"})
			case cart.ErrInvalidAddress, cart.ErrCartNotMutable, cart.ErrInvalidCatalogueItem, cart.ErrInvalidQuantity, cart.ErrDomainConflict:
				c.JSON(422, gin.H{"code": "DELIVERY_PREVIEW_NOT_READY"})
			default:
				httpx.RespondError(c, apperrors.Unavailable("Delivery preview is temporarily unavailable.", err))
			}
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(200, gin.H{"success": true, "data": view})
	})
}
