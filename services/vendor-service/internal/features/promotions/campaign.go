// Package promotions owns commerce promotion definitions, eligibility and
// immutable calculation evidence. CheckoutQuote remains the price authority.
package promotions

import (
	"errors"
	"sort"
	"strings"
	"time"
)

type Type string
type Status string
type Sponsor string
type Combinability string

const (
	PercentageDiscount Type = "PERCENTAGE_DISCOUNT"
	FixedDiscount      Type = "FIXED_DISCOUNT"
	Coupon             Type = "COUPON"
	Bundle             Type = "BUNDLE"
	QuantityBreak      Type = "QUANTITY_BREAK"
	FreeDelivery       Type = "FREE_DELIVERY"
	DeliveryPercentage Type = "DELIVERY_PERCENTAGE_DISCOUNT"
	DeliveryFlat       Type = "DELIVERY_FLAT_DISCOUNT"

	Draft     Status = "DRAFT"
	Scheduled Status = "SCHEDULED"
	Active    Status = "ACTIVE"
	Paused    Status = "PAUSED"
	Ended     Status = "ENDED"
	Archived  Status = "ARCHIVED"

	Vendor   Sponsor = "VENDOR"
	Platform Sponsor = "PLATFORM"
	Combined Sponsor = "COMBINED"

	Exclusive Combinability = "EXCLUSIVE"
	Stackable Combinability = "STACKABLE"
)

var (
	ErrInvalidCampaign = errors.New("promotion campaign is invalid")
	ErrInvalidState    = errors.New("promotion campaign state transition is invalid")
	ErrCouponInvalid   = errors.New("promotion coupon is invalid")
	ErrUsageExhausted  = errors.New("promotion usage limit exhausted")
)

type Scope struct {
	Type string `json:"type"`
	Ref  string `json:"ref"`
}

type BundleRequirement struct {
	ItemRef         string `json:"item_ref"`
	VariantRef      string `json:"variant_ref,omitempty"`
	MinimumQuantity int64  `json:"minimum_quantity"`
}

type QuantityTier struct {
	MinimumQuantity int64  `json:"minimum_quantity"`
	MaximumQuantity *int64 `json:"maximum_quantity,omitempty"`
	PercentageBPS   int    `json:"percentage_bps"`
	FixedMinor      int64  `json:"fixed_amount_minor"`
}

type Campaign struct {
	ID                    string              `json:"id"`
	BusinessID            string              `json:"business_id"`
	Name                  string              `json:"name"`
	Type                  Type                `json:"promotion_type"`
	Status                Status              `json:"status"`
	Currency              string              `json:"currency"`
	PercentageBPS         int                 `json:"percentage_bps"`
	FixedMinor            int64               `json:"fixed_amount_minor"`
	MaxDiscountMinor      *int64              `json:"max_discount_minor,omitempty"`
	MinimumOrderMinor     int64               `json:"minimum_order_minor"`
	StartsAt              time.Time           `json:"starts_at"`
	EndsAt                time.Time           `json:"ends_at"`
	Sponsor               Sponsor             `json:"funding_sponsor"`
	VendorFundingBPS      int                 `json:"vendor_funding_bps"`
	PlatformApprovalRef   string              `json:"platform_approval_ref,omitempty"`
	CouponRequired        bool                `json:"coupon_required"`
	CouponDigest          string              `json:"-"`
	GlobalUsageLimit      *int64              `json:"global_usage_limit,omitempty"`
	PerCustomerUsageLimit *int64              `json:"per_customer_usage_limit,omitempty"`
	Combinability         Combinability       `json:"combinability"`
	Priority              int                 `json:"priority"`
	Version               int64               `json:"version"`
	Scopes                []Scope             `json:"scopes,omitempty"`
	BundleRequirements    []BundleRequirement `json:"bundle_requirements,omitempty"`
	QuantityTiers         []QuantityTier      `json:"quantity_tiers,omitempty"`
}

func (c Campaign) Validate() error {
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.BusinessID) == "" || strings.TrimSpace(c.Name) == "" ||
		len(strings.TrimSpace(c.Currency)) != 3 || !c.EndsAt.After(c.StartsAt) || c.Version < 1 ||
		c.PercentageBPS < 0 || c.PercentageBPS > 10000 || c.FixedMinor < 0 || c.MinimumOrderMinor < 0 ||
		c.VendorFundingBPS < 0 || c.VendorFundingBPS > 10000 {
		return ErrInvalidCampaign
	}
	switch c.Type {
	case PercentageDiscount, FixedDiscount, Coupon, Bundle, QuantityBreak, FreeDelivery, DeliveryPercentage, DeliveryFlat:
	default:
		return ErrInvalidCampaign
	}
	switch c.Status {
	case Draft, Scheduled, Active, Paused, Ended, Archived:
	default:
		return ErrInvalidCampaign
	}
	switch c.Sponsor {
	case Vendor:
		if c.VendorFundingBPS != 0 && c.VendorFundingBPS != 10000 {
			return ErrInvalidCampaign
		}
	case Platform:
		if strings.TrimSpace(c.PlatformApprovalRef) == "" || c.VendorFundingBPS != 0 {
			return ErrInvalidCampaign
		}
	case Combined:
		if strings.TrimSpace(c.PlatformApprovalRef) == "" || c.VendorFundingBPS <= 0 || c.VendorFundingBPS >= 10000 {
			return ErrInvalidCampaign
		}
	default:
		return ErrInvalidCampaign
	}
	if c.Combinability != Exclusive && c.Combinability != Stackable {
		return ErrInvalidCampaign
	}
	if (c.Type == PercentageDiscount || c.Type == DeliveryPercentage) && (c.PercentageBPS <= 0 || c.FixedMinor != 0) {
		return ErrInvalidCampaign
	}
	if (c.Type == FixedDiscount || c.Type == DeliveryFlat || c.Type == FreeDelivery) && c.Type != FreeDelivery && (c.FixedMinor <= 0 || c.PercentageBPS != 0) {
		return ErrInvalidCampaign
	}
	if c.Type == Bundle && len(c.BundleRequirements) == 0 {
		return ErrInvalidCampaign
	}
	if c.Type == QuantityBreak {
		if len(c.QuantityTiers) == 0 || !validTiers(c.QuantityTiers) {
			return ErrInvalidCampaign
		}
	}
	return nil
}

func validTiers(input []QuantityTier) bool {
	tiers := append([]QuantityTier(nil), input...)
	sort.Slice(tiers, func(i, j int) bool { return tiers[i].MinimumQuantity < tiers[j].MinimumQuantity })
	for i, tier := range tiers {
		if tier.MinimumQuantity <= 0 || (tier.PercentageBPS <= 0) == (tier.FixedMinor <= 0) || tier.PercentageBPS > 10000 {
			return false
		}
		if tier.MaximumQuantity != nil && *tier.MaximumQuantity < tier.MinimumQuantity {
			return false
		}
		if i > 0 {
			previous := tiers[i-1]
			if previous.MaximumQuantity == nil || *previous.MaximumQuantity+1 != tier.MinimumQuantity {
				return false
			}
		}
	}
	return true
}

func CanTransition(from, to Status) bool {
	if from == to {
		return true
	}
	switch from {
	case Draft:
		return to == Scheduled || to == Active || to == Archived
	case Scheduled:
		return to == Active || to == Paused || to == Ended || to == Archived
	case Active:
		return to == Paused || to == Ended || to == Archived
	case Paused:
		return to == Active || to == Ended || to == Archived
	case Ended:
		return to == Archived
	default:
		return false
	}
}
