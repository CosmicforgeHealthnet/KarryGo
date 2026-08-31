package main

import (
	"context"
	"encoding/json"
	"fmt"

	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"cosmicforge/logistics/services/vendor-service/internal/features/promotions"
)

type checkoutPromotionAdapter struct {
	service *promotions.Service
	secret  []byte
}

func (a checkoutPromotionAdapter) Evaluate(ctx context.Context, input checkoutquote.PromotionContext) ([]checkoutquote.Adjustment, error) {
	if a.service == nil {
		return nil, nil
	}
	lines := make([]promotions.Line, 0, len(input.Lines))
	for _, line := range input.Lines {
		lines = append(lines, promotions.Line{BusinessID: line.BusinessID, BranchID: line.BranchID, ItemRef: line.ItemRef, VariantRef: line.VariantRef, Quantity: line.Quantity, AmountMinor: line.QuotedTotalMinor})
	}
	results, err := a.service.Evaluate(ctx, promotions.EvaluationInput{CustomerID: input.CustomerID, Currency: input.Currency,
		ProductSubtotalMinor: input.ProductSubtotalMinor, DeliveryCostMinor: input.DeliverySubtotalMinor,
		CouponDigest: promotions.DigestCoupon(a.secret, input.CouponCode), At: input.At, Lines: lines})
	if err != nil {
		return nil, err
	}
	var adjustments []checkoutquote.Adjustment
	for _, result := range results {
		evidence, _ := json.Marshal(result.EligibilityEvidence)
		appendFunding := func(suffix string, source checkoutquote.FundingSource, amount int64) {
			if amount <= 0 {
				return
			}
			at := result.AppliedAt
			adjustments = append(adjustments, checkoutquote.Adjustment{
				ID: fmt.Sprintf("promotion:%s:v%d:%s", result.CampaignID, result.CampaignVersion, suffix), Type: checkoutquote.AdjustmentDiscount,
				Scope: result.Scope, FundingSource: source, SourceRef: result.CampaignID, Description: string(result.Type), AmountMinor: amount,
				BusinessID: result.BusinessID, BranchID: result.BranchID, PromotionType: string(result.Type), CampaignVersion: result.CampaignVersion,
				EligibilityEvidence: evidence, CouponDigest: result.CouponDigest, CalculationBaseMinor: result.CalculationBaseMinor,
				PlatformApprovalRef:   result.PlatformApprovalRef,
				DeliveryDiscountMinor: minPromotionAmount(result.DeliveryDiscountMinor, amount), StackPriority: result.Priority, AppliedAt: &at})
		}
		appendFunding("vendor", checkoutquote.FundingVendor, result.VendorFundedMinor)
		appendFunding("platform", checkoutquote.FundingPlatform, result.PlatformFundedMinor)
	}
	return adjustments, nil
}

func minPromotionAmount(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

var _ checkoutquote.PromotionAuthority = checkoutPromotionAdapter{}
