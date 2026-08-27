package orders

import (
	"math"
	"math/big"
	"strings"
)

const CommerceRefundPolicyVersion = "commerce-refund-policy.v1"

type RefundPolicyTrigger string

const (
	RefundTriggerVendorRejected     RefundPolicyTrigger = "VENDOR_REJECTED"
	RefundTriggerVendorCancelled    RefundPolicyTrigger = "VENDOR_CANCELLED"
	RefundTriggerItemUnavailable    RefundPolicyTrigger = "ITEM_UNAVAILABLE"
	RefundTriggerReplacementCheaper RefundPolicyTrigger = "REPLACEMENT_PRICE_DECREASE"
	RefundTriggerVariableFinal      RefundPolicyTrigger = "VARIABLE_WEIGHT_FINALIZATION"
	RefundTriggerCheaperReplan      RefundPolicyTrigger = "FULFILMENT_REPLAN_PRICE_DECREASE"
	RefundTriggerDeliveryFailure    RefundPolicyTrigger = "DELIVERY_FAILURE"
)

type RefundPolicyAction string

const (
	RefundActionNone             RefundPolicyAction = "NONE"
	RefundActionAllocation       RefundPolicyAction = "REFUND_VENDOR_ALLOCATION"
	RefundActionItem             RefundPolicyAction = "REFUND_ITEM_ALLOCATION"
	RefundActionDifference       RefundPolicyAction = "REFUND_DIFFERENCE"
	RefundActionDelivery         RefundPolicyAction = "REFUND_DELIVERY"
	RefundActionGoodsAndDelivery RefundPolicyAction = "REFUND_GOODS_AND_DELIVERY"
	RefundActionSupport          RefundPolicyAction = "SUPPORT_CASE_REQUIRED"
)

type DeliveryFailurePolicy string

const (
	DeliveryFailureDeliveryOnly     DeliveryFailurePolicy = "DELIVERY_ONLY"
	DeliveryFailureGoodsAndDelivery DeliveryFailurePolicy = "GOODS_AND_DELIVERY"
	DeliveryFailureCustomerCaused   DeliveryFailurePolicy = "CUSTOMER_CAUSED_NO_AUTO_REFUND"
	DeliveryFailureReplanPossible   DeliveryFailurePolicy = "REPLAN_REQUIRED_NO_REFUND"
	DeliveryFailureAmbiguous        DeliveryFailurePolicy = "AMBIGUOUS_SUPPORT_REVIEW"
)

type RefundPolicyInput struct {
	Trigger                   RefundPolicyTrigger
	Order                     VendorOrder
	PreviousStatus            VendorStatus
	OrderLineID               string
	DifferenceMinor           int64
	MaxAuthorizedMinor        int64
	AuthorizationReleaseMinor int64
	SubstitutionPolicy        SubstitutionPolicy
	DeliveryFailurePolicy     DeliveryFailurePolicy
	PreserveProviderEarning   bool
	PlanVersion               int64
}

// RefundPolicyDecision contains economic instructions, never ledger entries.
// Wallet remains responsible for validating the immutable allocation and
// executing the money movement.
type RefundPolicyDecision struct {
	PolicyVersion                   string
	Trigger                         RefundPolicyTrigger
	Action                          RefundPolicyAction
	ReasonCode                      string
	Automatic                       bool
	SupportRequired                 bool
	OrderLineID                     string
	PlanVersion                     int64
	CustomerRefundMinor             int64
	PlatformFundingReversalMinor    int64
	OtherFundingReversalMinor       int64
	PlatformLossFundingMinor        int64
	VendorMerchandiseReductionMinor int64
	VendorDeliveryReductionMinor    int64
	ProviderDeliveryReductionMinor  int64
	PlatformRevenueReductionMinor   int64
	AuthorizationReleaseMinor       int64
}

func (d RefundPolicyDecision) OutputMinor() int64 {
	return d.CustomerRefundMinor + d.PlatformFundingReversalMinor + d.OtherFundingReversalMinor
}

func (d RefundPolicyDecision) AllocationReductionMinor() int64 {
	return d.VendorMerchandiseReductionMinor + d.VendorDeliveryReductionMinor +
		d.ProviderDeliveryReductionMinor + d.PlatformRevenueReductionMinor + d.AuthorizationReleaseMinor
}

func (d RefundPolicyDecision) Validate() error {
	values := []int64{d.CustomerRefundMinor, d.PlatformFundingReversalMinor, d.OtherFundingReversalMinor,
		d.PlatformLossFundingMinor, d.VendorMerchandiseReductionMinor, d.VendorDeliveryReductionMinor,
		d.ProviderDeliveryReductionMinor, d.PlatformRevenueReductionMinor, d.AuthorizationReleaseMinor}
	for _, value := range values {
		if value < 0 {
			return ErrAllocationImbalance
		}
	}
	output, ok := checkedMinorSum(d.CustomerRefundMinor, d.PlatformFundingReversalMinor, d.OtherFundingReversalMinor)
	if !ok {
		return ErrAllocationImbalance
	}
	reduction, ok := checkedMinorSum(d.VendorMerchandiseReductionMinor, d.VendorDeliveryReductionMinor, d.ProviderDeliveryReductionMinor, d.PlatformRevenueReductionMinor, d.AuthorizationReleaseMinor)
	if !ok {
		return ErrAllocationImbalance
	}
	if d.PolicyVersion == "" || d.ReasonCode == "" {
		return ErrOrderConflict
	}
	if d.SupportRequired {
		if d.Automatic || d.Action != RefundActionSupport || d.OutputMinor() != 0 {
			return ErrOrderConflict
		}
		return nil
	}
	if d.Action == RefundActionNone {
		if d.Automatic || d.OutputMinor() != 0 {
			return ErrOrderConflict
		}
		return nil
	}
	if !d.Automatic || output <= 0 || reduction != output-d.PlatformLossFundingMinor {
		return ErrAllocationImbalance
	}
	return nil
}

func EvaluateCommerceRefundPolicy(input RefundPolicyInput) (RefundPolicyDecision, error) {
	policyVersion := strings.TrimSpace(input.Order.RefundPolicyVersion)
	if policyVersion == "" {
		policyVersion = CommerceRefundPolicyVersion
	}
	if policyVersion != CommerceRefundPolicyVersion {
		return RefundPolicyDecision{}, ErrOrderConflict
	}
	base := RefundPolicyDecision{PolicyVersion: policyVersion, Trigger: input.Trigger, PlanVersion: input.PlanVersion}
	switch input.Trigger {
	case RefundTriggerVendorRejected, RefundTriggerVendorCancelled:
		if !beforePreparation(input.PreviousStatus) {
			base.Action, base.ReasonCode, base.SupportRequired = RefundActionSupport, "VALUE_CONSUMPTION_REQUIRES_REVIEW", true
			return base, base.Validate()
		}
		decision, err := fullAllocationRefund(input.Order, input.MaxAuthorizedMinor)
		if err != nil {
			return RefundPolicyDecision{}, err
		}
		decision.Trigger = input.Trigger
		decision.PolicyVersion = policyVersion
		decision.Action = RefundActionAllocation
		decision.Automatic = true
		if input.Trigger == RefundTriggerVendorRejected {
			decision.ReasonCode = "VENDOR_REJECTED_PREPARATION_NOT_STARTED"
		} else {
			decision.ReasonCode = "VENDOR_CANCELLED_PREPARATION_NOT_STARTED"
		}
		return decision, decision.Validate()

	case RefundTriggerItemUnavailable:
		if input.SubstitutionPolicy != PolicyAutoRefund {
			base.Action, base.ReasonCode = RefundActionNone, "ITEM_UNAVAILABLE_REPLACEMENT_POLICY"
			return base, base.Validate()
		}
		line, ok := orderLine(input.Order, input.OrderLineID)
		if !ok {
			return RefundPolicyDecision{}, ErrOrderNotFound
		}
		vendorFunding, platformFunding, otherFunding, err := lineFunding(input.Order, line)
		if err != nil {
			return RefundPolicyDecision{}, err
		}
		customer := line.QuotedTotalMinor - line.DiscountAllocatedMinor
		if customer < 0 {
			return RefundPolicyDecision{}, ErrAllocationImbalance
		}
		decision := base
		decision.Action, decision.Automatic = RefundActionItem, true
		decision.ReasonCode, decision.OrderLineID = "ITEM_UNAVAILABLE_AUTO_REFUND", line.ID
		decision.CustomerRefundMinor = customer
		decision.PlatformFundingReversalMinor = platformFunding
		decision.OtherFundingReversalMinor = otherFunding
		decision.VendorMerchandiseReductionMinor = line.QuotedTotalMinor - vendorFunding
		return decision, decision.Validate()

	case RefundTriggerReplacementCheaper:
		if input.DifferenceMinor <= 0 {
			return RefundPolicyDecision{}, ErrAdjustmentNotAvailable
		}
		base.Action, base.Automatic = RefundActionDifference, true
		base.ReasonCode, base.OrderLineID = "REPLACEMENT_PRICE_DECREASE", strings.TrimSpace(input.OrderLineID)
		base.CustomerRefundMinor = input.DifferenceMinor
		base.VendorMerchandiseReductionMinor = input.DifferenceMinor
		return base, base.Validate()

	case RefundTriggerVariableFinal:
		if input.DifferenceMinor <= 0 || input.AuthorizationReleaseMinor < 0 || input.AuthorizationReleaseMinor > input.DifferenceMinor {
			return RefundPolicyDecision{}, ErrAdjustmentNotAvailable
		}
		base.Action, base.Automatic = RefundActionDifference, true
		base.ReasonCode, base.OrderLineID = "VARIABLE_WEIGHT_FINALIZATION", strings.TrimSpace(input.OrderLineID)
		base.CustomerRefundMinor = input.DifferenceMinor
		base.AuthorizationReleaseMinor = input.AuthorizationReleaseMinor
		base.VendorMerchandiseReductionMinor = input.DifferenceMinor - base.AuthorizationReleaseMinor
		return base, base.Validate()

	case RefundTriggerCheaperReplan:
		if input.DifferenceMinor <= 0 || input.DifferenceMinor > input.Order.DeliveryCustomerMinor || input.PlanVersion <= 0 {
			return RefundPolicyDecision{}, ErrAdjustmentNotAvailable
		}
		base.Action, base.Automatic, base.ReasonCode = RefundActionDelivery, true, "FULFILMENT_REPLAN_PRICE_DECREASE"
		base.CustomerRefundMinor = input.DifferenceMinor
		if input.Order.DeliveryResponsibility == DeliveryVendorOwn {
			base.VendorDeliveryReductionMinor = input.DifferenceMinor
		} else if input.Order.DeliveryResponsibility == DeliveryPlatform {
			base.ProviderDeliveryReductionMinor = input.DifferenceMinor
		} else {
			return RefundPolicyDecision{}, ErrAdjustmentNotAvailable
		}
		return base, base.Validate()

	case RefundTriggerDeliveryFailure:
		switch input.DeliveryFailurePolicy {
		case DeliveryFailureCustomerCaused:
			base.Action, base.ReasonCode = RefundActionNone, "CUSTOMER_CAUSED_DELIVERY_FAILURE"
			return base, base.Validate()
		case DeliveryFailureReplanPossible:
			base.Action, base.ReasonCode = RefundActionNone, "DELIVERY_REPLAN_REMAINS_POSSIBLE"
			return base, base.Validate()
		case DeliveryFailureAmbiguous:
			base.Action, base.ReasonCode, base.SupportRequired = RefundActionSupport, "AMBIGUOUS_DELIVERY_FAILURE", true
			return base, base.Validate()
		case DeliveryFailureDeliveryOnly:
			if input.Order.DeliveryCustomerMinor <= 0 {
				return RefundPolicyDecision{}, ErrAdjustmentNotAvailable
			}
			base.Action, base.Automatic, base.ReasonCode = RefundActionDelivery, true, "QUALIFYING_DELIVERY_FAILURE"
			base.CustomerRefundMinor = input.Order.DeliveryCustomerMinor
			if input.PreserveProviderEarning && input.Order.DeliveryResponsibility == DeliveryPlatform {
				base.PlatformLossFundingMinor = base.CustomerRefundMinor
			} else if input.Order.DeliveryResponsibility == DeliveryPlatform {
				base.ProviderDeliveryReductionMinor = base.CustomerRefundMinor
			} else if input.Order.DeliveryResponsibility == DeliveryVendorOwn {
				base.VendorDeliveryReductionMinor = base.CustomerRefundMinor
			}
			return base, base.Validate()
		case DeliveryFailureGoodsAndDelivery:
			if strings.EqualFold(input.Order.Domain, "FOOD") {
				base.Action, base.ReasonCode, base.SupportRequired = RefundActionSupport, "PREPARED_FOOD_FAILURE_REQUIRES_REVIEW", true
				return base, base.Validate()
			}
			decision, err := fullAllocationRefund(input.Order, input.MaxAuthorizedMinor)
			if err != nil {
				return RefundPolicyDecision{}, err
			}
			decision.Trigger, decision.Action, decision.Automatic = input.Trigger, RefundActionGoodsAndDelivery, true
			decision.PolicyVersion = policyVersion
			decision.ReasonCode = "QUALIFYING_GOODS_AND_DELIVERY_FAILURE"
			if input.PreserveProviderEarning && decision.ProviderDeliveryReductionMinor > 0 {
				decision.PlatformLossFundingMinor += decision.ProviderDeliveryReductionMinor
				decision.ProviderDeliveryReductionMinor = 0
			}
			return decision, decision.Validate()
		default:
			return RefundPolicyDecision{}, ErrOrderConflict
		}
	default:
		return RefundPolicyDecision{}, ErrOrderConflict
	}
}

func fullAllocationRefund(order VendorOrder, maxAuthorized int64) (RefundPolicyDecision, error) {
	if maxAuthorized < order.CustomerPayableMinor || maxAuthorized < 0 {
		return RefundPolicyDecision{}, ErrAllocationImbalance
	}
	vendorDelivery, providerDelivery := int64(0), int64(0)
	switch order.DeliveryResponsibility {
	case DeliveryVendorOwn:
		vendorDelivery = order.DeliveryGrossMinor
	case DeliveryPlatform:
		providerDelivery = order.DeliveryGrossMinor
	case DeliveryNone:
	default:
		return RefundPolicyDecision{}, ErrAllocationImbalance
	}
	vendorMerchandise := order.VendorPayableMinor - vendorDelivery
	if vendorMerchandise < 0 {
		return RefundPolicyDecision{}, ErrAllocationImbalance
	}
	platformFunding, ok := checkedMinorSum(order.ProductPlatformFundedMinor, order.OrderPlatformFundedMinor, order.DeliveryPlatformFundedMinor)
	if !ok {
		return RefundPolicyDecision{}, ErrAllocationImbalance
	}
	otherFunding, ok := checkedMinorSum(order.ProductOtherFundedMinor, order.OrderOtherFundedMinor, order.DeliveryOtherFundedMinor)
	if !ok {
		return RefundPolicyDecision{}, ErrAllocationImbalance
	}
	decision := RefundPolicyDecision{
		PolicyVersion:                   CommerceRefundPolicyVersion,
		CustomerRefundMinor:             maxAuthorized,
		PlatformFundingReversalMinor:    platformFunding,
		OtherFundingReversalMinor:       otherFunding,
		VendorMerchandiseReductionMinor: vendorMerchandise,
		VendorDeliveryReductionMinor:    vendorDelivery,
		ProviderDeliveryReductionMinor:  providerDelivery,
		AuthorizationReleaseMinor:       maxAuthorized - order.CustomerPayableMinor,
	}
	output, outputOK := checkedMinorSum(decision.CustomerRefundMinor, decision.PlatformFundingReversalMinor, decision.OtherFundingReversalMinor)
	reduction, reductionOK := checkedMinorSum(decision.VendorMerchandiseReductionMinor, decision.VendorDeliveryReductionMinor, decision.ProviderDeliveryReductionMinor, decision.PlatformRevenueReductionMinor, decision.AuthorizationReleaseMinor)
	if !outputOK || !reductionOK || output != reduction {
		return RefundPolicyDecision{}, ErrAllocationImbalance
	}
	return decision, nil
}

func checkedMinorSum(values ...int64) (int64, bool) {
	var total int64
	for _, value := range values {
		if value < 0 || total > math.MaxInt64-value {
			return 0, false
		}
		total += value
	}
	return total, true
}

func beforePreparation(status VendorStatus) bool {
	switch status {
	case VendorStatusCreated, VendorStatusPendingPayment, VendorStatusPaid, VendorStatusSentToVendor, VendorStatusAccepted:
		return true
	default:
		return false
	}
}

func orderLine(order VendorOrder, id string) (OrderLine, bool) {
	for _, line := range order.Lines {
		if line.ID == strings.TrimSpace(id) {
			return line, true
		}
	}
	return OrderLine{}, false
}

func lineFunding(order VendorOrder, line OrderLine) (vendor, platform, other int64, err error) {
	discount := line.DiscountAllocatedMinor
	if discount == 0 {
		return 0, 0, 0, nil
	}
	pairs := [][2]int64{{order.ProductVendorFundedMinor, order.OrderVendorFundedMinor}, {order.ProductPlatformFundedMinor, order.OrderPlatformFundedMinor}, {order.ProductOtherFundedMinor, order.OrderOtherFundedMinor}}
	weights := make([]int64, len(pairs))
	for i, pair := range pairs {
		if pair[0] < 0 || pair[1] < 0 || pair[0] > math.MaxInt64-pair[1] {
			return 0, 0, 0, ErrAllocationImbalance
		}
		weights[i] = pair[0] + pair[1]
	}
	var total int64
	for _, value := range weights {
		if value < 0 || total > math.MaxInt64-value {
			return 0, 0, 0, ErrAllocationImbalance
		}
		total += value
	}
	if total <= 0 || discount > total || discount > line.QuotedTotalMinor {
		return 0, 0, 0, ErrAllocationImbalance
	}
	shares := make([]int64, len(weights))
	var used int64
	for i, weight := range weights {
		product := new(big.Int).Mul(big.NewInt(discount), big.NewInt(weight))
		product.Quo(product, big.NewInt(total))
		if !product.IsInt64() {
			return 0, 0, 0, ErrAllocationImbalance
		}
		shares[i] = product.Int64()
		used += shares[i]
	}
	for i := 0; used < discount; i = (i + 1) % len(shares) {
		if weights[i] > 0 {
			shares[i]++
			used++
		}
	}
	return shares[0], shares[1], shares[2], nil
}
