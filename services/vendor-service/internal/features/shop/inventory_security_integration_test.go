package shop

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestInventoryTenantBranchIDORAndAdjustmentLimit(t *testing.T) {
	f := section12Fixture(t)
	ctx := context.Background()
	global := Scope{Global: true}
	basis := createInventoryBasis(t, f, "IDOR Stock", 20)
	receipt, err := f.service.CreateReceipt(ctx, f.actorA, global, CreateReceiptInput{BranchID: f.branchShop, SourceRef: "idor-receipt", Lines: []ReceiptLineInput{{StockBasisID: basis.ID, Quantity: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := f.service.ReserveInventory(ctx, f.actorA, global, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "idor-reservation-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	quarantine, err := f.service.QuarantineInventory(ctx, f.actorA, global, basis.ID, 2, "QUALITY_CHECK", "")
	if err != nil {
		t.Fatal(err)
	}
	stocktake, err := f.service.CreateStocktake(ctx, f.actorA, global, f.branchShop, "CYCLE_COUNT", []string{basis.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.SetReorderRule(ctx, f.actorA, global, basis.ID, 2, 10, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.SetOperatingModels(ctx, f.actorA, global, f.branchShop2, SetOperatingModelsInput{PrimaryModel: ModelSupermarket, Models: []string{ModelSupermarket}}); err != nil {
		t.Fatal(err)
	}
	listing, err := f.service.CreateListing(ctx, f.actorA, global, f.branchShop2, CreateListingInput{VariantID: basis.VariantRef, OperatingModel: ModelSupermarket, RetailEnabled: boolp(true)})
	if err != nil {
		t.Fatal(err)
	}
	destination, err := f.service.ConfigureStockBasis(ctx, f.actorA, global, f.branchShop2, ConfigureStockBasisInput{ListingID: listing.ID, MeasureID: basis.MeasureRef, ConsumptionMode: ConsumptionExact, AllocationPolicy: AllocationFIFO})
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := f.service.CreateTransfer(ctx, f.actorA, global, CreateTransferInput{SourceBranchID: f.branchShop, DestinationBranchID: f.branchShop2, Lines: []TransferLineInput{{SourceStockBasisID: basis.ID, DestinationStockBasisID: destination.ID, Quantity: 1}}})
	if err != nil {
		t.Fatal(err)
	}

	foreignChecks := []struct {
		name string
		call func() error
	}{
		{"balance", func() error { _, err := f.service.InventoryBalance(ctx, f.actorB, global, basis.ID); return err }},
		{"receipt", func() error { _, err := f.service.GetReceipt(ctx, f.actorB, global, receipt.ID); return err }},
		{"receipt mutation", func() error { _, err := f.service.CancelReceipt(ctx, f.actorB, global, receipt.ID); return err }},
		{"reservation", func() error {
			_, err := f.service.GetInventoryReservation(ctx, f.actorB, global, reservation.ID)
			return err
		}},
		{"reservation mutation", func() error {
			_, err := f.service.ReleaseReservation(ctx, f.actorB, global, reservation.ID)
			return err
		}},
		{"adjustment", func() error {
			_, err := f.service.AdjustInventory(ctx, f.actorB, global, AdjustInventoryInput{StockBasisID: basis.ID, QuantityDelta: 1, ReasonCode: "FOUND_STOCK"})
			return err
		}},
		{"transfer", func() error { _, err := f.service.GetTransfer(ctx, f.actorB, global, transfer.ID); return err }},
		{"stocktake", func() error { _, err := f.service.GetStocktake(ctx, f.actorB, global, stocktake.ID); return err }},
		{"quarantine", func() error {
			_, err := f.service.ResolveQuarantine(ctx, f.actorB, global, quarantine.ID, "RELEASE")
			return err
		}},
		{"reorder", func() error {
			_, err := f.service.SetReorderRule(ctx, f.actorB, global, basis.ID, 1, 3, nil)
			return err
		}},
		{"preorder policy", func() error {
			_, err := f.service.SetInventoryPolicy(ctx, f.actorB, global, basis.ListingRef, SetInventoryPolicyInput{Mode: "NONE"})
			return err
		}},
		{"fulfilment measurement", func() error {
			_, err := f.service.RecordFulfilmentMeasurement(ctx, f.actorB, global, RecordFulfilmentInput{ReservationLineID: reservation.Lines[0].ID, ActualQuantity: 1, ActualMeasureID: basis.MeasureRef, SourceRef: "foreign"})
			return err
		}},
		{"branch list", func() error {
			_, _, err := f.service.ListReceipts(ctx, f.actorB, global, f.branchShop, "", 10, 0)
			return err
		}},
	}
	for _, check := range foreignChecks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); err == nil {
				t.Fatal("foreign inventory object was accessible")
			}
		})
	}
	branchOnly := Scope{BranchIDs: []string{f.branchShop2}}
	if _, err = f.service.InventoryBalance(ctx, f.actorA, branchOnly, basis.ID); err == nil {
		t.Fatal("branch-scoped actor read inventory from another branch")
	}
	if _, err = f.service.AdjustInventory(ctx, f.actorA, global, AdjustInventoryInput{StockBasisID: basis.ID, QuantityDelta: MaxInventoryQuantityPerLine + 1, ReasonCode: "FOUND_STOCK"}); err == nil {
		t.Fatal("unbounded positive stock adjustment was accepted")
	}
}
