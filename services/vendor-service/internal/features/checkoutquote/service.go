package checkoutquote

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"
)

type StoreAuthority interface {
	LoadSource(ctx context.Context, customerID, sessionID, planID string, now time.Time) (SourceSnapshot, error)
	Save(ctx context.Context, quote Quote, now time.Time) (Quote, error)
	Get(ctx context.Context, customerID, quoteID string, now time.Time) (Quote, error)
	Approve(ctx context.Context, customerID, quoteID, fingerprint string, now time.Time) (Quote, error)
}

type NoPromotions struct{}

func (NoPromotions) Evaluate(context.Context, PromotionContext) ([]Adjustment, error) {
	return nil, nil
}

type Options struct {
	Store      StoreAuthority
	Promotions PromotionAuthority
	QuoteTTL   time.Duration
	Now        func() time.Time
}

type Service struct {
	store      StoreAuthority
	promotions PromotionAuthority
	ttl        time.Duration
	now        func() time.Time
}

func NewService(options Options) (*Service, error) {
	if options.Store == nil {
		return nil, ErrDependencyMissing
	}
	if options.QuoteTTL < MinQuoteTTL || options.QuoteTTL > MaxQuoteTTL {
		return nil, ErrInvalidTTL
	}
	promotions := options.Promotions
	if promotions == nil {
		promotions = NoPromotions{}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Service{store: options.Store, promotions: promotions, ttl: options.QuoteTTL, now: now}, nil
}

func (s *Service) Create(ctx context.Context, customerID, sessionID, planID string) (Quote, error) {
	customerID = strings.TrimSpace(customerID)
	sessionID = strings.TrimSpace(sessionID)
	planID = strings.TrimSpace(planID)
	if customerID == "" || sessionID == "" || planID == "" {
		return Quote{}, ErrSourceNotQuotable
	}
	now := s.now().UTC()
	source, err := s.store.LoadSource(ctx, customerID, sessionID, planID, now)
	if err != nil {
		return Quote{}, err
	}
	adjustments, err := s.promotions.Evaluate(ctx, PromotionContext{
		CustomerID:            customerID,
		Currency:              source.Currency,
		ProductSubtotalMinor:  source.QuotedItemsTotalMinor,
		DeliverySubtotalMinor: source.DeliveryFunding.GrossMinor,
		Groups:                append([]GroupSnapshot(nil), source.Groups...),
		Lines:                 append([]LineSnapshot(nil), source.Lines...),
	})
	if err != nil {
		return Quote{}, err
	}
	quote, err := buildQuote(source, adjustments, now, s.ttl)
	if err != nil {
		return Quote{}, err
	}
	return s.store.Save(ctx, quote, now)
}

func (s *Service) Get(ctx context.Context, customerID, quoteID string) (Quote, error) {
	return s.store.Get(ctx, strings.TrimSpace(customerID), strings.TrimSpace(quoteID), s.now().UTC())
}

func (s *Service) Approve(ctx context.Context, customerID, quoteID, fingerprint string) (Quote, error) {
	return s.store.Approve(ctx, strings.TrimSpace(customerID), strings.TrimSpace(quoteID), strings.TrimSpace(fingerprint), s.now().UTC())
}

func buildQuote(source SourceSnapshot, adjustments []Adjustment, now time.Time, ttl time.Duration) (Quote, error) {
	if err := validateSourceSnapshot(source); err != nil {
		return Quote{}, ErrSourceNotQuotable
	}
	expiresAt := now.Add(ttl)
	if source.CheckoutExpiresAt.Before(expiresAt) {
		expiresAt = source.CheckoutExpiresAt
	}
	if source.FulfilmentPlanExpiresAt.Before(expiresAt) {
		expiresAt = source.FulfilmentPlanExpiresAt
	}
	if !expiresAt.After(now) {
		return Quote{}, ErrQuoteExpired
	}

	quote := Quote{
		CustomerID:                 source.CustomerID,
		CheckoutSessionID:          source.CheckoutSessionID,
		CheckoutValidationRevision: source.CheckoutValidationRevision,
		CartID:                     source.CartID,
		CartVersion:                source.CartVersion,
		FulfilmentPlanID:           source.FulfilmentPlanID,
		FulfilmentPlanRevision:     source.FulfilmentPlanRevision,
		FulfilmentPlanVersion:      source.FulfilmentPlanVersion,
		Currency:                   source.Currency,
		SelectedFulfilmentMode:     source.SelectedFulfilmentMode,
		Address:                    source.Address,
		ExpiresAt:                  expiresAt,
		ProductSubtotalMinor:       source.QuotedItemsTotalMinor,
		MaxProductSubtotalMinor:    source.MaxAuthorizedItemsTotalMinor,
		DeliverySubtotalMinor:      source.DeliveryFunding.GrossMinor,
		Groups:                     append([]GroupSnapshot(nil), source.Groups...),
		Lines:                      append([]LineSnapshot(nil), source.Lines...),
		Missions:                   append([]MissionSnapshot(nil), source.Missions...),
		State:                      StateQuoted,
		CreatedAt:                  now,
	}

	quote.FundingBreakdown = FundingBreakdown{
		ProductCustomerMinor:        source.QuotedItemsTotalMinor,
		DeliveryGrossMinor:          source.DeliveryFunding.GrossMinor,
		DeliveryCustomerMinor:       source.DeliveryFunding.CustomerMinor,
		DeliveryVendorFundedMinor:   source.DeliveryFunding.VendorFundedMinor,
		DeliveryPlatformFundedMinor: source.DeliveryFunding.PlatformFundedMinor,
	}

	var err error
	quote.SubsidyTotalMinor, err = safeAdd(source.DeliveryFunding.VendorFundedMinor, source.DeliveryFunding.PlatformFundedMinor)
	if err != nil {
		return Quote{}, err
	}
	quote.DiscountTotalMinor = quote.SubsidyTotalMinor

	adjustments = append([]Adjustment(nil), adjustments...)
	for i := range adjustments {
		if err = validateAdjustment(adjustments[i]); err != nil {
			return Quote{}, err
		}
		adjustment := adjustments[i]
		switch adjustment.Type {
		case AdjustmentDiscount:
			quote.DiscountTotalMinor, err = safeAdd(quote.DiscountTotalMinor, adjustment.AmountMinor)
			if err == nil {
				err = attributeDiscountFunding(&quote.FundingBreakdown, adjustment)
			}
		case AdjustmentFee:
			if adjustment.FundingSource != FundingCustomer {
				return Quote{}, ErrInvalidAdjustment
			}
			quote.FeeSubtotalMinor, err = safeAdd(quote.FeeSubtotalMinor, adjustment.AmountMinor)
			quote.FundingBreakdown.FeeCustomerMinor = quote.FeeSubtotalMinor
		case AdjustmentTax:
			if adjustment.FundingSource != FundingCustomer {
				return Quote{}, ErrInvalidAdjustment
			}
			quote.TaxSubtotalMinor, err = safeAdd(quote.TaxSubtotalMinor, adjustment.AmountMinor)
			quote.FundingBreakdown.TaxCustomerMinor = quote.TaxSubtotalMinor
		}
		if err != nil {
			return Quote{}, err
		}
	}
	quote.Adjustments = adjustments

	gross, err := safeAdd(quote.ProductSubtotalMinor, quote.DeliverySubtotalMinor, quote.FeeSubtotalMinor, quote.TaxSubtotalMinor)
	if err != nil || quote.DiscountTotalMinor > gross {
		return Quote{}, ErrInvalidAdjustment
	}
	quote.FinalPayableMinor = gross - quote.DiscountTotalMinor
	quote.QuotedTotalMinor = quote.FinalPayableMinor

	maxGross, err := safeAdd(quote.MaxProductSubtotalMinor, quote.DeliverySubtotalMinor, quote.FeeSubtotalMinor, quote.TaxSubtotalMinor)
	if err != nil || quote.DiscountTotalMinor > maxGross {
		return Quote{}, ErrInvalidAdjustment
	}
	quote.MaxAuthorizedTotalMinor = maxGross - quote.DiscountTotalMinor
	if quote.MaxAuthorizedTotalMinor < quote.QuotedTotalMinor {
		return Quote{}, ErrInvalidAdjustment
	}
	productDiscount := quote.FundingBreakdown.ProductVendorFundedMinor +
		quote.FundingBreakdown.ProductPlatformFundedMinor +
		quote.FundingBreakdown.ProductOtherFundedMinor
	quote.FundingBreakdown.ProductCustomerMinor = quote.ProductSubtotalMinor - productDiscount
	if quote.FundingBreakdown.ProductCustomerMinor < 0 {
		return Quote{}, ErrInvalidAdjustment
	}
	quote.PriceDisclosure = PriceDisclosureSnapshot{
		ProductSubtotalMinor:    quote.ProductSubtotalMinor,
		DeliverySubtotalMinor:   quote.DeliverySubtotalMinor,
		FeeSubtotalMinor:        quote.FeeSubtotalMinor,
		TaxSubtotalMinor:        quote.TaxSubtotalMinor,
		DiscountTotalMinor:      quote.DiscountTotalMinor,
		SubsidyTotalMinor:       quote.SubsidyTotalMinor,
		FinalPayableMinor:       quote.FinalPayableMinor,
		PriceContext:            "VENDOR_ONLINE_PRICE",
		StorePriceParityClaimed: false,
	}

	for i := range quote.Lines {
		quote.Lines[i].OnlinePriceDisclosure = "VENDOR_ONLINE_PRICE"
		quote.Lines[i].StorePriceParityClaimed = false
	}
	sort.Slice(quote.Adjustments, func(i, j int) bool { return quote.Adjustments[i].ID < quote.Adjustments[j].ID })
	quote.MaterialFingerprint, err = MaterialFingerprint(quote)
	if err != nil {
		return Quote{}, err
	}
	return quote, nil
}

func validateSourceSnapshot(source SourceSnapshot) error {
	if source.CustomerID == "" || source.CheckoutSessionID == "" || source.CartID == "" ||
		source.CartVersion < 1 || source.CheckoutValidationRevision < 1 || source.Currency == "" ||
		source.FulfilmentPlanID == "" || source.FulfilmentPlanRevision < 1 ||
		source.FulfilmentPlanVersion < 1 || source.SelectedFulfilmentMode == "" ||
		source.Address.SourceAddressRef == "" || source.Address.Address == "" ||
		source.Address.SnapshottedAt.IsZero() || len(source.Lines) == 0 || len(source.Groups) == 0 {
		return ErrSourceNotQuotable
	}
	if source.DeliveryFunding.CustomerMinor+source.DeliveryFunding.VendorFundedMinor+
		source.DeliveryFunding.PlatformFundedMinor != source.DeliveryFunding.GrossMinor {
		return ErrSourceNotQuotable
	}
	groups := make(map[string]GroupSnapshot, len(source.Groups))
	var product, maxProduct, delivery, customerDelivery, vendorDelivery, platformDelivery int64
	for _, group := range source.Groups {
		if group.CheckoutGroupID == "" || group.BusinessID == "" || group.BranchID == "" ||
			!group.DeliveryFundingBalanced() {
			return ErrSourceNotQuotable
		}
		if _, duplicate := groups[group.CheckoutGroupID]; duplicate {
			return ErrSourceNotQuotable
		}
		groups[group.CheckoutGroupID] = group
		var err error
		if product, err = safeAdd(product, group.ProductSubtotalMinor); err != nil {
			return err
		}
		if maxProduct, err = safeAdd(maxProduct, group.MaxProductSubtotalMinor); err != nil {
			return err
		}
		if delivery, err = safeAdd(delivery, group.DeliveryFunding.GrossMinor); err != nil {
			return err
		}
		customerDelivery, err = safeAdd(customerDelivery, group.DeliveryFunding.CustomerMinor)
		if err != nil {
			return err
		}
		vendorDelivery, err = safeAdd(vendorDelivery, group.DeliveryFunding.VendorFundedMinor)
		if err != nil {
			return err
		}
		platformDelivery, err = safeAdd(platformDelivery, group.DeliveryFunding.PlatformFundedMinor)
		if err != nil {
			return err
		}
	}
	if product != source.QuotedItemsTotalMinor || maxProduct != source.MaxAuthorizedItemsTotalMinor ||
		delivery != source.DeliveryFunding.GrossMinor || customerDelivery != source.DeliveryFunding.CustomerMinor ||
		vendorDelivery != source.DeliveryFunding.VendorFundedMinor || platformDelivery != source.DeliveryFunding.PlatformFundedMinor {
		return ErrSourceNotQuotable
	}
	var lineTotal, lineMax int64
	lineIDs := make(map[string]struct{}, len(source.Lines))
	for _, line := range source.Lines {
		group, found := groups[line.CheckoutGroupID]
		if !found || line.CheckoutLineID == "" || line.CartItemID == "" || line.Quantity < 1 ||
			line.BusinessID != group.BusinessID || line.BranchID != group.BranchID ||
			line.MaxAuthorizedTotalMinor < line.QuotedTotalMinor ||
			line.MeasureUnderfillBps < 0 || line.MeasureUnderfillBps > 10000 ||
			line.MeasureOverfillBps < 0 || line.MeasureOverfillBps > 10000 {
			return ErrSourceNotQuotable
		}
		if _, duplicate := lineIDs[line.CheckoutLineID]; duplicate {
			return ErrSourceNotQuotable
		}
		lineIDs[line.CheckoutLineID] = struct{}{}
		var err error
		lineTotal, err = safeAdd(lineTotal, line.QuotedTotalMinor)
		if err != nil {
			return err
		}
		lineMax, err = safeAdd(lineMax, line.MaxAuthorizedTotalMinor)
		if err != nil {
			return err
		}
	}
	if lineTotal != source.QuotedItemsTotalMinor || lineMax != source.MaxAuthorizedItemsTotalMinor {
		return ErrSourceNotQuotable
	}
	return nil
}

func validateAdjustment(value Adjustment) error {
	if strings.TrimSpace(value.ID) == "" || strings.TrimSpace(value.SourceRef) == "" || value.AmountMinor <= 0 {
		return ErrInvalidAdjustment
	}
	if value.Scope != ScopeProduct && value.Scope != ScopeDelivery && value.Scope != ScopeOrder {
		return ErrInvalidAdjustment
	}
	if value.Type != AdjustmentDiscount && value.Type != AdjustmentFee && value.Type != AdjustmentTax {
		return ErrInvalidAdjustment
	}
	if value.Type == AdjustmentDiscount {
		if value.FundingSource == FundingCustomer ||
			(value.FundingSource == FundingDeliverySubsidy && value.Scope != ScopeDelivery) {
			return ErrInvalidAdjustment
		}
	} else if value.FundingSource != FundingCustomer || value.Scope != ScopeOrder {
		return ErrInvalidAdjustment
	}
	return nil
}

func attributeDiscountFunding(funding *FundingBreakdown, adjustment Adjustment) error {
	var target *int64
	switch adjustment.Scope {
	case ScopeProduct:
		switch adjustment.FundingSource {
		case FundingVendor:
			target = &funding.ProductVendorFundedMinor
		case FundingPlatform:
			target = &funding.ProductPlatformFundedMinor
		case FundingOtherApproved:
			target = &funding.ProductOtherFundedMinor
		}
	case ScopeDelivery:
		if adjustment.AmountMinor > funding.DeliveryCustomerMinor {
			return ErrInvalidAdjustment
		}
		funding.DeliveryCustomerMinor -= adjustment.AmountMinor
		switch adjustment.FundingSource {
		case FundingVendor:
			target = &funding.DeliveryVendorFundedMinor
		case FundingPlatform:
			target = &funding.DeliveryPlatformFundedMinor
		case FundingDeliverySubsidy, FundingOtherApproved:
			target = &funding.DeliveryOtherFundedMinor
		}
	case ScopeOrder:
		switch adjustment.FundingSource {
		case FundingVendor:
			target = &funding.OrderVendorFundedMinor
		case FundingPlatform:
			target = &funding.OrderPlatformFundedMinor
		case FundingOtherApproved:
			target = &funding.OrderOtherFundedMinor
		}
	}
	if target == nil {
		return ErrInvalidAdjustment
	}
	value, err := safeAdd(*target, adjustment.AmountMinor)
	if err == nil {
		*target = value
	}
	return err
}

func safeAdd(values ...int64) (int64, error) {
	var total int64
	for _, value := range values {
		if value < 0 || total > math.MaxInt64-value {
			return 0, ErrInvalidAdjustment
		}
		total += value
	}
	return total, nil
}

// AssessReplan prepares the commercial decision for a later payment-capable
// section without moving money. A more expensive replacement can never be
// applied to the old approval; it requires a newly approved agreement.
func AssessReplan(current, candidate Quote) (ReplanAssessment, error) {
	if current.ID == "" || candidate.ID == "" || current.CustomerID == "" ||
		current.CustomerID != candidate.CustomerID ||
		current.CheckoutSessionID != candidate.CheckoutSessionID ||
		current.Currency != candidate.Currency || current.ApprovedAt == nil {
		return ReplanAssessment{}, ErrQuoteConflict
	}
	assessment := ReplanAssessment{
		CurrentQuoteID: current.ID, CandidateQuoteID: candidate.ID,
		CurrentTotalMinor: current.QuotedTotalMinor, CandidateTotalMinor: candidate.QuotedTotalMinor,
	}
	switch {
	case candidate.QuotedTotalMinor < current.QuotedTotalMinor:
		assessment.DifferenceMinor = current.QuotedTotalMinor - candidate.QuotedTotalMinor
		assessment.Disposition = ReplanReleaseOrRefundDifference
	case candidate.QuotedTotalMinor > current.QuotedTotalMinor:
		assessment.DifferenceMinor = candidate.QuotedTotalMinor - current.QuotedTotalMinor
		assessment.Disposition = ReplanCustomerReapproval
	default:
		assessment.Disposition = ReplanUnchanged
	}
	return assessment, nil
}
