package orders

// Section 17F payment usecases.
//
// The flow is deliberately order-first: the ParentOrder and its VendorOrders
// are created (exactly once, from an approved quote) BEFORE any money is asked
// for. That is what makes each payment leg able to name a real VendorOrder as
// its source reference, and it removes the worst failure mode by construction —
// there is no window where money exists for an order that was never created.
//
// The remaining window, money held for an order whose legs did not all answer,
// is durable and recoverable: every leg row is committed before the first call
// to the money authority.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

func orderHasVariableMeasurements(order ParentOrder) bool {
	for _, vendor := range order.VendorOrders {
		for _, line := range vendor.Lines {
			if line.PricingBasis == "VARIABLE_MEASURE" {
				return true
			}
		}
	}
	return false
}

// PaymentOptions extends the order service with the money authority.
type PaymentOptions struct {
	Authority PaymentAuthority

	// CustomerEmail resolves the email the Wallet authority requires for a
	// card-capable intent. It is a port so this feature never learns how to
	// reach Customer Service.
	CustomerEmail CustomerEmailResolver
}

// CustomerEmailResolver answers the customer's email for a payment.
type CustomerEmailResolver interface {
	CustomerEmail(ctx context.Context, customerID string) (string, error)
}

// noCustomerEmail is the safe default. An absent email is not an error: the
// Wallet authority only requires one for card-capable methods, and Section 17F
// pays from wallet balance.
type noCustomerEmail struct{}

func (noCustomerEmail) CustomerEmail(context.Context, string) (string, error) {
	return "", nil
}

// SetPaymentAuthority wires the money authority onto the order service.
//
// It is separate from NewService so a deployment without Wallet credentials
// builds a fully working order service with no payment surface at all, rather
// than a payment surface that can only ever fail.
func (s *Service) SetPaymentAuthority(options PaymentOptions) {
	s.payments = options.Authority
	s.customerEmail = options.CustomerEmail
	if s.customerEmail == nil {
		s.customerEmail = noCustomerEmail{}
	}
}

// PaymentEnabled reports whether a real money authority is wired.
func (s *Service) PaymentEnabled() bool {
	return s != nil && s.payments != nil && s.payments.Configured()
}

// StartPayment binds a payment operation to an approved quote and asks the
// money authority to hold each leg.
//
// Security properties, all enforced here rather than trusted from the caller:
//
//   - the quote must still be approved and unexpired, re-verified at THIS
//     moment rather than at approval time;
//   - the presented fingerprint must match the approved agreement, so a client
//     cannot swap one quote for another;
//   - every amount is read from the stored order, never from the request, so a
//     tampered total has nothing to tamper with;
//   - an operation already bound to a different agreement is refused rather
//     than reused.
func (s *Service) StartPayment(
	ctx context.Context, customerID, quoteID, fingerprint string,
) (ParentOrder, PaymentOperation, error) {
	if !s.enabled {
		return ParentOrder{}, PaymentOperation{}, ErrDisabled
	}
	if !s.PaymentEnabled() {
		return ParentOrder{}, PaymentOperation{}, ErrPaymentNotConfigured
	}

	customerID = strings.TrimSpace(customerID)
	quoteID = strings.TrimSpace(quoteID)
	fingerprint = strings.TrimSpace(fingerprint)
	if customerID == "" || quoteID == "" || fingerprint == "" {
		return ParentOrder{}, PaymentOperation{}, ErrQuoteNotOrderable
	}

	// Creating the order first is what gives each leg a real VendorOrder to
	// name. It is exactly-once per approved quote, so a retried payment start
	// finds the same order rather than creating a second one.
	//
	// A quote that has expired, lost its stock reservation or had its checkout
	// revalidated is refused here, before any money is asked for.
	order, _, err := s.CreateFromQuote(ctx, customerID, quoteID, fingerprint)
	if err != nil {
		return ParentOrder{}, PaymentOperation{}, err
	}

	binding, err := BindingFor(order)
	if err != nil {
		return ParentOrder{}, PaymentOperation{}, err
	}
	// The presented fingerprint must be the one the order was cut from. This
	// is a second, independent check: CreateFromQuote verified it against the
	// live quote, and this verifies it against the frozen order.
	if binding.QuoteFingerprint != fingerprint {
		return ParentOrder{}, PaymentOperation{}, ErrFingerprintMismatch
	}

	legs, err := PlanPaymentLegs(order)
	if err != nil {
		return ParentOrder{}, PaymentOperation{}, err
	}

	operation, _, err := s.store.EnsureOperation(ctx, order, binding, legs, s.now().UTC())
	if err != nil {
		return ParentOrder{}, PaymentOperation{}, err
	}
	// Bind the parent aggregate to the durable operation before capability
	// evidence or any Wallet call is made. Finalization and recovery use this
	// reference to read the exact operation's authority answer.
	if _, err = s.store.db.Exec(ctx, `UPDATE commerce_parent_orders SET payment_operation_ref=$1,version=version+1,updated_at=$2 WHERE id=$3 AND (payment_operation_ref='' OR payment_operation_ref IS NULL)`, operation.ID, s.now().UTC(), order.ID); err != nil {
		return ParentOrder{}, PaymentOperation{}, err
	}
	if capabilityAuthority, ok := s.payments.(PaymentCapabilityAuthority); ok {
		capability, capErr := capabilityAuthority.PaymentCapability(ctx, order.ID, operation.ID)
		if capErr != nil {
			return ParentOrder{}, PaymentOperation{}, capErr
		}
		if orderHasVariableMeasurements(order) && capability == PaymentUnsupported {
			return ParentOrder{}, PaymentOperation{}, ErrUnknownPaymentCapability
		}
		if _, capErr = s.store.SnapshotPaymentCapability(ctx, order.ID, operation.ID, string(capability), s.now().UTC()); capErr != nil {
			return ParentOrder{}, PaymentOperation{}, capErr
		}
		if orderHasVariableMeasurements(order) {
			if _, capErr = s.store.db.Exec(ctx, `UPDATE commerce_variable_measurements SET wallet_capability_snapshot=$1,updated_at=$2 WHERE parent_order_id=$3`, string(capability), s.now().UTC(), order.ID); capErr != nil {
				return ParentOrder{}, PaymentOperation{}, capErr
			}
		}
	} else if orderHasVariableMeasurements(order) {
		return ParentOrder{}, PaymentOperation{}, ErrUnknownPaymentCapability
	}

	// Every leg row is durable at this point, so a crash from here on is
	// recoverable by asking the authority about each one.
	operation, err = s.authorizeLegs(ctx, order, operation)
	if err != nil {
		return order, operation, err
	}

	refreshed, err := s.store.GetParent(ctx, customerID, order.ID)
	if err != nil {
		return order, operation, err
	}
	return refreshed, operation, nil
}

// authorizeLegs asks the money authority to hold each pending leg.
//
// It is safe to call repeatedly. Each leg carries a deterministic idempotency
// key, so a leg already held returns the same intent rather than a second one,
// and a leg already answered is skipped entirely.
func (s *Service) authorizeLegs(
	ctx context.Context, order ParentOrder, operation PaymentOperation,
) (PaymentOperation, error) {
	email, err := s.customerEmail.CustomerEmail(ctx, order.CustomerID)
	if err != nil {
		// An unavailable email lookup must not strand a payment. The wallet
		// method does not require one.
		email = ""
	}
	if authority, ok := s.payments.(CommercePaymentAuthority); ok {
		return s.authorizeCommercePayment(ctx, authority, order, operation, email)
	}

	current := operation
	for _, leg := range operation.Legs {
		if leg.Status != LegPending {
			continue
		}

		now := s.now().UTC()
		// Written before the call, so a crash mid-call still leaves evidence
		// that money may exist against this leg.
		if err = s.store.MarkLegRequested(ctx, leg, now); err != nil {
			return current, err
		}

		request := HoldLegRequest{
			IdempotencyKey:  leg.IdempotencyKey,
			SourceService:   PaymentSourceService,
			SourceReference: leg.VendorOrderID,
			CustomerID:      order.CustomerID,
			CustomerEmail:   email,
			AmountKobo:      leg.AmountMinor,
			Currency:        leg.Currency,
			Metadata: map[string]any{
				"parent_order_id":   order.ID,
				"quote_id":          order.QuoteID,
				"quote_fingerprint": order.QuoteFingerprint,
				"scope":             string(leg.Scope),
			},
		}
		if leg.Scope == ScopeVendorLeg {
			// Naming the beneficiary at creation is what lets the authority
			// settle this leg to this vendor, and only this vendor, later.
			request.ProviderType = PaymentProviderTypeVendor
			request.ProviderID = leg.BusinessID
		} else {
			// The platform leg names the ParentOrder, because it pays for the
			// order-level charge rather than for any vendor's goods.
			request.SourceReference = order.ID
		}

		result, holdErr := s.payments.HoldLeg(ctx, request)
		if holdErr == nil && result.AmountKobo != 0 && result.AmountKobo != leg.AmountMinor {
			// A Wallet answer for another amount is not a successful hold for
			// this leg. Keep it unresolved so recovery reads the authority again.
			holdErr = ErrPaymentAuthority
		}
		failureReason := ""
		if holdErr != nil {
			// The authority's own words are recorded. Vendor Commerce never
			// decides that a payment succeeded, and never decides that a
			// timeout means failure: an unrecognised answer keeps the leg
			// PENDING so recovery re-reads it.
			failureReason = redactAuthorityError(holdErr)
			result = LegResult{Status: ""}
		}

		current, err = s.store.RecordLegResult(ctx, leg.ID, result, failureReason, s.now().UTC())
		if err != nil {
			return current, err
		}
		if holdErr != nil {
			return current, ErrPaymentAuthority
		}
	}
	if current.State == PaymentHeld && s.production != nil {
		for _, vendorOrder := range order.VendorOrders {
			if activateErr := s.production.ActivateVendorOrderSystem(ctx, vendorOrder.ID); activateErr != nil {
				return current, activateErr
			}
		}
	}
	return current, nil
}

func commerceAllocationSnapshot(vendor VendorOrder) (map[string]any, string, error) {
	value := map[string]any{
		"vendor_order_id": vendor.ID, "business_id": vendor.BusinessID, "branch_id": vendor.BranchID,
		"quote_group_ref": vendor.QuoteGroupRef, "currency": vendor.Currency,
		"product_subtotal_minor":         vendor.ProductSubtotalMinor,
		"product_vendor_funded_minor":    vendor.ProductVendorFundedMinor,
		"product_platform_funded_minor":  vendor.ProductPlatformFundedMinor,
		"product_other_funded_minor":     vendor.ProductOtherFundedMinor,
		"order_vendor_funded_minor":      vendor.OrderVendorFundedMinor,
		"order_platform_funded_minor":    vendor.OrderPlatformFundedMinor,
		"order_other_funded_minor":       vendor.OrderOtherFundedMinor,
		"delivery_responsibility":        vendor.DeliveryResponsibility,
		"delivery_gross_minor":           vendor.DeliveryGrossMinor,
		"delivery_customer_minor":        vendor.DeliveryCustomerMinor,
		"delivery_vendor_funded_minor":   vendor.DeliveryVendorFundedMinor,
		"delivery_platform_funded_minor": vendor.DeliveryPlatformFundedMinor,
		"delivery_other_funded_minor":    vendor.DeliveryOtherFundedMinor,
		"customer_payable_minor":         vendor.CustomerPayableMinor,
		"vendor_payable_minor":           vendor.VendorPayableMinor,
		"refund_policy_version":          vendor.RefundPolicyVersion,
		"promotion_snapshot":             json.RawMessage(vendor.PromotionSnapshot),
		"delivery_funding_snapshot":      json.RawMessage(vendor.DeliveryFundingSnapshot),
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	return value, hex.EncodeToString(sum[:]), nil
}

func commerceAllocations(order ParentOrder, operation PaymentOperation) ([]CommerceAllocationRequest, map[string]string, error) {
	values := make([]CommerceAllocationRequest, 0, len(order.VendorOrders)+1)
	fingerprints := make(map[string]string, len(order.VendorOrders))
	maxByVendor := make(map[string]int64, len(order.VendorOrders))
	var platformMax int64
	for _, leg := range operation.Legs {
		if leg.Scope == ScopeVendorLeg {
			maxByVendor[leg.VendorOrderID] = leg.AmountMinor
		} else if leg.Scope == ScopePlatformLeg {
			platformMax = leg.AmountMinor
		}
	}
	for _, vendor := range order.VendorOrders {
		snapshot, fingerprint, err := commerceAllocationSnapshot(vendor)
		if err != nil {
			return nil, nil, err
		}
		vendorDelivery, providerDelivery := int64(0), int64(0)
		deliveryResponsibility := "NONE"
		switch vendor.DeliveryResponsibility {
		case DeliveryVendorOwn:
			vendorDelivery = vendor.DeliveryGrossMinor
			deliveryResponsibility = "VENDOR"
		case DeliveryPlatform:
			providerDelivery = vendor.DeliveryGrossMinor
			deliveryResponsibility = "PLATFORM"
		case DeliveryNone:
		default:
			return nil, nil, ErrAllocationImbalance
		}
		vendorMerchandise := vendor.VendorPayableMinor - vendorDelivery
		if vendorMerchandise < 0 {
			return nil, nil, ErrAllocationImbalance
		}
		values = append(values, CommerceAllocationRequest{
			AllocationReference: "vendor-order:" + vendor.ID, Scope: "VENDOR", VendorOrderID: vendor.ID,
			VendorID: vendor.BusinessID, BranchID: vendor.BranchID, DeliveryResponsibility: deliveryResponsibility,
			CustomerPayableMinor: vendor.CustomerPayableMinor, MaxAuthorizedMinor: maxByVendor[vendor.ID], MerchandiseGrossMinor: vendor.ProductSubtotalMinor,
			VendorProductDiscountMinor:   vendor.ProductVendorFundedMinor + vendor.OrderVendorFundedMinor,
			PlatformProductDiscountMinor: vendor.ProductPlatformFundedMinor + vendor.OrderPlatformFundedMinor,
			DeliveryCustomerMinor:        vendor.DeliveryCustomerMinor, VendorDeliverySubsidyMinor: vendor.DeliveryVendorFundedMinor,
			PlatformDeliverySubsidyMinor:  vendor.DeliveryPlatformFundedMinor,
			OtherFundingMinor:             vendor.ProductOtherFundedMinor + vendor.OrderOtherFundedMinor + vendor.DeliveryOtherFundedMinor,
			VendorMerchandisePayableMinor: vendorMerchandise, VendorDeliveryPayableMinor: vendorDelivery,
			ProviderDeliveryPayableMinor: providerDelivery, Snapshot: snapshot, SnapshotFingerprint: fingerprint,
		})
		fingerprints[vendor.ID] = fingerprint
	}
	if order.PlatformFeeTaxMinor > 0 {
		snapshot := map[string]any{"parent_order_id": order.ID, "platform_fee_tax_minor": order.PlatformFeeTaxMinor, "currency": order.Currency}
		raw, _ := json.Marshal(snapshot)
		sum := sha256.Sum256(raw)
		values = append(values, CommerceAllocationRequest{AllocationReference: "parent-platform:" + order.ID, Scope: "PLATFORM", DeliveryResponsibility: "NONE", CustomerPayableMinor: order.PlatformFeeTaxMinor, MaxAuthorizedMinor: platformMax, PlatformRevenueMinor: order.PlatformFeeTaxMinor, Snapshot: snapshot, SnapshotFingerprint: hex.EncodeToString(sum[:])})
	}
	return values, fingerprints, nil
}

func (s *Service) authorizeCommercePayment(ctx context.Context, authority CommercePaymentAuthority, order ParentOrder, operation PaymentOperation, email string) (PaymentOperation, error) {
	allocations, fingerprints, err := commerceAllocations(order, operation)
	if err != nil {
		return operation, err
	}
	for _, leg := range operation.Legs {
		if leg.Status == LegPending {
			if err = s.store.MarkLegRequested(ctx, leg, s.now().UTC()); err != nil {
				return operation, err
			}
		}
	}
	request := CommercePaymentRequest{ParentOrderID: order.ID, CustomerID: order.CustomerID, CustomerEmail: email, QuoteID: order.QuoteID, QuoteVersion: order.QuoteVersion, QuoteFingerprint: order.QuoteFingerprint, FinalAuthorizedMinor: order.CustomerApprovedTotalMinor, MaxAuthorizedMinor: order.MaxAuthorizedTotalMinor, AllocationFingerprint: operation.Binding.AllocationFingerprint, Currency: order.Currency, PaymentMethod: "wallet", IdempotencyKey: "parent-order-payment-" + order.ID, Allocations: allocations}
	result, authorityErr := authority.AuthorizeCommercePayment(ctx, request)
	if authorityErr == nil && (!result.Held() || result.FinalAuthorizedMinor != request.FinalAuthorizedMinor || result.MaxAuthorizedMinor != request.MaxAuthorizedMinor || result.AllocationFingerprint != request.AllocationFingerprint) {
		authorityErr = ErrPaymentAuthority
	}
	if authorityErr == nil {
		for vendorOrderID, expected := range fingerprints {
			if result.AllocationFingerprints[vendorOrderID] != expected {
				authorityErr = ErrPaymentAuthority
				break
			}
		}
	}
	current := operation
	for _, leg := range operation.Legs {
		if leg.Status != LegPending {
			continue
		}
		legResult := LegResult{IntentID: result.IntentID, Reference: result.Reference, Status: result.Status, AmountKobo: leg.AmountMinor, SnapshotFingerprint: result.AllocationFingerprints[leg.VendorOrderID]}
		failure := ""
		if authorityErr != nil {
			legResult.Status = ""
			failure = redactAuthorityError(authorityErr)
		}
		current, err = s.store.RecordLegResult(ctx, leg.ID, legResult, failure, s.now().UTC())
		if err != nil {
			return current, err
		}
	}
	if authorityErr != nil {
		return current, ErrPaymentAuthority
	}
	if current.State == PaymentHeld && s.production != nil {
		for _, vendor := range order.VendorOrders {
			if err = s.production.ActivateVendorOrderSystem(ctx, vendor.ID); err != nil {
				return current, err
			}
		}
	}
	return current, nil
}

// GetPaymentForCustomer reads one order's payment operation.
func (s *Service) GetPaymentForCustomer(
	ctx context.Context, customerID, parentOrderID string,
) (PaymentOperation, error) {
	if !s.enabled {
		return PaymentOperation{}, ErrDisabled
	}
	return s.store.GetOperationForOrder(ctx,
		strings.TrimSpace(customerID), strings.TrimSpace(parentOrderID))
}

// PaymentSourceService is the identity Vendor Commerce presents to the money
// authority. payment-wallet-service records it on every intent and ledger row,
// so a settlement is always traceable back to commerce.
const PaymentSourceService = "vendor-service"

// PaymentProviderTypeVendor is the provider type a vendor leg settles to.
const PaymentProviderTypeVendor = "vendor"

// redactAuthorityError keeps an authority failure loggable without carrying
// anything that is not a status or a reason.
func redactAuthorityError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	const limit = 200
	if len(message) > limit {
		return message[:limit]
	}
	return message
}
