package shop

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func createInventoryBasis(t *testing.T, f shopFixture, name string, quantity int64, expiries ...time.Time) InventoryStockBasis {
	t.Helper()
	ctx := context.Background()
	scope := Scope{Global: true}
	_, _ = f.service.SetOperatingModels(ctx, f.actorA, scope, f.branchShop, SetOperatingModelsInput{PrimaryModel: ModelSupermarket, Models: []string{ModelSupermarket}})
	category, err := f.service.CreateCategory(ctx, f.actorA, scope, CreateCategoryInput{Name: "Inventory " + name + " " + uuid.NewString()[:8]})
	if err != nil {
		t.Fatal(err)
	}
	product, err := f.service.CreateProduct(ctx, f.actorA, scope, CreateProductInput{CategoryID: category.ID, Name: name, ProductClass: "PACKAGED", OperatingModels: []string{ModelSupermarket}})
	if err != nil {
		t.Fatal(err)
	}
	variant, err := f.service.CreateVariant(ctx, f.actorA, scope, product.ID, CreateVariantInput{Name: name + " each", PackageLevel: "EACH", RetailPriceMinor: int64p(1000)})
	if err != nil {
		t.Fatal(err)
	}
	unit, err := f.service.CreateSellUnit(ctx, f.actorA, scope, product.ID, CreateSellUnitInput{VariantID: variant.ID, MeasureID: "smd_piece", MeasurementMode: ModeFixed, PriceMinor: int64p(1000), RetailEnabled: boolp(true)})
	if err != nil {
		t.Fatal(err)
	}
	listing, err := f.service.CreateListing(ctx, f.actorA, scope, f.branchShop, CreateListingInput{VariantID: variant.ID, OperatingModel: ModelSupermarket, RetailEnabled: boolp(true), SellUnitIDs: []string{unit.ID}})
	if err != nil {
		t.Fatal(err)
	}
	basis, err := f.service.ConfigureStockBasis(ctx, f.actorA, scope, f.branchShop, ConfigureStockBasisInput{ListingID: listing.ID, MeasureID: "smd_piece", ConsumptionMode: ConsumptionSameUnit, AllocationPolicy: AllocationFEFO, LotTracking: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(expiries) == 0 {
		expiries = []time.Time{time.Now().UTC().Add(30 * 24 * time.Hour)}
	}
	perLot := quantity / int64(len(expiries))
	lines := make([]ReceiptLineInput, 0, len(expiries))
	remaining := quantity
	for i, expiry := range expiries {
		lineQuantity := perLot
		if i == len(expiries)-1 {
			lineQuantity = remaining
		}
		remaining -= lineQuantity
		lines = append(lines, ReceiptLineInput{StockBasisID: basis.ID, Quantity: lineQuantity, LotNumber: "lot-" + uuid.NewString()[:8], ExpiresAt: &expiry})
	}
	receipt, err := f.service.CreateReceipt(ctx, f.actorA, scope, CreateReceiptInput{BranchID: f.branchShop, SourceRef: "seed-" + uuid.NewString(), ReceivedAt: time.Now().UTC(), Lines: lines})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.PostReceipt(ctx, f.actorA, scope, receipt.ID); err != nil {
		t.Fatal(err)
	}
	return basis
}

func TestInventoryReservationConcurrencyHardGate(t *testing.T) {
	f := section12Fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scope := Scope{Global: true}
	basis := createInventoryBasis(t, f, "Concurrency Stock", 10)

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "7x7-" + itoa(index) + "-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: 7}}})
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("concurrent 7+7 reservations succeeded=%d want=1", succeeded)
	}
	balance, err := f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
	if err != nil {
		t.Fatal(err)
	}
	if balance.OnHandQuantity != 10 || balance.ReservedQuantity != 7 || balance.AvailableQuantity != 3 {
		t.Fatalf("oversell gate balance=%+v", balance)
	}
}

func TestInventoryMultiLineAtomicityAndFEFOLocking(t *testing.T) {
	f := section12Fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scope := Scope{Global: true}
	early := time.Now().UTC().Add(24 * time.Hour)
	late := time.Now().UTC().Add(72 * time.Hour)
	first := createInventoryBasis(t, f, "FEFO Stock", 10, late, early)
	second := createInventoryBasis(t, f, "Atomic Stock", 2)

	if _, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "atomic-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: first.ID, Quantity: 4}, {StockBasisID: second.ID, Quantity: 3}}}); err == nil {
		t.Fatal("multi-line reservation with an insufficient line succeeded")
	}
	balance, err := f.service.InventoryBalance(ctx, f.actorA, scope, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if balance.ReservedQuantity != 0 {
		t.Fatalf("failed multi-line reservation leaked quantity: %+v", balance)
	}
	reservation, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "fefo-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: first.ID, Quantity: 5}}})
	if err != nil {
		t.Fatal(err)
	}
	var allocatedExpiry time.Time
	err = f.pool.QueryRow(ctx, `SELECT l.expires_at FROM shop_inventory_reservation_allocations a
		JOIN shop_inventory_reservation_lines rl ON rl.id=a.reservation_line_id
		JOIN shop_inventory_lots l ON l.id=a.lot_id
		WHERE rl.reservation_id=$1 ORDER BY l.expires_at LIMIT 1`, reservation.ID).Scan(&allocatedExpiry)
	if err != nil {
		t.Fatal(err)
	}
	if delta := allocatedExpiry.Sub(early); delta < -time.Millisecond || delta > time.Millisecond {
		t.Fatalf("FEFO allocated expiry=%s want=%s", allocatedExpiry, early)
	}
}

func TestInventoryReservationBoundaryRaces(t *testing.T) {
	tests := []struct {
		name            string
		first, second   int64
		wantSuccesses   int
		wantReserved    int64
		maxReservedOnly bool
	}{
		{name: "exact 4 plus 6", first: 4, second: 6, wantSuccesses: 2, wantReserved: 10},
		{name: "boundary 10 plus 1", first: 10, second: 1, wantSuccesses: 1, wantReserved: 10, maxReservedOnly: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := section12Fixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			scope := Scope{Global: true}
			basis := createInventoryBasis(t, f, "Boundary "+test.name, 10)
			start := make(chan struct{})
			results := make(chan error, 2)
			var wg sync.WaitGroup
			for index, quantity := range []int64{test.first, test.second} {
				wg.Add(1)
				go func(index int, quantity int64) {
					defer wg.Done()
					<-start
					_, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "boundary-" + itoa(index) + "-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: quantity}}})
					results <- err
				}(index, quantity)
			}
			close(start)
			wg.Wait()
			close(results)
			succeeded := 0
			for err := range results {
				if err == nil {
					succeeded++
				}
			}
			if succeeded != test.wantSuccesses {
				t.Fatalf("successes=%d want=%d", succeeded, test.wantSuccesses)
			}
			balance, err := f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
			if err != nil {
				t.Fatal(err)
			}
			if test.maxReservedOnly {
				if balance.ReservedQuantity <= 0 || balance.ReservedQuantity > test.wantReserved {
					t.Fatalf("boundary race oversold or lost reservation: %+v", balance)
				}
			} else if balance.ReservedQuantity != test.wantReserved {
				t.Fatalf("exact-fit reservations=%+v", balance)
			}
		})
	}
}

func TestInventoryCommitReleaseRace(t *testing.T) {
	f := section12Fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	scope := Scope{Global: true}
	basis := createInventoryBasis(t, f, "Commit Release Race", 10)
	reservation, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "commit-release-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: 7}}})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, transitionErr := f.service.CommitReservation(ctx, f.actorA, scope, reservation.ID)
		results <- transitionErr
	}()
	go func() {
		<-start
		_, transitionErr := f.service.ReleaseReservation(ctx, f.actorA, scope, reservation.ID)
		results <- transitionErr
	}()
	close(start)
	succeeded := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("commit/release race successes=%d want=1", succeeded)
	}
	balance, err := f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
	if err != nil {
		t.Fatal(err)
	}
	if balance.ReservedQuantity != 0 || (balance.OnHandQuantity != 3 && balance.OnHandQuantity != 10) {
		t.Fatalf("commit/release race corrupted balance: %+v", balance)
	}
	if err = f.service.ReconcileInventoryBalance(ctx, f.actorA.VendorID, basis.ID); err != nil {
		t.Fatal(err)
	}
}

func TestInventoryQuarantineAndTransferReservationRaces(t *testing.T) {
	t.Run("quarantine versus reservation", func(t *testing.T) {
		f := section12Fixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		scope := Scope{Global: true}
		basis := createInventoryBasis(t, f, "Quarantine Race", 10)
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			_, err := f.service.QuarantineInventory(ctx, f.actorA, scope, basis.ID, 7, "QUALITY_CHECK", "")
			results <- err
		}()
		go func() {
			<-start
			_, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "quarantine-race-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: 7}}})
			results <- err
		}()
		close(start)
		succeeded := 0
		for i := 0; i < 2; i++ {
			if <-results == nil {
				succeeded++
			}
		}
		if succeeded != 1 {
			t.Fatalf("quarantine/reservation successes=%d want=1", succeeded)
		}
		balance, _ := f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
		if balance.ReservedQuantity+balance.QuarantinedQuantity != 7 || balance.AvailableQuantity != 3 {
			t.Fatalf("quarantine/reservation balance=%+v", balance)
		}
	})

	t.Run("transfer versus reservation", func(t *testing.T) {
		f := section12Fixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		scope := Scope{Global: true}
		source := createInventoryBasis(t, f, "Transfer Race", 10)
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
		transfer, err := f.service.CreateTransfer(ctx, f.actorA, scope, CreateTransferInput{SourceBranchID: f.branchShop, DestinationBranchID: f.branchShop2, Lines: []TransferLineInput{{SourceStockBasisID: source.ID, DestinationStockBasisID: destination.ID, Quantity: 7}}})
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			_, err := f.service.DispatchTransfer(ctx, f.actorA, scope, transfer.ID)
			results <- err
		}()
		go func() {
			<-start
			_, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "transfer-race-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: source.ID, Quantity: 7}}})
			results <- err
		}()
		close(start)
		succeeded := 0
		for i := 0; i < 2; i++ {
			if <-results == nil {
				succeeded++
			}
		}
		if succeeded != 1 {
			t.Fatalf("transfer/reservation successes=%d want=1", succeeded)
		}
		balance, _ := f.service.InventoryBalance(ctx, f.actorA, scope, source.ID)
		if balance.OnHandQuantity-balance.ReservedQuantity != 3 {
			t.Fatalf("transfer/reservation balance=%+v", balance)
		}
	})
}

func TestInventoryExpiryReserveAndStocktakeRaces(t *testing.T) {
	t.Run("expiry versus replacement reservation", func(t *testing.T) {
		f := section12Fixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		scope := Scope{Global: true}
		basis := createInventoryBasis(t, f, "Expiry Reserve Race", 10)
		expires := time.Now().UTC().Add(100 * time.Millisecond)
		if _, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "expiring-race-" + uuid.NewString(), ExpiresAt: &expires, Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: 10}}}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(140 * time.Millisecond)
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() { <-start; _, err := f.service.ReconcileExpiredReservations(ctx, 500); results <- err }()
		go func() {
			<-start
			_, err := f.service.ReserveInventory(ctx, f.actorA, scope, ReserveInventoryInput{SourceType: "INTERNAL_TEST", SourceRef: "replacement-race-" + uuid.NewString(), Lines: []ReserveInventoryLineInput{{StockBasisID: basis.ID, Quantity: 10}}})
			results <- err
		}()
		close(start)
		for i := 0; i < 2; i++ {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		balance, err := f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
		if err != nil || balance.OnHandQuantity != 10 || balance.ReservedQuantity != 10 || balance.AvailableQuantity != 0 {
			t.Fatalf("expiry/reserve race balance=%+v err=%v", balance, err)
		}
	})

	t.Run("stocktake post versus movement", func(t *testing.T) {
		f := section12Fixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		scope := Scope{Global: true}
		basis := createInventoryBasis(t, f, "Stocktake Race", 10)
		stocktake, err := f.service.CreateStocktake(ctx, f.actorA, scope, f.branchShop, "CYCLE_COUNT", []string{basis.ID})
		if err != nil {
			t.Fatal(err)
		}
		if err = f.service.CountStocktakeLine(ctx, f.actorA, scope, stocktake.ID, basis.ID, 8); err != nil {
			t.Fatal(err)
		}
		balance, err := f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() { <-start; _, err := f.service.PostStocktake(ctx, f.actorA, scope, stocktake.ID); results <- err }()
		go func() {
			<-start
			_, err := f.service.AdjustInventory(ctx, f.actorA, scope, AdjustInventoryInput{StockBasisID: basis.ID, QuantityDelta: 1, ReasonCode: "FOUND_STOCK", ExpectedRevision: balance.Revision})
			results <- err
		}()
		close(start)
		succeeded := 0
		for i := 0; i < 2; i++ {
			if <-results == nil {
				succeeded++
			}
		}
		if succeeded != 1 {
			t.Fatalf("stocktake/movement race successes=%d want=1", succeeded)
		}
		balance, err = f.service.InventoryBalance(ctx, f.actorA, scope, basis.ID)
		if err != nil || (balance.OnHandQuantity != 8 && balance.OnHandQuantity != 11) {
			t.Fatalf("stocktake/movement race balance=%+v err=%v", balance, err)
		}
		if err = f.service.ReconcileInventoryBalance(ctx, f.actorA.VendorID, basis.ID); err != nil {
			t.Fatal(err)
		}
	})
}
