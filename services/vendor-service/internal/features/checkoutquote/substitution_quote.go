package checkoutquote

import (
	"context"
	"math"
	"strings"
	"time"
)

// CreateSubstitutionReplacementQuote creates a fresh immutable quote from the
// existing checkout/fulfilment evidence while replacing exactly one cart line
// with the already-resolved Shop catalogue candidate. It reuses the normal
// promotion and quote persistence authority; it never edits the original quote.
func (s *Service) CreateSubstitutionReplacementQuote(ctx context.Context, customerID, sessionID, planID, cartItemID, itemRef, variantRef, listingRef, sellUnitRef string, unitPriceMinor int64) (Quote, error) {
	if s == nil || s.store == nil || unitPriceMinor < 0 || strings.TrimSpace(cartItemID) == "" {
		return Quote{}, ErrSourceNotQuotable
	}
	now := s.now().UTC()
	source, err := s.store.LoadSource(ctx, strings.TrimSpace(customerID), strings.TrimSpace(sessionID), strings.TrimSpace(planID), now)
	if err != nil {
		return Quote{}, err
	}
	found := false
	var delta int64
	for i := range source.Lines {
		line := &source.Lines[i]
		if line.CartItemID != cartItemID {
			continue
		}
		found = true
		oldTotal := line.QuotedTotalMinor
		newTotal, e := safeMul(unitPriceMinor, line.Quantity)
		if e != nil {
			return Quote{}, ErrInvalidAdjustment
		}
		delta = newTotal - oldTotal
		line.ItemRef, line.VariantRef, line.ListingRef, line.SellUnitRef = itemRef, variantRef, listingRef, sellUnitRef
		line.VendorOnlineUnitPriceMinor = unitPriceMinor
		line.QuotedTotalMinor = newTotal
		if line.MaxAuthorizedTotalMinor < newTotal {
			line.MaxAuthorizedTotalMinor = newTotal
		}
	}
	if !found {
		return Quote{}, ErrSourceNotQuotable
	}
	source.QuotedItemsTotalMinor, err = safeAdd(source.QuotedItemsTotalMinor, delta)
	if err != nil {
		return Quote{}, ErrInvalidAdjustment
	}
	if source.MaxAuthorizedItemsTotalMinor < source.QuotedItemsTotalMinor {
		source.MaxAuthorizedItemsTotalMinor = source.QuotedItemsTotalMinor
	}
	adjustments, err := s.promotions.Evaluate(ctx, PromotionContext{CustomerID: source.CustomerID, Currency: source.Currency, ProductSubtotalMinor: source.QuotedItemsTotalMinor, DeliverySubtotalMinor: source.DeliveryFunding.GrossMinor, Groups: append([]GroupSnapshot(nil), source.Groups...), Lines: append([]LineSnapshot(nil), source.Lines...)})
	if err != nil {
		return Quote{}, err
	}
	return s.saveReplacementSource(ctx, source, adjustments, now)
}

func safeMul(a, b int64) (int64, error) {
	if a < 0 || b < 0 || (a != 0 && b > math.MaxInt64/a) {
		return 0, ErrInvalidAdjustment
	}
	return a * b, nil
}

func (s *Service) saveReplacementSource(ctx context.Context, source SourceSnapshot, adjustments []Adjustment, now time.Time) (Quote, error) {
	quote, err := buildQuote(source, adjustments, now, s.ttl)
	if err != nil {
		return Quote{}, err
	}
	return s.store.Save(ctx, quote, now)
}
