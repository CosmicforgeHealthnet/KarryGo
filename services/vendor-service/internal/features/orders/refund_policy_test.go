package orders

import (
	"math"
	"testing"
)

func refundTestOrder() VendorOrder {
	return VendorOrder{
		ID: "vo-a", Domain: "SHOP", CustomerPayableMinor: 7_500, VendorPayableMinor: 7_000,
		ProductSubtotalMinor: 7_000, DeliveryResponsibility: DeliveryPlatform, DeliveryGrossMinor: 1_000,
		DeliveryCustomerMinor: 500, DeliveryPlatformFundedMinor: 500,
		Lines: []OrderLine{{ID: "line-a", QuotedTotalMinor: 7_000}},
	}
}

func TestRefundPolicyVendorRejectionIsAllocationScoped(t *testing.T) {
	d, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerVendorRejected, Order: refundTestOrder(), PreviousStatus: VendorStatusAccepted, MaxAuthorizedMinor: 7_500})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Automatic || d.Action != RefundActionAllocation || d.CustomerRefundMinor != 7_500 || d.PlatformFundingReversalMinor != 500 || d.ProviderDeliveryReductionMinor != 1_000 {
		t.Fatalf("decision=%+v", d)
	}
}

func TestRefundPolicyAfterPreparationRoutesToSupport(t *testing.T) {
	d, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerVendorCancelled, Order: refundTestOrder(), PreviousStatus: VendorStatusPreparing, MaxAuthorizedMinor: 7_500})
	if err != nil || !d.SupportRequired || d.Automatic || d.OutputMinor() != 0 {
		t.Fatalf("decision=%+v err=%v", d, err)
	}
}

func TestRefundPolicyAutoRefundUsesFrozenLineFunding(t *testing.T) {
	order := refundTestOrder()
	order.Lines[0].DiscountAllocatedMinor = 1_000
	order.ProductVendorFundedMinor, order.ProductPlatformFundedMinor = 400, 600
	d, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerItemUnavailable, Order: order, OrderLineID: "line-a", SubstitutionPolicy: PolicyAutoRefund})
	if err != nil {
		t.Fatal(err)
	}
	if d.CustomerRefundMinor != 6_000 || d.PlatformFundingReversalMinor != 600 || d.VendorMerchandiseReductionMinor != 6_600 {
		t.Fatalf("decision=%+v", d)
	}
}

func TestRefundPolicyCheaperReplanBoundedToCustomerContribution(t *testing.T) {
	order := refundTestOrder()
	if _, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerCheaperReplan, Order: order, DifferenceMinor: 501, PlanVersion: 2}); err == nil {
		t.Fatal("accepted refund above customer delivery contribution")
	}
	d, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerCheaperReplan, Order: order, DifferenceMinor: 300, PlanVersion: 2})
	if err != nil || d.CustomerRefundMinor != 300 || d.ProviderDeliveryReductionMinor != 300 {
		t.Fatalf("decision=%+v err=%v", d, err)
	}
}

func TestRefundPolicyProviderEarningPreservedByExplicitPlatformLoss(t *testing.T) {
	d, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerDeliveryFailure, Order: refundTestOrder(), DeliveryFailurePolicy: DeliveryFailureDeliveryOnly, PreserveProviderEarning: true})
	if err != nil || d.ProviderDeliveryReductionMinor != 0 || d.PlatformLossFundingMinor != 500 || d.CustomerRefundMinor != 500 {
		t.Fatalf("decision=%+v err=%v", d, err)
	}
}

func TestRefundPolicyPreparedFoodFailureDoesNotGuess(t *testing.T) {
	order := refundTestOrder()
	order.Domain = "FOOD"
	d, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerDeliveryFailure, Order: order, DeliveryFailurePolicy: DeliveryFailureGoodsAndDelivery, MaxAuthorizedMinor: 7_500})
	if err != nil || !d.SupportRequired || d.Automatic {
		t.Fatalf("decision=%+v err=%v", d, err)
	}
}

func TestRefundPolicyDoesNotRefundReplannableProviderFailure(t *testing.T) {
	d, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerDeliveryFailure, Order: refundTestOrder(), DeliveryFailurePolicy: DeliveryFailureReplanPossible})
	if err != nil || d.Automatic || d.SupportRequired || d.Action != RefundActionNone || d.ReasonCode != "DELIVERY_REPLAN_REMAINS_POSSIBLE" {
		t.Fatalf("decision=%+v err=%v", d, err)
	}
}

func TestRefundPolicyCustomerCausedFailureDoesNotMoveMoney(t *testing.T) {
	d, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerDeliveryFailure, Order: refundTestOrder(), DeliveryFailurePolicy: DeliveryFailureCustomerCaused})
	if err != nil || d.Automatic || d.SupportRequired || d.Action != RefundActionNone || d.OutputMinor() != 0 {
		t.Fatalf("decision=%+v err=%v", d, err)
	}
}

func TestRefundPolicyAmbiguousFailureRequiresSupportWithoutMoney(t *testing.T) {
	d, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerDeliveryFailure, Order: refundTestOrder(), DeliveryFailurePolicy: DeliveryFailureAmbiguous})
	if err != nil || d.Automatic || !d.SupportRequired || d.Action != RefundActionSupport || d.OutputMinor() != 0 {
		t.Fatalf("decision=%+v err=%v", d, err)
	}
}

func TestRefundPolicyShopGoodsAndDeliveryIsAllocationScoped(t *testing.T) {
	d, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerDeliveryFailure, Order: refundTestOrder(), DeliveryFailurePolicy: DeliveryFailureGoodsAndDelivery, MaxAuthorizedMinor: 7_500})
	if err != nil || !d.Automatic || d.Action != RefundActionGoodsAndDelivery || d.CustomerRefundMinor != 7_500 || d.PlatformFundingReversalMinor != 500 {
		t.Fatalf("decision=%+v err=%v", d, err)
	}
}

func TestCommerceRefundRecoveryUsesOnlyAllocationReleaseState(t *testing.T) {
	decision, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerVendorRejected, Order: refundTestOrder(), PreviousStatus: VendorStatusAccepted, MaxAuthorizedMinor: 7_500})
	if err != nil {
		t.Fatal(err)
	}
	record := RefundPolicyRecord{ID: "decision-a", VendorOrderID: "vo-a", IdempotencyKey: "refund-a", Decision: decision}
	held, err := BuildCommerceRefundRequest(record, CommerceAllocationState{SnapshotFingerprint: "snapshot-a"})
	if err != nil || held.EscrowRecoveryMinor != 8_000 || held.VendorRecoveryMinor != 0 || held.ProviderRecoveryMinor != 0 {
		t.Fatalf("held request=%+v err=%v", held, err)
	}
	released, err := BuildCommerceRefundRequest(record, CommerceAllocationState{SnapshotFingerprint: "snapshot-a", VendorReleasedMinor: 7_000, DeliveryReleasedMinor: 1_000})
	if err != nil || released.EscrowRecoveryMinor != 0 || released.VendorRecoveryMinor != 7_000 || released.ProviderRecoveryMinor != 1_000 {
		t.Fatalf("released request=%+v err=%v", released, err)
	}
}

func TestCommerceRefundRecoveryPreservesProviderThroughPlatformLoss(t *testing.T) {
	decision, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerDeliveryFailure, Order: refundTestOrder(), DeliveryFailurePolicy: DeliveryFailureDeliveryOnly, PreserveProviderEarning: true})
	if err != nil {
		t.Fatal(err)
	}
	request, err := BuildCommerceRefundRequest(RefundPolicyRecord{ID: "decision-a", VendorOrderID: "vo-a", IdempotencyKey: "refund-a", Decision: decision}, CommerceAllocationState{SnapshotFingerprint: "snapshot-a", DeliveryReleasedMinor: 1_000})
	if err != nil || request.PlatformLossFundingMinor != 500 || request.ProviderRecoveryMinor != 0 || request.EscrowRecoveryMinor != 0 {
		t.Fatalf("request=%+v err=%v", request, err)
	}
}

func TestVariableFinalizationSeparatesAuthorizationRelease(t *testing.T) {
	d, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerVariableFinal, Order: refundTestOrder(), OrderLineID: "line-a", DifferenceMinor: 700, AuthorizationReleaseMinor: 400})
	if err != nil || d.CustomerRefundMinor != 700 || d.AuthorizationReleaseMinor != 400 || d.VendorMerchandiseReductionMinor != 300 {
		t.Fatalf("decision=%+v err=%v", d, err)
	}
}

func TestRefundPolicyRejectsFundingOverflow(t *testing.T) {
	order := refundTestOrder()
	order.ProductPlatformFundedMinor = math.MaxInt64
	order.OrderPlatformFundedMinor = 1
	if _, err := EvaluateCommerceRefundPolicy(RefundPolicyInput{Trigger: RefundTriggerVendorRejected, Order: order, PreviousStatus: VendorStatusAccepted, MaxAuthorizedMinor: 7_500}); err == nil {
		t.Fatal("overflowing immutable funding snapshot was accepted")
	}
}

func TestCommerceRefundRequestRejectsReleasedRecoveryOverflow(t *testing.T) {
	decision := RefundPolicyDecision{
		PolicyVersion: CommerceRefundPolicyVersion, Trigger: RefundTriggerDeliveryFailure,
		Action: RefundActionDelivery, ReasonCode: "TEST", Automatic: true,
		CustomerRefundMinor: math.MaxInt64, VendorMerchandiseReductionMinor: math.MaxInt64,
		VendorDeliveryReductionMinor: 1,
	}
	record := RefundPolicyRecord{ID: "decision-a", VendorOrderID: "vo-a", IdempotencyKey: "refund-a", Decision: decision}
	if _, err := BuildCommerceRefundRequest(record, CommerceAllocationState{SnapshotFingerprint: "snapshot", VendorReleasedMinor: 1, DeliveryReleasedMinor: 1}); err == nil {
		t.Fatal("overflowing released recovery sources were accepted")
	}
}
