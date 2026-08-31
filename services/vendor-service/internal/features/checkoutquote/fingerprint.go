package checkoutquote

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

type fingerprintMaterial struct {
	CheckoutSessionID          string                  `json:"checkout_session_id"`
	CheckoutValidationRevision int64                   `json:"checkout_validation_revision"`
	CartID                     string                  `json:"cart_id"`
	CartVersion                int64                   `json:"cart_version"`
	CustomerID                 string                  `json:"customer_id"`
	Currency                   string                  `json:"currency"`
	Address                    AddressSnapshot         `json:"address"`
	FulfilmentPlanID           string                  `json:"fulfilment_plan_id"`
	FulfilmentPlanRevision     int64                   `json:"fulfilment_plan_revision"`
	FulfilmentPlanVersion      int64                   `json:"fulfilment_plan_version"`
	SelectedFulfilmentMode     string                  `json:"selected_fulfilment_mode"`
	ProductSubtotalMinor       int64                   `json:"product_subtotal_minor"`
	MaxProductSubtotalMinor    int64                   `json:"max_product_subtotal_minor"`
	DeliverySubtotalMinor      int64                   `json:"delivery_subtotal_minor"`
	FeeSubtotalMinor           int64                   `json:"fee_subtotal_minor"`
	TaxSubtotalMinor           int64                   `json:"tax_subtotal_minor"`
	DiscountTotalMinor         int64                   `json:"discount_total_minor"`
	SubsidyTotalMinor          int64                   `json:"subsidy_total_minor"`
	FinalPayableMinor          int64                   `json:"final_payable_minor"`
	MaxAuthorizedTotalMinor    int64                   `json:"max_authorized_total_minor"`
	Funding                    FundingBreakdown        `json:"funding"`
	Disclosure                 PriceDisclosureSnapshot `json:"disclosure"`
	Groups                     []GroupSnapshot         `json:"groups"`
	Lines                      []LineSnapshot          `json:"lines"`
	Missions                   []MissionSnapshot       `json:"missions"`
	Adjustments                []Adjustment            `json:"adjustments"`
}

type fingerprintAgreement struct {
	Material     fingerprintMaterial `json:"material"`
	QuoteVersion int64               `json:"quote_version"`
	ExpiresAt    time.Time           `json:"expires_at"`
}

// sourceFingerprintMaterial is the authoritative Section 17B/17C evidence,
// separate from promotions and derived totals. It lets approval prove the
// old quote still describes the current cart allocation and fulfilment plan
// without recalculating or mutating the quote itself.
type sourceFingerprintMaterial struct {
	CheckoutSessionID          string            `json:"checkout_session_id"`
	CheckoutValidationRevision int64             `json:"checkout_validation_revision"`
	CartID                     string            `json:"cart_id"`
	CartVersion                int64             `json:"cart_version"`
	CustomerID                 string            `json:"customer_id"`
	Currency                   string            `json:"currency"`
	Address                    AddressSnapshot   `json:"address"`
	ProductSubtotalMinor       int64             `json:"product_subtotal_minor"`
	MaxProductSubtotalMinor    int64             `json:"max_product_subtotal_minor"`
	FulfilmentPlanID           string            `json:"fulfilment_plan_id"`
	FulfilmentPlanRevision     int64             `json:"fulfilment_plan_revision"`
	FulfilmentPlanVersion      int64             `json:"fulfilment_plan_version"`
	SelectedFulfilmentMode     string            `json:"selected_fulfilment_mode"`
	DeliveryFunding            DeliveryFunding   `json:"delivery_funding"`
	Groups                     []GroupSnapshot   `json:"groups"`
	Lines                      []LineSnapshot    `json:"lines"`
	Missions                   []MissionSnapshot `json:"missions"`
}

func canonicalSource(material sourceFingerprintMaterial) sourceFingerprintMaterial {
	material.Groups = append([]GroupSnapshot(nil), material.Groups...)
	material.Lines = append([]LineSnapshot(nil), material.Lines...)
	material.Missions = append([]MissionSnapshot(nil), material.Missions...)
	sort.Slice(material.Groups, func(i, j int) bool { return material.Groups[i].CheckoutGroupID < material.Groups[j].CheckoutGroupID })
	sort.Slice(material.Lines, func(i, j int) bool {
		if material.Lines[i].CartItemID == material.Lines[j].CartItemID {
			return material.Lines[i].CheckoutLineID < material.Lines[j].CheckoutLineID
		}
		return material.Lines[i].CartItemID < material.Lines[j].CartItemID
	})
	sort.Slice(material.Missions, func(i, j int) bool { return material.Missions[i].MissionID < material.Missions[j].MissionID })
	for i := range material.Missions {
		material.Missions[i].GroupIDs = append([]string(nil), material.Missions[i].GroupIDs...)
		sort.Strings(material.Missions[i].GroupIDs)
	}
	return material
}

func SourceFingerprint(source SourceSnapshot) (string, error) {
	return hashCanonical(canonicalSource(sourceFingerprintMaterial{
		CheckoutSessionID: source.CheckoutSessionID, CheckoutValidationRevision: source.CheckoutValidationRevision,
		CartID: source.CartID, CartVersion: source.CartVersion, CustomerID: source.CustomerID,
		Currency: source.Currency, Address: source.Address, ProductSubtotalMinor: source.QuotedItemsTotalMinor,
		MaxProductSubtotalMinor: source.MaxAuthorizedItemsTotalMinor, FulfilmentPlanID: source.FulfilmentPlanID,
		FulfilmentPlanRevision: source.FulfilmentPlanRevision, FulfilmentPlanVersion: source.FulfilmentPlanVersion,
		SelectedFulfilmentMode: source.SelectedFulfilmentMode, DeliveryFunding: source.DeliveryFunding,
		Groups: source.Groups, Lines: source.Lines, Missions: source.Missions,
	}))
}

func QuoteSourceFingerprint(quote Quote) (string, error) {
	var delivery DeliveryFunding
	for _, group := range quote.Groups {
		delivery.GrossMinor += group.DeliveryFunding.GrossMinor
		delivery.CustomerMinor += group.DeliveryFunding.CustomerMinor
		delivery.VendorFundedMinor += group.DeliveryFunding.VendorFundedMinor
		delivery.PlatformFundedMinor += group.DeliveryFunding.PlatformFundedMinor
	}
	return hashCanonical(canonicalSource(sourceFingerprintMaterial{
		CheckoutSessionID: quote.CheckoutSessionID, CheckoutValidationRevision: quote.CheckoutValidationRevision,
		CartID: quote.CartID, CartVersion: quote.CartVersion, CustomerID: quote.CustomerID,
		Currency: quote.Currency, Address: quote.Address, ProductSubtotalMinor: quote.ProductSubtotalMinor,
		MaxProductSubtotalMinor: quote.MaxProductSubtotalMinor, FulfilmentPlanID: quote.FulfilmentPlanID,
		FulfilmentPlanRevision: quote.FulfilmentPlanRevision, FulfilmentPlanVersion: quote.FulfilmentPlanVersion,
		SelectedFulfilmentMode: quote.SelectedFulfilmentMode, DeliveryFunding: delivery,
		Groups: quote.Groups, Lines: quote.Lines, Missions: quote.Missions,
	}))
}

func canonicalMaterial(quote Quote) fingerprintMaterial {
	groups := append([]GroupSnapshot(nil), quote.Groups...)
	lines := append([]LineSnapshot(nil), quote.Lines...)
	missions := append([]MissionSnapshot(nil), quote.Missions...)
	adjustments := append([]Adjustment(nil), quote.Adjustments...)

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].CheckoutGroupID < groups[j].CheckoutGroupID
	})
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].CartItemID == lines[j].CartItemID {
			return lines[i].CheckoutLineID < lines[j].CheckoutLineID
		}
		return lines[i].CartItemID < lines[j].CartItemID
	})
	sort.Slice(missions, func(i, j int) bool {
		return missions[i].MissionID < missions[j].MissionID
	})
	for i := range missions {
		missions[i].GroupIDs = append([]string(nil), missions[i].GroupIDs...)
		sort.Strings(missions[i].GroupIDs)
	}
	sort.Slice(adjustments, func(i, j int) bool {
		left, _ := json.Marshal(adjustments[i])
		right, _ := json.Marshal(adjustments[j])
		return string(left) < string(right)
	})

	return fingerprintMaterial{
		CheckoutSessionID:          quote.CheckoutSessionID,
		CheckoutValidationRevision: quote.CheckoutValidationRevision,
		CartID:                     quote.CartID,
		CartVersion:                quote.CartVersion,
		CustomerID:                 quote.CustomerID,
		Currency:                   quote.Currency,
		Address:                    quote.Address,
		FulfilmentPlanID:           quote.FulfilmentPlanID,
		FulfilmentPlanRevision:     quote.FulfilmentPlanRevision,
		FulfilmentPlanVersion:      quote.FulfilmentPlanVersion,
		SelectedFulfilmentMode:     quote.SelectedFulfilmentMode,
		ProductSubtotalMinor:       quote.ProductSubtotalMinor,
		MaxProductSubtotalMinor:    quote.MaxProductSubtotalMinor,
		DeliverySubtotalMinor:      quote.DeliverySubtotalMinor,
		FeeSubtotalMinor:           quote.FeeSubtotalMinor,
		TaxSubtotalMinor:           quote.TaxSubtotalMinor,
		DiscountTotalMinor:         quote.DiscountTotalMinor,
		SubsidyTotalMinor:          quote.SubsidyTotalMinor,
		FinalPayableMinor:          quote.FinalPayableMinor,
		MaxAuthorizedTotalMinor:    quote.MaxAuthorizedTotalMinor,
		Funding:                    quote.FundingBreakdown,
		Disclosure:                 quote.PriceDisclosure,
		Groups:                     groups,
		Lines:                      lines,
		Missions:                   missions,
		Adjustments:                adjustments,
	}
}

func hashCanonical(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func MaterialFingerprint(quote Quote) (string, error) {
	return hashCanonical(canonicalMaterial(quote))
}

func QuoteFingerprint(quote Quote) (string, error) {
	return hashCanonical(fingerprintAgreement{
		Material:     canonicalMaterial(quote),
		QuoteVersion: quote.QuoteVersion,
		ExpiresAt:    quote.ExpiresAt,
	})
}
