package purchasing

import (
	"math"
	"testing"
)

func TestSection15C_AcceptedOnlyExample(t *testing.T) {
	line := InspectGoodsReceiptLineInput{
		DeliveredQuantity:   98,
		AcceptedQuantity:    90,
		DamagedQuantity:     5,
		RejectedQuantity:    3,
		ShortQuantity:       2,
		PurchaseOrderLineID: "pol_1",
	}

	if err := validateInspectionQuantities(line); err != nil {
		t.Fatalf("example should validate: %v", err)
	}

	inventory, err := multiplyExactInt64(
		line.AcceptedQuantity,
		1,
	)
	if err != nil {
		t.Fatal(err)
	}

	if inventory != 90 {
		t.Fatalf(
			"inventory quantity=%d want 90",
			inventory,
		)
	}
}

func TestSection15C_DeliveryEquationRejectsInflatedAcceptedStock(t *testing.T) {
	line := InspectGoodsReceiptLineInput{
		PurchaseOrderLineID: "pol_1",
		DeliveredQuantity:   98,
		AcceptedQuantity:    98,
		DamagedQuantity:     5,
		RejectedQuantity:    3,
	}

	if err := validateInspectionQuantities(line); err == nil {
		t.Fatal("invalid delivered/inspection equation was accepted")
	}
}

func TestSection15C_ExactConversionNoOverflowOrRounding(t *testing.T) {
	got, err := multiplyExactInt64(40, 12)
	if err != nil || got != 480 {
		t.Fatalf("exact conversion got=%d err=%v", got, err)
	}

	if _, err = multiplyExactInt64(
		math.MaxInt64,
		2,
	); err == nil {
		t.Fatal("overflowing conversion was silently accepted")
	}
}

func TestSection15C_OverDeliveryPolicy(t *testing.T) {
	max, err := allowedOverDelivery(100, 500)
	if err != nil {
		t.Fatal(err)
	}

	if max != 105 {
		t.Fatalf("max=%d want 105", max)
	}
}

func TestSection15C_CostVariancePolicy(t *testing.T) {
	policy := GoodsReceivingPolicy{
		RequireCostVarianceApproval: true,
		CostVarianceApprovalBps:     500,
		CostVarianceApprovalMinor:   1000,
	}

	if got := costDiscrepancyStatus(
		policy,
		4000,
		800,
	); got != DiscrepancyApprovalRequired {
		t.Fatalf("status=%s", got)
	}
}

func TestSection15C_WrongItemCannotEnterInventory(t *testing.T) {
	line := InspectGoodsReceiptLineInput{
		WrongItem:          true,
		WrongItemReference: "supplier-item-x",
		PurchaseUnitCode:   "CASE",
		DeliveredQuantity:  3,
		AcceptedQuantity:   1,
	}

	if err := validateInspectionQuantities(line); err == nil {
		t.Fatal("wrong item was allowed to carry accepted inventory")
	}
}
