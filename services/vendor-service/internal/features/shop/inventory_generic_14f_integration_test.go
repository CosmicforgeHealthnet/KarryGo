package shop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	inventory "cosmicforge/logistics/services/vendor-service/internal/features/inventory"
)

func transferScope14F(
	t *testing.T,
	f inventory14EFixture,
) inventory.BranchScope {
	t.Helper()

	scope, err :=
		inventory.BusinessScope(
			f.actor.BusinessID,
		)

	if err != nil {
		t.Fatal(err)
	}

	return scope
}

func createDestinationItem14F(
	t *testing.T,
	f inventory14EFixture,
	sourceItemID string,
) string {
	t.Helper()

	ctx :=
		context.Background()

	var (
		variantID string
		measureID string
	)

	err :=
		f.shop.pool.QueryRow(
			ctx,
			`
            SELECT
                variant_id,
                measure_definition_id
              FROM shop_inventory_stock_bases
             WHERE id=$1
               AND business_id=$2
            `,
			sourceItemID,
			f.actor.BusinessID,
		).Scan(
			&variantID,
			&measureID,
		)

	if err != nil {
		t.Fatal(err)
	}

	// A SAME_UNIT stock basis requires an enabled sell unit on its own listing
	// that consumes the stock measure exactly. Sell units are variant-level, so
	// the destination reuses the source variant's unit and enables it on the
	// destination listing.
	var sellUnitID string

	if err = f.shop.pool.QueryRow(
		ctx,
		`
        SELECT id
          FROM shop_sell_units
         WHERE business_id=$1
           AND variant_id=$2
           AND measure_definition_id=$3
           AND status='ACTIVE'
         LIMIT 1
        `,
		f.actor.BusinessID,
		variantID,
		measureID,
	).Scan(&sellUnitID); err != nil {
		t.Fatal(err)
	}

	scope :=
		Scope{
			Global: true,
		}

	_, err =
		f.shop.service.SetOperatingModels(
			ctx,
			f.shop.actorA,
			scope,
			f.shop.branchShop2,
			SetOperatingModelsInput{
				PrimaryModel: ModelSupermarket,

				Models: []string{
					ModelSupermarket,
				},
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	listing, err :=
		f.shop.service.CreateListing(
			ctx,
			f.shop.actorA,
			scope,
			f.shop.branchShop2,
			CreateListingInput{
				VariantID: variantID,

				OperatingModel: ModelSupermarket,

				RetailEnabled: boolp(true),

				SellUnitIDs: []string{
					sellUnitID,
				},
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	basis, err :=
		f.shop.service.ConfigureStockBasis(
			ctx,
			f.shop.actorA,
			scope,
			f.shop.branchShop2,
			ConfigureStockBasisInput{
				ListingID: listing.ID,

				MeasureID: measureID,

				// A transfer moves the same physical stock between branches, so
				// the destination must share the source basis mode. The source
				// fixture is SAME_UNIT; EXACT_CONVERSION here would describe a
				// different quantity semantic and generic Inventory correctly
				// refuses to move stock across that boundary.
				ConsumptionMode: ConsumptionSameUnit,

				AllocationPolicy: AllocationFIFO,
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	return basis.ID
}

func createTransfer14F(
	t *testing.T,
	f inventory14EFixture,
	sourceItem string,
	destinationItem string,
	quantity int64,
) inventory.Transfer {
	t.Helper()

	out, err :=
		f.service.CreateTransfer(
			context.Background(),
			f.actor,
			transferScope14F(
				t,
				f,
			),
			inventory.CreateTransferInput{
				SourceBranchID: f.shop.branchShop,

				DestinationBranchID: f.shop.branchShop2,

				Lines: []inventory.TransferLineInput{
					{
						SourceInventoryItemID: sourceItem,

						DestinationInventoryItemID: destinationItem,

						Quantity: quantity,
					},
				},
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	return out
}

func TestSection14FPostgresStocktakeMovementRace(
	t *testing.T,
) {
	f :=
		inventory14EFixtureForTest(t)

	itemID :=
		f.createItem(
			t,
			"14F Stocktake Race",
			10,
		)

	stocktake, err :=
		f.service.CreateStocktake(
			context.Background(),
			f.actor,
			f.scope,
			inventory.CreateStocktakeInput{
				BranchID: f.shop.branchShop,

				Type: inventory.StocktakeCycle,

				InventoryItemIDs: []string{
					itemID,
				},
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	if err :=
		f.service.CountStocktakeLine(
			context.Background(),
			f.actor,
			f.scope,
			stocktake.ID,
			itemID,
			8,
		); err != nil {

		t.Fatal(err)
	}

	ctx,
		cancel :=
		context.WithTimeout(
			context.Background(),
			30*time.Second,
		)

	defer cancel()

	start :=
		make(
			chan struct{},
		)

	results :=
		make(
			chan error,
			2,
		)

	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()
		<-start

		_,
			err :=
			f.service.PostStocktake(
				ctx,
				f.actor,
				f.scope,
				stocktake.ID,
			)

		results <- err
	}()

	go func() {
		defer wg.Done()
		<-start

		_,
			err :=
			f.service.Adjust(
				ctx,
				f.actor,
				f.scope,
				inventory.QuantityMutationInput{
					BranchID: f.shop.branchShop,

					InventoryItemID: itemID,

					SourceType: "INTERNAL",

					SourceRef: "14f-stocktake-race",

					ReasonCode: "CONTROLLED_CORRECTION",
				},
				1,
			)

		results <- err
	}()

	close(start)
	wg.Wait()
	close(results)

	var (
		postErr   error
		adjustErr error
	)

	index := 0

	for result := range results {

		if index == 0 {
			postErr = result
		} else {
			adjustErr = result
		}

		index++
	}

	_ = postErr
	_ = adjustErr

	balance :=
		f.balance(
			t,
			itemID,
		)

	if balance.OnHandQuantity != 9 &&
		balance.OnHandQuantity != 11 {

		t.Fatalf(
			"stocktake/movement race overwrote real movement: %+v",
			balance,
		)
	}

	assert14EInvariant(
		t,
		f,
		itemID,
	)
}

func TestSection14FPostgresTransferDispatchReservationRace(
	t *testing.T,
) {
	f :=
		inventory14EFixtureForTest(t)

	source :=
		f.createItem(
			t,
			"14F Transfer Race",
			10,
		)

	destination :=
		createDestinationItem14F(
			t,
			f,
			source,
		)

	transfer :=
		createTransfer14F(
			t,
			f,
			source,
			destination,
			7,
		)

	ctx,
		cancel :=
		context.WithTimeout(
			context.Background(),
			30*time.Second,
		)

	defer cancel()

	start :=
		make(
			chan struct{},
		)

	results :=
		make(
			chan error,
			2,
		)

	go func() {
		<-start

		_,
			err :=
			f.service.DispatchTransfer(
				ctx,
				f.actor,
				transferScope14F(
					t,
					f,
				),
				transfer.ID,
			)

		results <- err
	}()

	go func() {
		<-start

		_,
			err :=
			reserve14E(
				ctx,
				f,
				source,
				7,
				"14f-transfer-reservation-"+
					uuid.NewString(),
				nil,
			)

		results <- err
	}()

	close(start)

	successes := 0

	for index := 0; index < 2; index++ {

		if <-results == nil {
			successes++
		}
	}

	if successes != 1 {
		t.Fatalf(
			"transfer/reservation successes=%d want=1",
			successes,
		)
	}

	assert14EInvariant(
		t,
		f,
		source,
	)
}

func TestSection14FPostgresTransferReceiveIdempotency(
	t *testing.T,
) {
	f :=
		inventory14EFixtureForTest(t)

	source :=
		f.createItem(
			t,
			"14F Transfer Idempotency",
			10,
		)

	destination :=
		createDestinationItem14F(
			t,
			f,
			source,
		)

	transfer :=
		createTransfer14F(
			t,
			f,
			source,
			destination,
			7,
		)

	scope :=
		transferScope14F(
			t,
			f,
		)

	dispatched, err :=
		f.service.DispatchTransfer(
			context.Background(),
			f.actor,
			scope,
			transfer.ID,
		)

	if err != nil {
		t.Fatal(err)
	}

	sourceRef :=
		"14f-transfer-receipt-" +
			uuid.NewString()

	input :=
		inventory.ReceiveTransferInput{
			SourceRef: sourceRef,

			Lines: []inventory.ReceiveTransferLineInput{
				{
					LineID: dispatched.Lines[0].ID,

					ReceivedQuantity: 7,
				},
			},
		}

	first, err :=
		f.service.ReceiveTransfer(
			context.Background(),
			f.actor,
			scope,
			transfer.ID,
			input,
		)

	if err != nil {
		t.Fatal(err)
	}

	second, err :=
		f.service.ReceiveTransfer(
			context.Background(),
			f.actor,
			scope,
			transfer.ID,
			input,
		)

	if err != nil {
		t.Fatal(err)
	}

	if first.Status !=
		inventory.TransferReceived ||
		second.Status !=
			inventory.TransferReceived {

		t.Fatalf(
			"idempotent receive status first=%s second=%s",
			first.Status,
			second.Status,
		)
	}

	destinationBalance, err :=
		f.service.Availability(
			context.Background(),
			f.actor,
			func() inventory.BranchScope {
				scope, scopeErr :=
					inventory.ExactBranchScope(
						f.actor.BusinessID,
						f.shop.branchShop2,
					)

				if scopeErr != nil {
					t.Fatal(scopeErr)
				}

				return scope
			}(),
			destination,
		)

	if err != nil {
		t.Fatal(err)
	}

	if destinationBalance.OnHandQuantity != 7 {
		t.Fatalf(
			"idempotent receive duplicated stock: %+v",
			destinationBalance,
		)
	}

	changed :=
		input

	changed.Lines =
		[]inventory.ReceiveTransferLineInput{
			{
				LineID: dispatched.Lines[0].ID,

				DamagedQuantity: 7,
			},
		}

	_,
		err =
		f.service.ReceiveTransfer(
			context.Background(),
			f.actor,
			scope,
			transfer.ID,
			changed,
		)

	if !errors.Is(
		err,
		inventory.ErrTransferIdempotencyConflict,
	) {
		t.Fatalf(
			"changed receive fingerprint error=%v",
			err,
		)
	}
}

func TestSection14FPostgresQuarantineReservationRace(
	t *testing.T,
) {
	f :=
		inventory14EFixtureForTest(t)

	itemID :=
		f.createItem(
			t,
			"14F Quarantine Race",
			10,
		)

	ctx,
		cancel :=
		context.WithTimeout(
			context.Background(),
			30*time.Second,
		)

	defer cancel()

	start :=
		make(
			chan struct{},
		)

	results :=
		make(
			chan error,
			2,
		)

	go func() {
		<-start

		_,
			err :=
			f.service.CreateQuarantine(
				ctx,
				f.actor,
				f.scope,
				inventory.CreateQuarantineInput{
					BranchID: f.shop.branchShop,

					InventoryItemID: itemID,

					Quantity: 7,

					ReasonCode: "QUALITY_CHECK",
				},
			)

		results <- err
	}()

	go func() {
		<-start

		_,
			err :=
			reserve14E(
				ctx,
				f,
				itemID,
				7,
				"14f-quarantine-reservation-"+
					uuid.NewString(),
				nil,
			)

		results <- err
	}()

	close(start)

	successes := 0

	for index := 0; index < 2; index++ {

		if <-results == nil {
			successes++
		}
	}

	if successes != 1 {
		t.Fatalf(
			"quarantine/reservation successes=%d want=1",
			successes,
		)
	}

	assert14EInvariant(
		t,
		f,
		itemID,
	)
}

func TestSection14FPostgresAlertDedupeAndReorderThreshold(
	t *testing.T,
) {
	f :=
		inventory14EFixtureForTest(t)

	itemID :=
		f.createItem(
			t,
			"14F Reorder",
			3,
		)

	_,
		err :=
		f.service.SetReorderRule(
			context.Background(),
			f.actor,
			f.scope,
			inventory.SetReorderRuleInput{
				BranchID: f.shop.branchShop,

				InventoryItemID: itemID,

				MinimumQuantity: 5,

				TargetQuantity: 10,

				Status: inventory.ReorderActive,
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	if _,
		err =
		f.service.GenerateInventoryAlerts(
			context.Background(),
			500,
		); err != nil {

		t.Fatal(err)
	}

	if _,
		err =
		f.service.GenerateInventoryAlerts(
			context.Background(),
			500,
		); err != nil {

		t.Fatal(err)
	}

	var count int

	err =
		f.shop.pool.QueryRow(
			context.Background(),
			`
            SELECT count(*)
              FROM inventory_alerts
             WHERE business_id=$1
               AND branch_id=$2
               AND inventory_item_id=$3
               AND alert_type='LOW_STOCK'
            `,
			f.actor.BusinessID,
			f.shop.branchShop,
			itemID,
		).Scan(
			&count,
		)

	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Fatalf(
			"LOW_STOCK alert rows=%d want=1",
			count,
		)
	}

	alerts, err :=
		f.service.ListAlerts(
			context.Background(),
			f.actor,
			f.scope,
			f.shop.branchShop,
			inventory.AlertOpen,
			100,
		)

	if err != nil {
		t.Fatal(err)
	}

	found :=
		false

	for _, alert := range alerts {

		if alert.Type ==
			inventory.AlertLowStock &&
			alert.InventoryItemID ==
				itemID {

			found = true
			break
		}
	}

	if !found {
		t.Fatal(
			"reorder threshold did not produce LOW_STOCK",
		)
	}
}

func TestSection14FPostgresExpiryHandling(
	t *testing.T,
) {
	f :=
		inventory14EFixtureForTest(t)

	basis :=
		createInventoryBasis(
			t,
			f.shop,
			"14F Expiry",
			1,
		)

	expires :=
		time.Now().
			UTC().
			Add(
				150 *
					time.Millisecond,
			)

	_,
		err :=
		f.service.Receive(
			context.Background(),
			f.actor,
			f.scope,
			inventory.ReceiveInput{
				BranchID: f.shop.branchShop,

				InventoryItemID: basis.ID,

				Quantity: 4,

				SourceType: "INTERNAL",

				SourceRef: "14f-expiry-seed-" +
					uuid.NewString(),

				ReasonCode: "EXPIRY_TEST",

				Lot: inventory.LotMetadata{
					LotNumber: "14f-expiry-" +
						uuid.NewString(),

					ExpiresAt: &expires,
				},
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(
		220 *
			time.Millisecond,
	)

	processed, err :=
		f.service.ExpireDueLots(
			context.Background(),
			500,
		)

	if err != nil {
		t.Fatal(err)
	}

	if processed < 1 {
		t.Fatal(
			"expired lot was not processed",
		)
	}

	balance :=
		f.balance(
			t,
			basis.ID,
		)

	if balance.OnHandQuantity != 0 {
		t.Fatalf(
			"expired stock remained on hand: %+v",
			balance,
		)
	}

	var movements int

	err =
		f.shop.pool.QueryRow(
			context.Background(),
			`
            SELECT count(*)
              FROM inventory_movements
             WHERE business_id=$1
               AND inventory_item_id=$2
               AND movement_type='EXPIRY'
            `,
			f.actor.BusinessID,
			basis.ID,
		).Scan(
			&movements,
		)

	if err != nil {
		t.Fatal(err)
	}

	if movements < 1 {
		t.Fatal(
			"expiry did not create immutable EXPIRY movement",
		)
	}
}
