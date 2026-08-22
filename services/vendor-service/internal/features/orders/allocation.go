package orders

// Multi-vendor financial isolation.
//
// One customer payment is never one undifferentiated vendor balance. This file
// turns an approved quote into per-vendor allocations that reconcile exactly to
// the customer-approved total, and refuses rather than guessing when a benefit
// cannot be attributed without one vendor paying for another's promotion.

import (
	"math"
	"sort"
)

// VendorAllocation is one vendor/branch's complete money position.
type VendorAllocation struct {
	QuoteGroupRef string
	BusinessID    string
	BranchID      string

	ProductSubtotalMinor    int64
	MaxProductSubtotalMinor int64

	ProductVendorFundedMinor   int64
	ProductPlatformFundedMinor int64
	ProductOtherFundedMinor    int64

	OrderVendorFundedMinor   int64
	OrderPlatformFundedMinor int64
	OrderOtherFundedMinor    int64

	DeliveryResponsibility DeliveryResponsibility
	DeliveryFeeBeneficiary FeeBeneficiary

	DeliveryGrossMinor          int64
	DeliveryCustomerMinor       int64
	DeliveryVendorFundedMinor   int64
	DeliveryPlatformFundedMinor int64

	// DeliveryOtherFundedMinor holds delivery benefit funded by an approved
	// source that is neither the vendor nor the platform. It is kept separate
	// so a delivery subsidy is never silently absorbed into a vendor payable.
	DeliveryOtherFundedMinor int64

	CustomerPayableMinor int64
	VendorPayableMinor   int64

	// LineDiscounts allocates product-scope benefit down to the individual
	// item snapshot, so a historical receipt can show what each line cost.
	LineDiscounts map[string]int64

	Allocations []AllocationDraft
}

// AllocationDraft is one allocation register row before it is persisted.
type AllocationDraft struct {
	QuoteGroupRef   string
	AllocationType  AllocationType
	BeneficiaryType BeneficiaryType
	BeneficiaryRef  string
	FundingSource   FundingSource
	SourceRef       string
	AmountMinor     int64
}

// AllocationPlan is the complete, reconciled allocation of one approved quote.
type AllocationPlan struct {
	Vendors []VendorAllocation

	// PlatformFeeTaxMinor is parent-scope customer money that belongs to no
	// single vendor. It is deliberately not spread across vendor orders.
	PlatformFeeTaxMinor int64

	// ParentAllocations are the fee and tax rows, which carry no vendor order.
	ParentAllocations []AllocationDraft
}

// ComputeAllocation turns an approved quote into per-vendor allocations.
//
// The invariant it guarantees, and asserts before returning: the sum of every
// vendor's customer payable plus the parent-scope fee and tax equals the
// customer-approved total exactly. Nothing is rounded away and nothing is left
// unattributed.
func ComputeAllocation(source QuoteSource) (AllocationPlan, error) {
	if len(source.Groups) == 0 || len(source.Lines) == 0 {
		return AllocationPlan{}, ErrQuoteNotOrderable
	}

	index := make(map[string]int, len(source.Groups))
	vendors := make([]VendorAllocation, 0, len(source.Groups))

	for _, group := range source.Groups {
		if group.QuoteGroupRef == "" || group.BusinessID == "" || group.BranchID == "" {
			return AllocationPlan{}, ErrQuoteNotOrderable
		}
		if _, duplicate := index[group.QuoteGroupRef]; duplicate {
			return AllocationPlan{}, ErrQuoteNotOrderable
		}
		if group.DeliveryCustomerMinor+group.DeliveryVendorFundedMinor+
			group.DeliveryPlatformFundedMinor != group.DeliveryGrossMinor {
			return AllocationPlan{}, ErrQuoteNotOrderable
		}
		// A delivery fee with no named executor would be money owed to
		// nobody, and a named executor with no fee is equally incoherent.
		switch group.DeliveryResponsibility {
		case DeliveryPlatform, DeliveryVendorOwn:
		case DeliveryNone:
			if group.DeliveryGrossMinor != 0 {
				return AllocationPlan{}, ErrQuoteNotOrderable
			}
		default:
			return AllocationPlan{}, ErrQuoteNotOrderable
		}

		index[group.QuoteGroupRef] = len(vendors)
		vendors = append(vendors, VendorAllocation{
			QuoteGroupRef:               group.QuoteGroupRef,
			BusinessID:                  group.BusinessID,
			BranchID:                    group.BranchID,
			ProductSubtotalMinor:        group.ProductSubtotalMinor,
			MaxProductSubtotalMinor:     group.MaxProductSubtotalMinor,
			DeliveryResponsibility:      group.DeliveryResponsibility,
			DeliveryFeeBeneficiary:      beneficiaryFor(group.DeliveryResponsibility),
			DeliveryGrossMinor:          group.DeliveryGrossMinor,
			DeliveryCustomerMinor:       group.DeliveryCustomerMinor,
			DeliveryVendorFundedMinor:   group.DeliveryVendorFundedMinor,
			DeliveryPlatformFundedMinor: group.DeliveryPlatformFundedMinor,
			LineDiscounts:               map[string]int64{},
		})
	}

	// Every line must belong to a known group, so no item snapshot can end up
	// on a vendor order that did not sell it.
	linesByGroup := make(map[string][]LineSource, len(vendors))
	for _, line := range source.Lines {
		if _, found := index[line.QuoteGroupRef]; !found ||
			line.CartItemID == "" || line.Quantity < 1 {
			return AllocationPlan{}, ErrQuoteNotOrderable
		}
		linesByGroup[line.QuoteGroupRef] = append(linesByGroup[line.QuoteGroupRef], line)
	}

	plan := AllocationPlan{Vendors: vendors}

	if err := applyAdjustments(&plan, source.Adjustments); err != nil {
		return AllocationPlan{}, err
	}
	if err := finaliseVendors(&plan, linesByGroup); err != nil {
		return AllocationPlan{}, err
	}
	if err := applyFeesAndTaxes(&plan, source); err != nil {
		return AllocationPlan{}, err
	}
	if err := reconcile(plan, source); err != nil {
		return AllocationPlan{}, err
	}
	return plan, nil
}

// applyAdjustments attributes each approved commercial adjustment.
//
// Attribution rule, applied to discounts:
//
//   - a single-vendor quote attributes everything to that one vendor;
//   - a vendor-funded benefit on a multi-vendor quote is REFUSED, because
//     Section 17D's adjustment carries no vendor identity and splitting it
//     would make vendor B pay for vendor A's promotion;
//   - a platform, delivery-subsidy or other-approved benefit costs no vendor
//     anything, so it is spread deterministically by largest remainder.
//
// Fees and taxes are always customer-funded order-scope amounts and are handled
// as parent-scope money, never charged against a vendor.
func applyAdjustments(plan *AllocationPlan, adjustments []AdjustmentSource) error {
	for _, adjustment := range adjustments {
		if adjustment.AmountMinor <= 0 || adjustment.AdjustmentIDEmpty() {
			return ErrQuoteNotOrderable
		}
		if adjustment.Type != AdjustmentDiscount {
			// Fee and tax are recorded from the quote's own subtotals in
			// applyFeesAndTaxes. A non-customer-funded fee or tax never
			// reached an approved quote, and is refused here too.
			if adjustment.FundingSource != FundingCustomer || adjustment.Scope != ScopeOrder {
				return ErrQuoteNotOrderable
			}
			continue
		}

		weights, err := discountWeights(plan.Vendors, adjustment.Scope)
		if err != nil {
			return err
		}
		shares, err := attribute(plan.Vendors, adjustment, weights)
		if err != nil {
			return err
		}

		for position, amount := range shares {
			if amount == 0 {
				continue
			}
			vendor := &plan.Vendors[position]
			if err = creditDiscount(vendor, adjustment, amount); err != nil {
				return err
			}
			vendor.Allocations = append(vendor.Allocations, AllocationDraft{
				QuoteGroupRef:   vendor.QuoteGroupRef,
				AllocationType:  AllocationDiscount,
				BeneficiaryType: BeneficiaryTypeCustomer,
				FundingSource:   adjustment.FundingSource,
				SourceRef:       adjustment.SourceRef,
				AmountMinor:     amount,
			})
		}
	}
	return nil
}

// AdjustmentIDEmpty reports a missing adjustment identity. Approved evidence
// always names its own source.
func (a AdjustmentSource) AdjustmentIDEmpty() bool {
	return a.AdjustmentID == "" || a.SourceRef == ""
}

func discountWeights(vendors []VendorAllocation, scope string) ([]int64, error) {
	weights := make([]int64, len(vendors))
	for i, vendor := range vendors {
		switch scope {
		case ScopeDelivery:
			weights[i] = vendor.DeliveryCustomerMinor
		case ScopeProduct, ScopeOrder:
			weights[i] = vendor.ProductSubtotalMinor
		default:
			return nil, ErrQuoteNotOrderable
		}
	}
	return weights, nil
}

// attribute returns each vendor's share of one adjustment.
func attribute(vendors []VendorAllocation, adjustment AdjustmentSource, weights []int64) ([]int64, error) {
	if len(vendors) == 1 {
		return []int64{adjustment.AmountMinor}, nil
	}
	if adjustment.FundingSource == FundingVendor {
		return nil, ErrNotAttributable
	}
	return distribute(adjustment.AmountMinor, weights)
}

// distribute splits an amount by weight using largest remainder, so the parts
// always sum back to the whole and the result is deterministic.
func distribute(amount int64, weights []int64) ([]int64, error) {
	if amount < 0 {
		return nil, ErrQuoteNotOrderable
	}
	shares := make([]int64, len(weights))
	if len(weights) == 0 {
		return nil, ErrQuoteNotOrderable
	}

	var total int64
	for _, weight := range weights {
		if weight < 0 || total > math.MaxInt64-weight {
			return nil, ErrQuoteNotOrderable
		}
		total += weight
	}

	// With no weight to go by, an equal split is the only neutral answer.
	if total == 0 {
		base := amount / int64(len(weights))
		remainder := amount - base*int64(len(weights))
		for i := range shares {
			shares[i] = base
			if int64(i) < remainder {
				shares[i]++
			}
		}
		return shares, nil
	}

	type remainderEntry struct {
		position  int
		remainder int64
	}

	assigned := int64(0)
	entries := make([]remainderEntry, 0, len(weights))

	for i, weight := range weights {
		if weight > 0 && amount > math.MaxInt64/weight {
			return nil, ErrQuoteNotOrderable
		}
		product := amount * weight
		shares[i] = product / total
		assigned += shares[i]
		entries = append(entries, remainderEntry{position: i, remainder: product % total})
	}

	// Ties break on position, so the same inputs always produce the same
	// allocation regardless of map or query ordering.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].remainder != entries[j].remainder {
			return entries[i].remainder > entries[j].remainder
		}
		return entries[i].position < entries[j].position
	})

	for i := int64(0); i < amount-assigned; i++ {
		shares[entries[i%int64(len(entries))].position]++
	}
	return shares, nil
}

// creditDiscount records who funded a benefit on one vendor allocation.
func creditDiscount(vendor *VendorAllocation, adjustment AdjustmentSource, amount int64) error {
	switch adjustment.Scope {
	case ScopeProduct:
		switch adjustment.FundingSource {
		case FundingVendor:
			vendor.ProductVendorFundedMinor += amount
		case FundingPlatform:
			vendor.ProductPlatformFundedMinor += amount
		case FundingOtherApproved, FundingDeliverySubsidy:
			vendor.ProductOtherFundedMinor += amount
		default:
			return ErrQuoteNotOrderable
		}
	case ScopeOrder:
		switch adjustment.FundingSource {
		case FundingVendor:
			vendor.OrderVendorFundedMinor += amount
		case FundingPlatform:
			vendor.OrderPlatformFundedMinor += amount
		case FundingOtherApproved, FundingDeliverySubsidy:
			vendor.OrderOtherFundedMinor += amount
		default:
			return ErrQuoteNotOrderable
		}
	case ScopeDelivery:
		// A delivery benefit reduces what the customer pays for delivery. The
		// executing party is still owed the full gross fee, so the funder's
		// contribution moves out of the customer column and into theirs.
		if amount > vendor.DeliveryCustomerMinor {
			return ErrAllocationImbalance
		}
		vendor.DeliveryCustomerMinor -= amount
		switch adjustment.FundingSource {
		case FundingVendor:
			vendor.DeliveryVendorFundedMinor += amount
		case FundingPlatform:
			vendor.DeliveryPlatformFundedMinor += amount
		case FundingDeliverySubsidy, FundingOtherApproved:
			vendor.DeliveryOtherFundedMinor += amount
		default:
			return ErrQuoteNotOrderable
		}
	default:
		return ErrQuoteNotOrderable
	}
	return nil
}

// finaliseVendors computes each vendor's payables and writes the product and
// delivery allocation rows.
func finaliseVendors(plan *AllocationPlan, linesByGroup map[string][]LineSource) error {
	for i := range plan.Vendors {
		vendor := &plan.Vendors[i]

		productDiscount := vendor.ProductVendorFundedMinor +
			vendor.ProductPlatformFundedMinor + vendor.ProductOtherFundedMinor
		orderDiscount := vendor.OrderVendorFundedMinor +
			vendor.OrderPlatformFundedMinor + vendor.OrderOtherFundedMinor

		productCustomer := vendor.ProductSubtotalMinor - productDiscount
		if productCustomer < 0 {
			return ErrAllocationImbalance
		}

		customerPayable := productCustomer + vendor.DeliveryCustomerMinor - orderDiscount
		if customerPayable < 0 {
			return ErrAllocationImbalance
		}
		vendor.CustomerPayableMinor = customerPayable

		// What the vendor is owed. A vendor-funded benefit is the vendor's own
		// contribution, so it reduces the vendor's payable and nobody else's —
		// including a delivery subsidy the vendor offered on a delivery the
		// platform executed, where the provider is still owed the gross fee
		// and the vendor is the one who pays for the customer's discount.
		vendorPayable := vendor.ProductSubtotalMinor -
			vendor.ProductVendorFundedMinor -
			vendor.OrderVendorFundedMinor -
			vendor.DeliveryVendorFundedMinor
		if vendor.DeliveryResponsibility == DeliveryVendorOwn {
			vendorPayable += vendor.DeliveryGrossMinor
		}
		if vendorPayable < 0 {
			return ErrAllocationImbalance
		}
		vendor.VendorPayableMinor = vendorPayable

		if err := allocateLineDiscounts(vendor, linesByGroup[vendor.QuoteGroupRef], productDiscount); err != nil {
			return err
		}
		appendProductAllocations(vendor, productCustomer)
		appendDeliveryAllocations(vendor)
	}
	return nil
}

// allocateLineDiscounts pushes the group's product benefit down to its lines.
func allocateLineDiscounts(vendor *VendorAllocation, lines []LineSource, productDiscount int64) error {
	if len(lines) == 0 {
		return ErrQuoteNotOrderable
	}
	// A stable order keeps the split reproducible across reads.
	ordered := append([]LineSource(nil), lines...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].CartItemID < ordered[j].CartItemID })

	weights := make([]int64, len(ordered))
	for i, line := range ordered {
		weights[i] = line.QuotedTotalMinor
	}
	shares, err := distribute(productDiscount, weights)
	if err != nil {
		return err
	}
	for i, line := range ordered {
		if shares[i] > line.QuotedTotalMinor {
			return ErrAllocationImbalance
		}
		vendor.LineDiscounts[line.CartItemID] = shares[i]
	}
	return nil
}

func appendProductAllocations(vendor *VendorAllocation, productCustomer int64) {
	add := func(funding FundingSource, amount int64) {
		if amount <= 0 {
			return
		}
		vendor.Allocations = append(vendor.Allocations, AllocationDraft{
			QuoteGroupRef:   vendor.QuoteGroupRef,
			AllocationType:  AllocationProduct,
			BeneficiaryType: BeneficiaryTypeVendor,
			BeneficiaryRef:  vendor.BusinessID,
			FundingSource:   funding,
			AmountMinor:     amount,
		})
	}
	// The four parts always reconstruct this vendor's own product subtotal.
	// They can never reconstruct another vendor's.
	add(FundingCustomer, productCustomer)
	add(FundingVendor, vendor.ProductVendorFundedMinor)
	add(FundingPlatform, vendor.ProductPlatformFundedMinor)
	add(FundingOtherApproved, vendor.ProductOtherFundedMinor)
}

func appendDeliveryAllocations(vendor *VendorAllocation) {
	if vendor.DeliveryGrossMinor == 0 {
		return
	}
	beneficiary := BeneficiaryTypePlatformProvide
	beneficiaryRef := ""
	if vendor.DeliveryFeeBeneficiary == BeneficiaryVendor {
		beneficiary = BeneficiaryTypeVendor
		beneficiaryRef = vendor.BusinessID
	}
	add := func(funding FundingSource, amount int64) {
		if amount <= 0 {
			return
		}
		vendor.Allocations = append(vendor.Allocations, AllocationDraft{
			QuoteGroupRef:   vendor.QuoteGroupRef,
			AllocationType:  AllocationDelivery,
			BeneficiaryType: beneficiary,
			BeneficiaryRef:  beneficiaryRef,
			FundingSource:   funding,
			AmountMinor:     amount,
		})
	}
	add(FundingCustomer, vendor.DeliveryCustomerMinor)
	add(FundingVendor, vendor.DeliveryVendorFundedMinor)
	add(FundingPlatform, vendor.DeliveryPlatformFundedMinor)
	add(FundingDeliverySubsidy, vendor.DeliveryOtherFundedMinor)
}

func applyFeesAndTaxes(plan *AllocationPlan, source QuoteSource) error {
	if source.FeeSubtotalMinor < 0 || source.TaxSubtotalMinor < 0 {
		return ErrQuoteNotOrderable
	}
	plan.PlatformFeeTaxMinor = source.FeeSubtotalMinor + source.TaxSubtotalMinor

	if source.FeeSubtotalMinor > 0 {
		plan.ParentAllocations = append(plan.ParentAllocations, AllocationDraft{
			AllocationType:  AllocationFee,
			BeneficiaryType: BeneficiaryTypePlatform,
			FundingSource:   FundingCustomer,
			AmountMinor:     source.FeeSubtotalMinor,
		})
	}
	if source.TaxSubtotalMinor > 0 {
		plan.ParentAllocations = append(plan.ParentAllocations, AllocationDraft{
			AllocationType:  AllocationTax,
			BeneficiaryType: BeneficiaryTypePlatform,
			FundingSource:   FundingCustomer,
			AmountMinor:     source.TaxSubtotalMinor,
		})
	}
	return nil
}

// reconcile is the structural guard. If the allocation does not add back up to
// the exact figure the customer approved, no order is created at all.
func reconcile(plan AllocationPlan, source QuoteSource) error {
	var customer, product, delivery int64
	for _, vendor := range plan.Vendors {
		customer += vendor.CustomerPayableMinor
		product += vendor.ProductSubtotalMinor
		delivery += vendor.DeliveryGrossMinor
	}
	if product != source.ProductSubtotalMinor || delivery != source.DeliverySubtotalMinor {
		return ErrAllocationImbalance
	}
	if customer+plan.PlatformFeeTaxMinor != source.FinalPayableMinor {
		return ErrAllocationImbalance
	}
	return nil
}
