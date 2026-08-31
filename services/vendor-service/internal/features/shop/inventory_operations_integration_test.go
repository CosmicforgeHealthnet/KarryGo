package shop

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestInventoryReservationLifecycleExpiryAndLedger(t *testing.T) {
	f := section12Fixture(t)
	ctx := context.Background()
	scope := Scope{Global: true}
	basis := createInventoryBasis(t, f, "Lifecycle", 10)
	reservation, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "lifecycle-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: 4}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ConfirmReservation(ctx, f.actorA, scope, reservation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.CommitReservation(ctx, f.actorA, scope, reservation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.CommitReservation(ctx, f.actorA, scope, reservation.ID); err != nil {
		t.Fatalf("idempotent commit: %v", err)
	}
	balance, err := f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
	if err != nil {
		t.Fatal(err)
	}
	if balance.OnHandQuantity != 6 || balance.ReservedQuantity != 0 || balance.AvailableQuantity != 6 {
		t.Fatalf("committed balance=%+v", balance)
	}
	released, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "release-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ReleaseReservation(ctx, f.actorA, scope, released.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.service.ReconcileInventoryBalance(ctx, f.actorA.VendorID, basis.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE shop_inventory_movements SET quantity=999
		WHERE id=(SELECT id FROM shop_inventory_movements WHERE business_id=$1 LIMIT 1)`, f.actorA.VendorID); err == nil {
		t.Fatal("append-only inventory movement was updated")
	}
	expires := time.Now().UTC().Add(80 * time.Millisecond)
	expiring, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "expiry-" + uuid.NewString(), ExpiresAt: &expires, Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: 6}}})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(120 * time.Millisecond)
	availability, err := f.service.EffectiveAvailability(ctx, f.actorA, scope, f.branchShop, basis.VariantRef)
	if err != nil {
		t.Fatal(err)
	}
	if availability.PhysicalAvailable != 6 {
		t.Fatalf("expired hold still consumed effective stock: %+v reservation=%s", availability, expiring.ID)
	}
	if _, err = f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "after-expiry-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: 6}}}); err != nil {
		t.Fatalf("expired stock was not reusable: %v", err)
	}
}

func TestBackorderQuarantineAdjustmentAndStocktake(t *testing.T) {
	f := section12Fixture(t)
	ctx := context.Background()
	scope := Scope{Global: true}
	basis := createInventoryBasis(t, f, "Policy Stock", 5)
	if _, err := f.service.SetInventoryPolicy(ctx, f.actorA, scope, basis.ListingRef, SetInventoryPolicyInput{Mode: "BACKORDER", MaximumOutstandingQuantity: int64p(10), MaximumPerRequest: int64p(8)}); err != nil {
		t.Fatal(err)
	}
	reservation, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "backorder-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: 8}}})
	if err != nil {
		t.Fatal(err)
	}
	if reservation.Lines[0].PhysicalQuantity != 5 || reservation.Lines[0].BackorderQuantity != 3 {
		t.Fatalf("backorder split=%+v", reservation.Lines[0])
	}
	balance, err := f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
	if err != nil {
		t.Fatal(err)
	}
	if balance.OnHandQuantity != 5 || balance.ReservedQuantity != 5 || balance.BackorderQuantity != 3 {
		t.Fatalf("negative/incorrect backorder balance=%+v", balance)
	}
	receipt, err := f.service.CreateReceipt(ctx, f.actorA, scope, CreateReceiptInput{BranchID: f.branchShop, SourceRef: "restock-" + uuid.NewString(), Lines: []ReceiptLineInput{{StockBasisID: basis.ID, Quantity: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.PostReceipt(ctx, f.actorA, scope, receipt.ID); err != nil {
		t.Fatal(err)
	}
	reservation, err = f.service.GetInventoryReservation(ctx, f.actorA, scope, reservation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reservation.Lines[0].BackorderQuantity != 0 || reservation.Lines[0].PhysicalQuantity != 8 {
		t.Fatalf("receipt did not allocate FIFO backorder: %+v", reservation.Lines[0])
	}
	if _, err = f.service.ReleaseReservation(ctx, f.actorA, scope, reservation.ID); err != nil {
		t.Fatal(err)
	}
	balance, _ = f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
	quarantine, err := f.service.QuarantineInventory(ctx, f.actorA, scope, basis.ID, 4, "QUALITY_CHECK", "")
	if err != nil {
		t.Fatal(err)
	}
	afterQuarantine, _ := f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
	if afterQuarantine.AvailableQuantity != balance.AvailableQuantity-4 {
		t.Fatalf("quarantine did not remove sellability: before=%+v after=%+v", balance, afterQuarantine)
	}
	if _, err = f.service.ResolveQuarantine(ctx, f.actorA, scope, quarantine.ID, "RELEASE"); err != nil {
		t.Fatal(err)
	}
	balance, _ = f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
	if _, err = f.service.AdjustInventory(ctx, f.actorA, scope, AdjustInventoryInput{StockBasisID: basis.ID, QuantityDelta: -2, ReasonCode: "WASTE", ExpectedRevision: balance.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.AdjustInventory(ctx, f.actorA, scope, AdjustInventoryInput{StockBasisID: basis.ID, QuantityDelta: 1, ReasonCode: "FOUND_STOCK", ExpectedRevision: balance.Revision}); err == nil {
		t.Fatal("stale adjustment revision accepted")
	}
	stocktake, err := f.service.CreateStocktake(ctx, f.actorA, scope, f.branchShop, "CYCLE_COUNT", []string{basis.ID})
	if err != nil {
		t.Fatal(err)
	}
	balance, _ = f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
	if err = f.service.CountStocktakeLine(ctx, f.actorA, scope, stocktake.ID, basis.ID, balance.OnHandQuantity); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.AdjustInventory(ctx, f.actorA, scope, AdjustInventoryInput{StockBasisID: basis.ID, QuantityDelta: 1, ReasonCode: "FOUND_STOCK", ExpectedRevision: balance.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.PostStocktake(ctx, f.actorA, scope, stocktake.ID); err == nil {
		t.Fatal("stale stocktake overwrote later movement")
	}
}

func TestTransferAndVariableFulfilment(t *testing.T) {
	f := section12Fixture(t)
	ctx := context.Background()
	scope := Scope{Global: true}
	source := createInventoryBasis(t, f, "Transfer Stock", 10)
	if _, err := f.service.SetOperatingModels(ctx, f.actorA, scope, f.branchShop2, SetOperatingModelsInput{PrimaryModel: ModelSupermarket, Models: []string{ModelSupermarket}}); err != nil {
		t.Fatal(err)
	}
	listing, err := f.service.CreateListing(ctx, f.actorA, scope, f.branchShop2, CreateListingInput{VariantID: source.VariantRef, OperatingModel: ModelSupermarket, RetailEnabled: boolp(true)})
	if err != nil {
		t.Fatal(err)
	}
	destination, err := f.service.ConfigureStockBasis(ctx, f.actorA, scope, f.branchShop2, ConfigureStockBasisInput{ListingID: listing.ID, MeasureID: source.MeasureRef, ConsumptionMode: ConsumptionExact, AllocationPolicy: AllocationFIFO})
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := f.service.CreateTransfer(ctx, f.actorA, scope, CreateTransferInput{SourceBranchID: f.branchShop, DestinationBranchID: f.branchShop2, Lines: []TransferLineInput{{SourceStockBasisID: source.ID, DestinationStockBasisID: destination.ID, Quantity: 6}}})
	if err != nil {
		t.Fatal(err)
	}
	transfer, err = f.service.DispatchTransfer(ctx, f.actorA, scope, transfer.ID)
	if err != nil {
		t.Fatal(err)
	}
	sourceBalance, _ := f.service.InventoryBalance(ctx, f.actorA, scope, source.ID)
	if sourceBalance.OnHandQuantity != 4 || sourceBalance.InTransitQuantity != 6 {
		t.Fatalf("dispatch double-counted stock: %+v", sourceBalance)
	}
	transfer, err = f.service.ReceiveTransfer(ctx, f.actorA, scope, transfer.ID, []ReceiveTransferLineInput{{LineID: transfer.Lines[0].ID, ReceivedQuantity: 4}})
	if err != nil {
		t.Fatal(err)
	}
	destinationBalance, _ := f.service.InventoryBalance(ctx, f.actorA, scope, destination.ID)
	sourceBalance, _ = f.service.InventoryBalance(ctx, f.actorA, scope, source.ID)
	if transfer.Status != "PARTIALLY_RECEIVED" || destinationBalance.OnHandQuantity != 4 || sourceBalance.InTransitQuantity != 2 {
		t.Fatalf("partial transfer receipt source=%+v destination=%+v transfer=%+v", sourceBalance, destinationBalance, transfer)
	}
	transfer, err = f.service.ReceiveTransfer(ctx, f.actorA, scope, transfer.ID, []ReceiveTransferLineInput{{LineID: transfer.Lines[0].ID, DamagedQuantity: 1, MissingQuantity: 1}})
	if err != nil {
		t.Fatal(err)
	}
	sourceBalance, _ = f.service.InventoryBalance(ctx, f.actorA, scope, source.ID)
	if transfer.Status != "RECEIVED" || sourceBalance.InTransitQuantity != 0 {
		t.Fatalf("transfer receipt balances source=%+v destination=%+v", sourceBalance, destinationBalance)
	}
	variable := createInventoryBasis(t, f, "Variable Count", 10)
	variable, err = f.service.ConfigureStockBasis(ctx, f.actorA, scope, f.branchShop, ConfigureStockBasisInput{ListingID: variable.ListingRef, MeasureID: variable.MeasureRef, ConsumptionMode: ConsumptionVariable, AllocationPolicy: AllocationFIFO})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE shop_sell_units SET underfill_tolerance_bps=1000,overfill_tolerance_bps=1000,actual_measurement_required=true WHERE business_id=$1 AND variant_id=$2`, f.actorA.VendorID, variable.VariantRef); err != nil {
		t.Fatal(err)
	}
	reservation, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "variable-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: variable.ID, Quantity: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	measurement, err := f.service.RecordFulfilmentMeasurement(ctx, f.actorA, scope, RecordFulfilmentInput{ReservationLineID: reservation.Lines[0].ID, ActualQuantity: 10, ActualMeasureID: variable.MeasureRef, SourceRef: "measure-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if measurement.ToleranceResult != "WITHIN_TOLERANCE" {
		t.Fatalf("measurement=%+v", measurement)
	}
	if _, err = f.service.CommitReservation(ctx, f.actorA, scope, reservation.ID); err != nil {
		t.Fatal(err)
	}
}

func TestReceiptAndTransferCancellationEvidence(t *testing.T) {
	f := section12Fixture(t)
	ctx := context.Background()
	scope := Scope{Global: true}
	basis := createInventoryBasis(t, f, "Receipt Reversal", 5)

	draft, err := f.service.CreateReceipt(ctx, f.actorA, scope, CreateReceiptInput{BranchID: f.branchShop, SourceRef: "draft-cancel-" + uuid.NewString(), Lines: []ReceiptLineInput{{StockBasisID: basis.ID, Quantity: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	draft, err = f.service.CancelReceipt(ctx, f.actorA, scope, draft.ID)
	if err != nil || draft.Status != "CANCELLED" {
		t.Fatalf("draft receipt cancellation=%+v err=%v", draft, err)
	}

	posted, err := f.service.CreateReceipt(ctx, f.actorA, scope, CreateReceiptInput{BranchID: f.branchShop, SourceRef: "posted-cancel-" + uuid.NewString(), Lines: []ReceiptLineInput{{StockBasisID: basis.ID, Quantity: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.PostReceipt(ctx, f.actorA, scope, posted.ID); err != nil {
		t.Fatal(err)
	}
	before, _ := f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
	posted, err = f.service.CancelReceipt(ctx, f.actorA, scope, posted.ID)
	if err != nil || posted.Status != "CANCELLED" {
		t.Fatalf("posted receipt reversal=%+v err=%v", posted, err)
	}
	after, _ := f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
	if after.OnHandQuantity != before.OnHandQuantity-3 {
		t.Fatalf("receipt reversal balance before=%+v after=%+v", before, after)
	}
	var reversals int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM shop_inventory_movements WHERE business_id=$1 AND source_type='RECEIPT' AND source_ref=$2 AND reason_code='RECEIPT_REVERSAL'`, f.actorA.VendorID, posted.ID).Scan(&reversals); err != nil || reversals != 1 {
		t.Fatalf("receipt reversal evidence count=%d err=%v", reversals, err)
	}

	blocked, err := f.service.CreateReceipt(ctx, f.actorA, scope, CreateReceiptInput{BranchID: f.branchShop, SourceRef: "blocked-cancel-" + uuid.NewString(), Lines: []ReceiptLineInput{{StockBasisID: basis.ID, Quantity: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.PostReceipt(ctx, f.actorA, scope, blocked.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "block-reversal-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: 6}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.CancelReceipt(ctx, f.actorA, scope, blocked.ID); err == nil {
		t.Fatal("receipt cancellation removed stock already participating in a reservation")
	}

	if _, err = f.service.SetOperatingModels(ctx, f.actorA, scope, f.branchShop2, SetOperatingModelsInput{PrimaryModel: ModelSupermarket, Models: []string{ModelSupermarket}}); err != nil {
		t.Fatal(err)
	}
	listing, err := f.service.CreateListing(ctx, f.actorA, scope, f.branchShop2, CreateListingInput{VariantID: basis.VariantRef, OperatingModel: ModelSupermarket, RetailEnabled: boolp(true)})
	if err != nil {
		t.Fatal(err)
	}
	destination, err := f.service.ConfigureStockBasis(ctx, f.actorA, scope, f.branchShop2, ConfigureStockBasisInput{ListingID: listing.ID, MeasureID: basis.MeasureRef, ConsumptionMode: ConsumptionExact, AllocationPolicy: AllocationFIFO})
	if err != nil {
		t.Fatal(err)
	}
	transfer, err := f.service.CreateTransfer(ctx, f.actorA, scope, CreateTransferInput{SourceBranchID: f.branchShop, DestinationBranchID: f.branchShop2, Lines: []TransferLineInput{{SourceStockBasisID: basis.ID, DestinationStockBasisID: destination.ID, Quantity: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	transfer, err = f.service.CancelTransfer(ctx, f.actorA, scope, transfer.ID)
	if err != nil || transfer.Status != "CANCELLED" || transfer.Lines[0].Status != "CANCELLED" {
		t.Fatalf("draft transfer cancellation=%+v err=%v", transfer, err)
	}
}

func TestInventoryAlertWorkerCoversOperationalConditions(t *testing.T) {
	f := section12Fixture(t)
	ctx := context.Background()
	scope := Scope{Global: true}
	basis := createInventoryBasis(t, f, "Alert Conditions", 5)
	stocktake, err := f.service.CreateStocktake(ctx, f.actorA, scope, f.branchShop, "CYCLE_COUNT", []string{basis.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.CountStocktakeLine(ctx, f.actorA, scope, stocktake.ID, basis.ID, 4); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.PostStocktake(ctx, f.actorA, scope, stocktake.ID); err != nil {
		t.Fatal(err)
	}
	hours := 8760
	if _, err = f.service.SetReorderRule(ctx, f.actorA, scope, basis.ID, 3, 10, &hours); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.QuarantineInventory(ctx, f.actorA, scope, basis.ID, 1, "QUALITY_CHECK", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.SetInventoryPolicy(ctx, f.actorA, scope, basis.ListingRef, SetInventoryPolicyInput{Mode: "BACKORDER", MaximumOutstandingQuantity: int64p(10), MaximumPerRequest: int64p(8)}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "alert-backorder-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: 6}}}); err != nil {
		t.Fatal(err)
	}
	generated, err := f.service.GenerateInventoryAlerts(ctx, 100)
	if err != nil || generated == 0 {
		t.Fatalf("generated alerts=%d err=%v", generated, err)
	}
	rows, err := f.pool.Query(ctx, `SELECT alert_type FROM shop_inventory_alerts WHERE business_id=$1 AND stock_basis_id=$2 AND status='OPEN'`, f.actorA.VendorID, basis.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	types := map[string]bool{}
	for rows.Next() {
		var alertType string
		if err = rows.Scan(&alertType); err != nil {
			t.Fatal(err)
		}
		types[alertType] = true
	}
	for _, want := range []string{"OUT_OF_STOCK", "EXPIRING_SOON", "QUARANTINED", "BACKORDER_OUTSTANDING", "RESERVATION_CONFLICT", "STOCKTAKE_VARIANCE"} {
		if !types[want] {
			t.Fatalf("missing %s alert from %v", want, types)
		}
	}
	alerts, more, err := f.service.ListInventoryAlerts(ctx, f.actorA, scope, f.branchShop, "OPEN", "", 100, 0)
	if err != nil || more || len(alerts) < 6 {
		t.Fatalf("alert read family count=%d more=%v err=%v", len(alerts), more, err)
	}
	acknowledged, err := f.service.TransitionInventoryAlert(ctx, f.actorA, scope, alerts[0].ID, "ACKNOWLEDGED")
	if err != nil || acknowledged.Status != "ACKNOWLEDGED" {
		t.Fatalf("acknowledged alert=%+v err=%v", acknowledged, err)
	}
	resolved, err := f.service.TransitionInventoryAlert(ctx, f.actorA, scope, alerts[0].ID, "RESOLVED")
	if err != nil || resolved.Status != "RESOLVED" {
		t.Fatalf("resolved alert=%+v err=%v", resolved, err)
	}
}

func TestExpiredLotSynchronousGateAndReconciliation(t *testing.T) {
	f := section12Fixture(t)
	ctx := context.Background()
	scope := Scope{Global: true}
	basis := createInventoryBasis(t, f, "Lot Expiry", 5)
	if _, err := f.pool.Exec(ctx, `UPDATE shop_inventory_lots SET expires_at=now()+interval '100 milliseconds' WHERE business_id=$1 AND stock_basis_id=$2`, f.actorA.VendorID, basis.ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(140 * time.Millisecond)
	availability, err := f.service.EffectiveAvailability(ctx, f.actorA, scope, f.branchShop, basis.VariantRef)
	if err != nil || availability.PhysicalAvailable != 0 {
		t.Fatalf("expired lot remained synchronously sellable: %+v err=%v", availability, err)
	}
	if _, err = f.service.ReconcileExpiredLots(ctx, 500); err != nil {
		t.Fatal(err)
	}
	balance, err := f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
	if err != nil || balance.OnHandQuantity != 0 {
		t.Fatalf("expiry reconciliation balance=%+v err=%v", balance, err)
	}
	var expiryMovements int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM shop_inventory_movements WHERE business_id=$1 AND stock_basis_id=$2 AND movement_type='EXPIRY'`, f.actorA.VendorID, basis.ID).Scan(&expiryMovements); err != nil || expiryMovements != 1 {
		t.Fatalf("expiry movement count=%d err=%v", expiryMovements, err)
	}

	reservedBasis := createInventoryBasis(t, f, "Reserved Lot Expiry", 4)
	if _, err = f.pool.Exec(ctx, `UPDATE shop_inventory_lots SET expires_at=now()+interval '100 milliseconds' WHERE business_id=$1 AND stock_basis_id=$2`, f.actorA.VendorID, reservedBasis.ID); err != nil {
		t.Fatal(err)
	}
	reservation, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "reserved-expiry-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: reservedBasis.ID, Quantity: 4}}})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(140 * time.Millisecond)
	if _, err = f.service.CommitReservation(ctx, f.actorA, scope, reservation.ID); err == nil {
		t.Fatal("reservation committed expired allocated stock")
	}
	if _, err = f.service.ReconcileExpiredLots(ctx, 500); err != nil {
		t.Fatal(err)
	}
	balance, _ = f.service.InventoryBalance(ctx, f.actorA, scope, reservedBasis.ID)
	if balance.OnHandQuantity != 4 || balance.ReservedQuantity != 4 {
		t.Fatalf("expiry worker invalidated an active reservation: %+v", balance)
	}
	if _, err = f.service.ReleaseReservation(ctx, f.actorA, scope, reservation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ReconcileExpiredLots(ctx, 500); err != nil {
		t.Fatal(err)
	}
	balance, _ = f.service.InventoryBalance(ctx, f.actorA, scope, reservedBasis.ID)
	if balance.OnHandQuantity != 0 || balance.ReservedQuantity != 0 {
		t.Fatalf("released expired lot was not reconciled: %+v", balance)
	}
}
