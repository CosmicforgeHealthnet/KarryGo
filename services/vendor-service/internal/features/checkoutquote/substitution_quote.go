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
	if s == nil || s.store == nil || s.replacementSource == nil || unitPriceMinor < 0 || strings.TrimSpace(cartItemID) == "" {
		return Quote{}, ErrSourceNotQuotable
	}
	// Validate the original authority evidence before preparing any successor.
	if _, err := s.store.LoadSource(ctx, customerID, sessionID, planID, s.now().UTC()); err != nil {
		return Quote{}, err
	}
	freshSession, freshPlan, err := s.replacementSource.PrepareReplacement(ctx, customerID, sessionID, planID, cartItemID, itemRef, variantRef, listingRef, sellUnitRef, unitPriceMinor)
	if err != nil {
		return Quote{}, err
	}
	if freshSession == "" || freshPlan == "" || freshSession == sessionID || freshPlan == planID {
		return Quote{}, ErrSourceNotQuotable
	}
	return s.Create(ctx, customerID, freshSession, freshPlan)
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
