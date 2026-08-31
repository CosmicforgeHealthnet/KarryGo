package shop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	inventory "cosmicforge/logistics/services/vendor-service/internal/features/inventory"
)

type inventory14ENoopEvidence struct{}

func (inventory14ENoopEvidence) RecordInventoryEvidence(
	context.Context,
	inventory.InventoryTransaction,
	inventory.Evidence,
) error {
	return nil
}

type inventory14ENoopEvents struct{}

func (inventory14ENoopEvents) PublishInventoryEvent(
	context.Context,
	inventory.InventoryTransaction,
	inventory.Event,
) error {
	return nil
}

type inventory14EFixture struct {
	shop shopFixture

	service *inventory.StockService

	actor inventory.Actor
	scope inventory.BranchScope
}

func inventory14EFixtureForTest(
	t *testing.T,
) inventory14EFixture {
	t.Helper()

	f :=
		section12Fixture(t)

	ctx :=
		context.Background()

	if err :=
		database.ApplyMigrations(
			ctx,
			f.pool,
			"../../../migrations",
		); err != nil {

		t.Fatal(err)
	}

	flags :=
		inventory.Flags{
			Enabled:       true,
			WritesEnabled: true,
		}

	foundation :=
		inventory.NewService(
			inventory.Dependencies{
				Flags: flags,
			},
		)

	service :=
		inventory.NewStockService(
			inventory.StockOptions{
				DB: f.pool,

				Foundation: foundation,

				Transactions: inventory.NewPostgresTransactionBoundary(
					f.pool,
				),

				Evidence: inventory14ENoopEvidence{},

				Events: inventory14ENoopEvents{},
			},
		)

	scope, err :=
		inventory.ExactBranchScope(
			f.actorA.VendorID,
			f.branchShop,
		)

	if err != nil {
		t.Fatal(err)
	}

	return inventory14EFixture{
		shop: f,

		service: service,

		actor: inventory.Actor{
			StaffID: f.actorA.StaffID,

			BusinessID: f.actorA.VendorID,

			CorrelationID: "14e-postgres-race",
		},

		scope: scope,
	}
}

func (
	f inventory14EFixture,
) createItem(
	t *testing.T,
	name string,
	quantity int64,
) string {
	t.Helper()

	basis :=
		createInventoryBasis(
			t,
			f.shop,
			name,
			quantity,
		)

	// Shop receipt state and Generic Inventory state are intentionally separate
	// during the compatibility bridge. Seed the generic authority explicitly.
	_, err :=
		f.service.Receive(
			context.Background(),
			f.actor,
			f.scope,
			inventory.ReceiveInput{
				BranchID: f.shop.branchShop,

				InventoryItemID: basis.ID,

				Quantity: quantity,

				SourceType: "INTERNAL",

				SourceRef: "14e-seed-" +
					uuid.NewString(),

				Lot: inventory.LotMetadata{
					LotNumber: "14e-" +
						uuid.NewString(),
				},
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	return basis.ID
}

func (
	f inventory14EFixture,
) balance(
	t *testing.T,
	itemID string,
) inventory.Balance {
	t.Helper()

	out, err :=
		f.service.Availability(
			context.Background(),
			f.actor,
			f.scope,
			itemID,
		)

	if err != nil {
		t.Fatal(err)
	}

	return out
}

func assert14EInvariant(
	t *testing.T,
	f inventory14EFixture,
	itemID string,
) {
	t.Helper()

	var (
		onHand      int64
		reserved    int64
		quarantined int64
	)

	err :=
		f.shop.pool.QueryRow(
			context.Background(),
			`
            SELECT
                on_hand_quantity,
                reserved_quantity,
                quarantined_quantity
              FROM inventory_balances
             WHERE business_id=$1
               AND inventory_item_id=$2
            `,
			f.actor.BusinessID,
			itemID,
		).Scan(
			&onHand,
			&reserved,
			&quarantined,
		)

	if err != nil {
		t.Fatal(err)
	}

	if onHand < 0 {
		t.Fatalf(
			"on_hand became negative: %d",
			onHand,
		)
	}

	if reserved < 0 {
		t.Fatalf(
			"reserved became negative: %d",
			reserved,
		)
	}

	if quarantined < 0 {
		t.Fatalf(
			"quarantined became negative: %d",
			quarantined,
		)
	}

	if reserved+
		quarantined >
		onHand {

		t.Fatalf(
			"reserved+quarantined=%d exceeds on_hand=%d",
			reserved+quarantined,
			onHand,
		)
	}
}

func reserve14E(
	ctx context.Context,
	f inventory14EFixture,
	itemID string,
	quantity int64,
	source string,
	expiresAt *time.Time,
) (
	inventory.Reservation,
	error,
) {
	return f.service.CreateReservation(
		ctx,
		f.actor,
		f.scope,
		inventory.CreateReservationInput{
			BranchID: f.shop.branchShop,

			SourceType: inventory.ReservationSourceInternal,

			SourceRef: source,

			ExpiresAt: expiresAt,

			Lines: []inventory.ReservationLineInput{
				{
					InventoryItemID: itemID,

					Quantity: quantity,
				},
			},
		},
	)
}

func TestSection14EPostgresSevenPlusSevenRace(
	t *testing.T,
) {
	f :=
		inventory14EFixtureForTest(t)

	itemID :=
		f.createItem(
			t,
			"Generic 14E 7+7",
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

	var wg sync.WaitGroup

	for index := 0; index < 2; index++ {

		wg.Add(1)

		go func(
			index int,
		) {
			defer wg.Done()

			<-start

			_,
				err :=
				reserve14E(
					ctx,
					f,
					itemID,
					7,
					"seven-seven-"+
						string(
							rune(
								'a'+index,
							),
						)+
						"-"+
						uuid.NewString(),
					nil,
				)

			results <- err
		}(index)
	}

	close(start)
	wg.Wait()
	close(results)

	successes := 0

	for err := range results {

		if err == nil {
			successes++
		}
	}

	if successes != 1 {
		t.Fatalf(
			"7+7 successes=%d want=1",
			successes,
		)
	}

	balance :=
		f.balance(
			t,
			itemID,
		)

	if balance.OnHandQuantity != 10 ||
		balance.ReservedQuantity != 7 ||
		balance.AvailableQuantity != 3 {

		t.Fatalf(
			"7+7 balance=%+v",
			balance,
		)
	}

	assert14EInvariant(
		t,
		f,
		itemID,
	)
}

func TestSection14EPostgresMultiLineAtomicity(
	t *testing.T,
) {
	f :=
		inventory14EFixtureForTest(t)

	itemA :=
		f.createItem(
			t,
			"Generic Atomic A",
			2,
		)

	itemB :=
		f.createItem(
			t,
			"Generic Atomic B",
			5,
		)

	itemC :=
		f.createItem(
			t,
			"Generic Atomic C",
			2,
		)

	_,
		err :=
		f.service.CreateReservation(
			context.Background(),
			f.actor,
			f.scope,
			inventory.CreateReservationInput{
				BranchID: f.shop.branchShop,

				SourceType: inventory.ReservationSourceInternal,

				SourceRef: "atomic-" +
					uuid.NewString(),

				Lines: []inventory.ReservationLineInput{
					{
						InventoryItemID: itemA,

						Quantity: 2,
					},
					{
						InventoryItemID: itemB,

						Quantity: 5,
					},
					{
						InventoryItemID: itemC,

						Quantity: 3,
					},
				},
			},
		)

	if !errors.Is(
		err,
		inventory.ErrInsufficientStock,
	) {
		t.Fatalf(
			"atomic reservation error=%v",
			err,
		)
	}

	for _, itemID := range []string{
		itemA,
		itemB,
		itemC,
	} {

		balance :=
			f.balance(
				t,
				itemID,
			)

		if balance.ReservedQuantity != 0 {
			t.Fatalf(
				"partial reservation leaked for %s: %+v",
				itemID,
				balance,
			)
		}

		assert14EInvariant(
			t,
			f,
			itemID,
		)
	}
}

func TestSection14EPostgresIdempotency(
	t *testing.T,
) {
	f :=
		inventory14EFixtureForTest(t)

	itemID :=
		f.createItem(
			t,
			"Generic Idempotency",
			10,
		)

	source :=
		"idem-" +
			uuid.NewString()

	first, err :=
		reserve14E(
			context.Background(),
			f,
			itemID,
			3,
			source,
			nil,
		)

	if err != nil {
		t.Fatal(err)
	}

	second, err :=
		reserve14E(
			context.Background(),
			f,
			itemID,
			3,
			source,
			nil,
		)

	if err != nil {
		t.Fatal(err)
	}

	if first.ID !=
		second.ID {

		t.Fatalf(
			"idempotency replay IDs differ: %s %s",
			first.ID,
			second.ID,
		)
	}

	balance :=
		f.balance(
			t,
			itemID,
		)

	if balance.ReservedQuantity != 3 {
		t.Fatalf(
			"idempotency replay duplicated reservation: %+v",
			balance,
		)
	}

	_,
		err =
		reserve14E(
			context.Background(),
			f,
			itemID,
			4,
			source,
			nil,
		)

	if !errors.Is(
		err,
		inventory.ErrReservationIdempotencyConflict,
	) {
		t.Fatalf(
			"changed fingerprint error=%v",
			err,
		)
	}
}

func TestSection14EPostgresReceiptReservationRace(
	t *testing.T,
) {
	f :=
		inventory14EFixtureForTest(t)

	itemID :=
		f.createItem(
			t,
			"Generic Receipt Race",
			5,
		)

	receipt, err :=
		f.service.CreateReceipt(
			context.Background(),
			f.actor,
			f.scope,
			inventory.CreateReceiptInput{
				BranchID: f.shop.branchShop,

				SourceRef: "receipt-race-" +
					uuid.NewString(),

				ReceivedAt: time.Now().UTC(),

				Lines: []inventory.ReceiptLineInput{
					{
						InventoryItemID: itemID,

						Quantity: 5,

						LotNumber: "race-lot-" +
							uuid.NewString(),
					},
				},
			},
		)

	if err != nil {
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

	go func() {
		<-start

		_,
			err :=
			f.service.PostReceipt(
				ctx,
				f.actor,
				f.scope,
				receipt.ID,
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
				"receipt-reservation-"+
					uuid.NewString(),
				nil,
			)

		results <- err
	}()

	close(start)

	<-results
	<-results

	balance :=
		f.balance(
			t,
			itemID,
		)

	if balance.OnHandQuantity != 10 {
		t.Fatalf(
			"receipt/reservation on_hand=%d want=10",
			balance.OnHandQuantity,
		)
	}

	if balance.ReservedQuantity != 0 &&
		balance.ReservedQuantity != 7 {

		t.Fatalf(
			"receipt/reservation reserved=%d",
			balance.ReservedQuantity,
		)
	}

	assert14EInvariant(
		t,
		f,
		itemID,
	)
}

func TestSection14EPostgresCommitReleaseRace(
	t *testing.T,
) {
	f :=
		inventory14EFixtureForTest(t)

	itemID :=
		f.createItem(
			t,
			"Generic Commit Release",
			10,
		)

	reservation, err :=
		reserve14E(
			context.Background(),
			f,
			itemID,
			7,
			"commit-release-"+
				uuid.NewString(),
			nil,
		)

	if err != nil {
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

	go func() {
		<-start

		_,
			err :=
			f.service.CommitReservation(
				ctx,
				f.actor,
				f.scope,
				reservation.ID,
			)

		results <- err
	}()

	go func() {
		<-start

		_,
			err :=
			f.service.ReleaseReservation(
				ctx,
				f.actor,
				f.scope,
				reservation.ID,
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
			"commit/release successes=%d want=1",
			successes,
		)
	}

	balance :=
		f.balance(
			t,
			itemID,
		)

	if balance.ReservedQuantity != 0 {
		t.Fatalf(
			"commit/release left reserved stock: %+v",
			balance,
		)
	}

	if balance.OnHandQuantity != 3 &&
		balance.OnHandQuantity != 10 {

		t.Fatalf(
			"commit/release corrupted on_hand: %+v",
			balance,
		)
	}

	assert14EInvariant(
		t,
		f,
		itemID,
	)
}

func TestSection14EPostgresExpiryCommitRace(
	t *testing.T,
) {
	f :=
		inventory14EFixtureForTest(t)

	itemID :=
		f.createItem(
			t,
			"Generic Expiry Commit",
			10,
		)

	expires :=
		time.Now().
			UTC().
			Add(
				120 *
					time.Millisecond,
			)

	reservation, err :=
		reserve14E(
			context.Background(),
			f,
			itemID,
			7,
			"expiry-commit-"+
				uuid.NewString(),
			&expires,
		)

	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(
		170 *
			time.Millisecond,
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
			f.service.ExpireDueReservations(
				ctx,
				100,
			)

		results <- err
	}()

	go func() {
		<-start

		_,
			err :=
			f.service.CommitReservation(
				ctx,
				f.actor,
				f.scope,
				reservation.ID,
			)

		results <- err
	}()

	close(start)

	for index := 0; index < 2; index++ {

		err :=
			<-results

		if err != nil &&
			!errors.Is(
				err,
				inventory.ErrReservationExpired,
			) &&
			!errors.Is(
				err,
				inventory.ErrReservationStateConflict,
			) {

			t.Fatalf(
				"expiry/commit unexpected error=%v",
				err,
			)
		}
	}

	current, err :=
		f.service.GetReservation(
			ctx,
			f.actor,
			f.scope,
			reservation.ID,
		)

	if err != nil {
		t.Fatal(err)
	}

	if current.Status !=
		inventory.ReservationExpired {

		t.Fatalf(
			"expiry/commit final status=%s want EXPIRED",
			current.Status,
		)
	}

	balance :=
		f.balance(
			t,
			itemID,
		)

	if balance.OnHandQuantity != 10 ||
		balance.ReservedQuantity != 0 {

		t.Fatalf(
			"expiry/commit balance=%+v",
			balance,
		)
	}

	assert14EInvariant(
		t,
		f,
		itemID,
	)
}

func TestSection14EPostgresQuarantineReservationRace(
	t *testing.T,
) {
	f :=
		inventory14EFixtureForTest(t)

	itemID :=
		f.createItem(
			t,
			"Generic Quarantine Race",
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
			f.service.Quarantine(
				ctx,
				f.actor,
				f.scope,
				inventory.QuantityMutationInput{
					BranchID: f.shop.branchShop,

					InventoryItemID: itemID,

					Quantity: 7,

					SourceType: "INTERNAL",

					SourceRef: "quarantine-race",
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
				"quarantine-reservation-"+
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

	balance :=
		f.balance(
			t,
			itemID,
		)

	if balance.ReservedQuantity+
		balance.QuarantinedQuantity !=
		7 {

		t.Fatalf(
			"quarantine/reservation balance=%+v",
			balance,
		)
	}

	assert14EInvariant(
		t,
		f,
		itemID,
	)
}

func TestSection14EPostgresAdjustmentReservationRace(
	t *testing.T,
) {
	f :=
		inventory14EFixtureForTest(t)

	itemID :=
		f.createItem(
			t,
			"Generic Adjustment Race",
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
			f.service.Adjust(
				ctx,
				f.actor,
				f.scope,
				inventory.QuantityMutationInput{
					BranchID: f.shop.branchShop,

					InventoryItemID: itemID,

					SourceType: "INTERNAL",

					SourceRef: "adjustment-race",

					ReasonCode: "CONTROLLED_ADJUSTMENT",
				},
				-7,
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
				"adjustment-reservation-"+
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
			"adjustment/reservation successes=%d want=1",
			successes,
		)
	}

	assert14EInvariant(
		t,
		f,
		itemID,
	)
}
