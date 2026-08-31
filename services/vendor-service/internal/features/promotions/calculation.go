package promotions

import (
	"math"
	"sort"
	"strings"
	"time"
)

type Line struct {
	BusinessID  string
	BranchID    string
	Capability  string
	CategoryRef string
	ItemRef     string
	VariantRef  string
	Quantity    int64
	AmountMinor int64
}

type EvaluationInput struct {
	CustomerID           string
	Currency             string
	ProductSubtotalMinor int64
	DeliveryCostMinor    int64
	CouponDigest         string
	CustomerSegments     map[string]bool
	Capabilities         map[string]bool
	CategoryRefs         map[string]bool
	At                   time.Time
	Lines                []Line
}

type Result struct {
	CampaignID            string         `json:"promotion_id"`
	CampaignVersion       int64          `json:"campaign_version"`
	BusinessID            string         `json:"business_id"`
	BranchID              string         `json:"branch_id,omitempty"`
	Type                  Type           `json:"promotion_type"`
	Scope                 string         `json:"scope"`
	CalculationBaseMinor  int64          `json:"calculation_base_minor"`
	DiscountMinor         int64          `json:"discount_minor"`
	DeliveryDiscountMinor int64          `json:"delivery_discount_minor"`
	VendorFundedMinor     int64          `json:"vendor_funded_minor"`
	PlatformFundedMinor   int64          `json:"platform_funded_minor"`
	CouponDigest          string         `json:"coupon_digest,omitempty"`
	PlatformApprovalRef   string         `json:"platform_approval_ref,omitempty"`
	Priority              int            `json:"stack_priority"`
	AppliedAt             time.Time      `json:"applied_at"`
	EligibilityEvidence   map[string]any `json:"eligibility_evidence"`
}

func Evaluate(campaigns []Campaign, input EvaluationInput) []Result {
	ordered := append([]Campaign(nil), campaigns...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Priority == ordered[j].Priority {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].Priority < ordered[j].Priority
	})
	var results []Result
	for _, campaign := range ordered {
		result, ok := EvaluateCampaign(campaign, input)
		if !ok {
			continue
		}
		if campaign.Combinability == Exclusive {
			// EXCLUSIVE means the selected campaign is the complete promotion
			// set, even if a lower-priority stackable candidate was evaluated
			// first. Priority and ID still deterministically select which
			// exclusive candidate wins.
			return []Result{result}
		}
		results = append(results, result)
	}
	return results
}

func EvaluateCampaign(c Campaign, input EvaluationInput) (Result, bool) {
	windowEligibleStatus := c.Status == Active || c.Status == Scheduled
	if c.Validate() != nil || !windowEligibleStatus || input.At.Before(c.StartsAt) || !input.At.Before(c.EndsAt) ||
		strings.ToUpper(input.Currency) != strings.ToUpper(c.Currency) || input.ProductSubtotalMinor < c.MinimumOrderMinor {
		return Result{}, false
	}
	if c.CouponRequired || c.Type == Coupon {
		if input.CouponDigest == "" || input.CouponDigest != c.CouponDigest {
			return Result{}, false
		}
	}
	eligible, business, branch, eligibleAmount, quantity, evidence := eligibility(c, input)
	if !eligible {
		return Result{}, false
	}
	base := eligibleAmount
	scope := "PRODUCT"
	if c.Type == FreeDelivery || c.Type == DeliveryPercentage || c.Type == DeliveryFlat {
		base = input.DeliveryCostMinor
		scope = "DELIVERY"
	}
	if base <= 0 {
		return Result{}, false
	}
	discount := int64(0)
	switch c.Type {
	case PercentageDiscount, DeliveryPercentage:
		discount = basisPoints(base, c.PercentageBPS)
	case FixedDiscount, DeliveryFlat:
		discount = min64(base, c.FixedMinor)
	case FreeDelivery:
		discount = base
	case Coupon, Bundle:
		if c.PercentageBPS > 0 {
			discount = basisPoints(base, c.PercentageBPS)
		} else {
			discount = min64(base, c.FixedMinor)
		}
	case QuantityBreak:
		tier, found := tierFor(c.QuantityTiers, quantity)
		if !found {
			return Result{}, false
		}
		if tier.PercentageBPS > 0 {
			discount = basisPoints(base, tier.PercentageBPS)
		} else {
			discount = min64(base, tier.FixedMinor)
		}
	}
	if c.MaxDiscountMinor != nil && discount > *c.MaxDiscountMinor {
		discount = *c.MaxDiscountMinor
	}
	if discount <= 0 || discount > base {
		return Result{}, false
	}
	vendor, platform := funding(c, discount)
	result := Result{CampaignID: c.ID, CampaignVersion: c.Version, BusinessID: business, BranchID: branch,
		Type: c.Type, Scope: scope, CalculationBaseMinor: base, DiscountMinor: discount,
		VendorFundedMinor: vendor, PlatformFundedMinor: platform, Priority: c.Priority,
		AppliedAt: input.At.UTC(), EligibilityEvidence: evidence}
	result.PlatformApprovalRef = c.PlatformApprovalRef
	if scope == "DELIVERY" {
		result.DeliveryDiscountMinor = discount
	}
	if input.CouponDigest != "" && (c.CouponRequired || c.Type == Coupon) {
		result.CouponDigest = input.CouponDigest
	}
	return result, true
}

func eligibility(c Campaign, input EvaluationInput) (bool, string, string, int64, int64, map[string]any) {
	lines := make([]Line, 0, len(input.Lines))
	for _, line := range input.Lines {
		if line.BusinessID == c.BusinessID {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return false, "", "", 0, 0, nil
	}
	scopeGroups := make(map[string][]string)
	for _, scope := range c.Scopes {
		scopeGroups[scope.Type] = append(scopeGroups[scope.Type], scope.Ref)
	}
	if refs := scopeGroups["CUSTOMER_SEGMENT"]; len(refs) > 0 {
		matched := false
		for _, ref := range refs {
			matched = matched || input.CustomerSegments[ref]
		}
		if !matched {
			return false, "", "", 0, 0, nil
		}
		delete(scopeGroups, "CUSTOMER_SEGMENT")
	}
	for scopeType, refs := range scopeGroups {
		if scopeType != "BRANCH" && scopeType != "ITEM" && scopeType != "VARIANT" && scopeType != "CATEGORY" && scopeType != "CAPABILITY" {
			return false, "", "", 0, 0, nil
		}
		filtered := make([]Line, 0, len(lines))
		for _, line := range lines {
			matched := false
			for _, ref := range refs {
				switch scopeType {
				case "BRANCH":
					matched = line.BranchID == ref
				case "ITEM":
					matched = line.ItemRef == ref
				case "VARIANT":
					matched = line.VariantRef == ref
				case "CATEGORY":
					matched = line.CategoryRef == ref || input.CategoryRefs[categoryEligibilityKey(c.BusinessID, line.ItemRef, ref)]
				case "CAPABILITY":
					matched = line.Capability == ref || input.Capabilities[capabilityEligibilityKey(c.BusinessID, line.BranchID, ref)]
				}
				if matched {
					break
				}
			}
			if matched {
				filtered = append(filtered, line)
			}
		}
		if len(filtered) == 0 {
			return false, "", "", 0, 0, nil
		}
		lines = filtered
	}
	if c.Type == Bundle {
		for _, required := range c.BundleRequirements {
			var total int64
			for _, line := range lines {
				if line.ItemRef == required.ItemRef && (required.VariantRef == "" || line.VariantRef == required.VariantRef) {
					total += line.Quantity
				}
			}
			if total < required.MinimumQuantity {
				return false, "", "", 0, 0, nil
			}
		}
	}
	var amount, quantity int64
	branch := ""
	for _, line := range lines {
		amount = safeSum(amount, line.AmountMinor)
		quantity = safeSum(quantity, line.Quantity)
		if branch == "" {
			branch = line.BranchID
		} else if branch != line.BranchID {
			branch = ""
		}
	}
	return amount >= 0 && quantity >= 0, c.BusinessID, branch, amount, quantity, map[string]any{
		"business_id": c.BusinessID, "branch_id": branch, "eligible_merchandise_minor": amount,
		"quantity": quantity, "order_threshold_base": "PRODUCT_SUBTOTAL_BEFORE_DISCOUNTS",
		"window_start_inclusive": true, "window_end_exclusive": true,
	}
}

func categoryEligibilityKey(businessID, itemRef, categoryRef string) string {
	return businessID + "\x00" + itemRef + "\x00" + categoryRef
}

func capabilityEligibilityKey(businessID, branchID, capability string) string {
	return businessID + "\x00" + branchID + "\x00" + capability
}

func tierFor(tiers []QuantityTier, quantity int64) (QuantityTier, bool) {
	for _, tier := range tiers {
		if quantity >= tier.MinimumQuantity && (tier.MaximumQuantity == nil || quantity <= *tier.MaximumQuantity) {
			return tier, true
		}
	}
	return QuantityTier{}, false
}

func basisPoints(base int64, bps int) int64 {
	if base <= 0 || bps <= 0 || bps > 10000 || base > math.MaxInt64/int64(bps) {
		return 0
	}
	return base * int64(bps) / 10000
}

func funding(c Campaign, amount int64) (int64, int64) {
	switch c.Sponsor {
	case Vendor:
		return amount, 0
	case Platform:
		return 0, amount
	case Combined:
		vendor := basisPoints(amount, c.VendorFundingBPS)
		return vendor, amount - vendor
	default:
		return 0, 0
	}
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
func safeSum(a, b int64) int64 {
	if a < 0 || b < 0 || a > math.MaxInt64-b {
		return -1
	}
	return a + b
}
