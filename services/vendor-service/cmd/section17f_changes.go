package main

import (
	"context"

	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

// section17FChangeQuoteAuthority reuses the existing immutable Section 17D
// quote service. A requested address/service change therefore goes through the
// same checkout, inventory, logistics, pricing and promotion authorities as a
// first-time quote.
type section17FChangeQuoteAuthority struct{ quotes *checkoutquote.Service }

func (a section17FChangeQuoteAuthority) CreateReplacementQuote(ctx context.Context, customerID, sessionID, planID string) (orders.ReplacementQuote, error) {
	quote, err := a.quotes.Create(ctx, customerID, sessionID, planID)
	if err != nil {
		return orders.ReplacementQuote{}, err
	}
	return replacementQuote(quote), nil
}

func (a section17FChangeQuoteAuthority) CreateSubstitutionReplacementQuote(ctx context.Context, request orders.SubstitutionCommercialChangeRequest) (orders.ReplacementQuote, error) {
	quote, err := a.quotes.CreateSubstitutionReplacementQuote(ctx, request.CustomerID, request.CheckoutSessionID, request.FulfilmentPlanID, request.CartItemID, request.ItemRef, request.VariantRef, request.ListingRef, request.SellUnitRef, request.UnitPriceMinor)
	if err != nil {
		return orders.ReplacementQuote{}, err
	}
	return replacementQuote(quote), nil
}
func (a section17FChangeQuoteAuthority) GetReplacementQuote(ctx context.Context, customerID, quoteID string) (orders.ReplacementQuote, error) {
	quote, err := a.quotes.Get(ctx, customerID, quoteID)
	if err != nil {
		return orders.ReplacementQuote{}, err
	}
	return replacementQuote(quote), nil
}
func (a section17FChangeQuoteAuthority) ApproveReplacementQuote(ctx context.Context, customerID, quoteID, fingerprint string) (orders.ReplacementQuote, error) {
	quote, err := a.quotes.Approve(ctx, customerID, quoteID, fingerprint)
	if err != nil {
		return orders.ReplacementQuote{}, err
	}
	return replacementQuote(quote), nil
}

func replacementQuote(q checkoutquote.Quote) orders.ReplacementQuote {
	return orders.ReplacementQuote{
		CheckoutSessionID: q.CheckoutSessionID,
		ID:                q.ID, Version: q.QuoteVersion, Fingerprint: q.QuoteFingerprint,
		CustomerID: q.CustomerID,
		Address: orders.AddressSnapshot{
			SourceAddressRef: q.Address.SourceAddressRef, Title: q.Address.Title,
			Address: q.Address.Address, Latitude: q.Address.Latitude, Longitude: q.Address.Longitude,
			Description: q.Address.Description, SnapshottedAt: q.Address.SnapshottedAt,
		},
		FulfilmentPlanID: q.FulfilmentPlanID, SelectedMode: q.SelectedFulfilmentMode,
		TotalMinor: q.FinalPayableMinor, MaxAuthorizedMinor: q.MaxAuthorizedTotalMinor,
		Currency: q.Currency, ExpiresAt: q.ExpiresAt, Approved: q.State == checkoutquote.StateApproved,
	}
}

var _ orders.CommercialChangeQuoteAuthority = section17FChangeQuoteAuthority{}
