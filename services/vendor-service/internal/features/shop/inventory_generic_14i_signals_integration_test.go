package shop

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	inventory "cosmicforge/logistics/services/vendor-service/internal/features/inventory"
)

// Section 14I search/analytics invalidation certification.
//
// Section 14G's coverage of this is a static check that a migration file
// exists plus a constant-equality assertion. Neither establishes the property
// that actually matters to a Search consumer: that a stock mutation emits
// invalidation evidence when availability *changes*, and stays quiet when it
// does not. A signal on every write would make cache invalidation useless; no
// signal at all would leave Search serving stock that is gone.

// capturingEvents records every published Inventory event so a test can ask
// what a downstream consumer would actually have received.
type capturingEvents14I struct {
	mu     sync.Mutex
	events []inventory.Event
}

func (c *capturingEvents14I) PublishInventoryEvent(
	_ context.Context,
	_ inventory.InventoryTransaction,
	event inventory.Event,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
	return nil
}

// typed returns the recorded events of one type, newest last.
func (c *capturingEvents14I) typed(eventType string) []inventory.Event {
	c.mu.Lock()
	defer c.mu.Unlock()

	var out []inventory.Event
	for _, event := range c.events {
		if event.Type == eventType {
			out = append(out, event)
		}
	}
	return out
}

func (c *capturingEvents14I) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = nil
}

// signalFixture14I builds the same stock service the 14E fixture does, but with
// a capturing event sink in place of the discarding one.
type signalFixture14I struct {
	shop    shopFixture
	service *inventory.StockService
	actor   inventory.Actor
	scope   inventory.BranchScope
	events  *capturingEvents14I
}

func signalFixtureForTest14I(t *testing.T) signalFixture14I {
	t.Helper()

	f := section12Fixture(t)
	ctx := context.Background()

	if err := database.ApplyMigrations(ctx, f.pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}

	flags := inventory.Flags{Enabled: true, WritesEnabled: true}
	events := &capturingEvents14I{}

	service := inventory.NewStockService(inventory.StockOptions{
		DB:           f.pool,
		Foundation:   inventory.NewService(inventory.Dependencies{Flags: flags}),
		Transactions: inventory.NewPostgresTransactionBoundary(f.pool),
		Evidence:     inventory14ENoopEvidence{},
		Events:       events,
	})

	scope, err := inventory.ExactBranchScope(f.actorA.VendorID, f.branchShop)
	if err != nil {
		t.Fatal(err)
	}

	return signalFixture14I{
		shop:    f,
		service: service,
		actor: inventory.Actor{
			StaffID:       f.actorA.StaffID,
			BusinessID:    f.actorA.VendorID,
			CorrelationID: "14i-signals",
		},
		scope:  scope,
		events: events,
	}
}

// TestSection14IAvailabilitySignalsFireOnlyOnTransition is the whole gate: a
// Search consumer must be told when an item becomes sellable or stops being
// sellable, and must not be woken up by writes that changed neither.
func TestSection14IAvailabilitySignalsFireOnlyOnTransition(t *testing.T) {
	f := signalFixtureForTest14I(t)
	ctx := context.Background()

	// The Shop receipt seeds Shop stock only. Generic Inventory keeps its own
	// balance across the compatibility bridge and still starts at zero, so the
	// first generic receipt below is genuinely the nothing-to-something
	// transition. The quantity here only has to be a valid Shop receipt.
	basis := createInventoryBasis(t, f.shop, "14I Signals", 1)

	// First receipt: nothing to something. This is a real transition and must
	// produce exactly one availability signal.
	f.events.reset()

	if _, err := f.service.Receive(ctx, f.actor, f.scope, inventory.ReceiveInput{
		BranchID:        f.shop.branchShop,
		InventoryItemID: basis.ID,
		Quantity:        10,
		SourceType:      "INTERNAL",
		SourceRef:       "14i-signal-" + uuid.NewString(),
		Lot:             inventory.LotMetadata{LotNumber: "14i-" + uuid.NewString()},
	}); err != nil {
		t.Fatal(err)
	}

	available := f.events.typed(inventory.EventInventoryItemAvailable)
	if len(available) != 1 {
		t.Fatalf("becoming available emitted %d signals; want exactly 1", len(available))
	}
	if available[0].BusinessID != f.actor.BusinessID {
		t.Errorf("signal business = %q, want %q", available[0].BusinessID, f.actor.BusinessID)
	}
	if available[0].BranchID == nil || *available[0].BranchID != f.shop.branchShop {
		t.Error("availability signal is not bound to the branch that changed")
	}

	// Second receipt: something to more of something. Availability did not
	// change, so a Search consumer must not be invalidated again.
	f.events.reset()

	if _, err := f.service.Receive(ctx, f.actor, f.scope, inventory.ReceiveInput{
		BranchID:        f.shop.branchShop,
		InventoryItemID: basis.ID,
		Quantity:        5,
		SourceType:      "INTERNAL",
		SourceRef:       "14i-signal-" + uuid.NewString(),
		Lot:             inventory.LotMetadata{LotNumber: "14i-" + uuid.NewString()},
	}); err != nil {
		t.Fatal(err)
	}

	if repeated := f.events.typed(inventory.EventInventoryItemAvailable); len(repeated) != 0 {
		t.Fatalf("a receipt that did not change availability emitted %d redundant signals",
			len(repeated))
	}

	// Now take every unit away. Something to nothing is a transition in the
	// other direction and must produce exactly one out-of-stock signal.
	f.events.reset()

	if _, err := f.service.Adjust(ctx, f.actor, f.scope, inventory.QuantityMutationInput{
		BranchID:        f.shop.branchShop,
		InventoryItemID: basis.ID,
		SourceType:      "INTERNAL",
		SourceRef:       "14i-signal-out-" + uuid.NewString(),
		ReasonCode:      "CONTROLLED_CORRECTION",
	}, -15); err != nil {
		t.Fatal(err)
	}

	outOfStock := f.events.typed(inventory.EventInventoryItemOutOfStock)
	if len(outOfStock) != 1 {
		t.Fatalf("going out of stock emitted %d signals; want exactly 1", len(outOfStock))
	}

	// A second mutation that leaves the item still out of stock must stay quiet.
	f.events.reset()

	if _, err := f.service.Adjust(ctx, f.actor, f.scope, inventory.QuantityMutationInput{
		BranchID:        f.shop.branchShop,
		InventoryItemID: basis.ID,
		SourceType:      "INTERNAL",
		SourceRef:       "14i-signal-out2-" + uuid.NewString(),
		ReasonCode:      "CONTROLLED_CORRECTION",
	}, 0); err != nil {
		// A zero adjustment may be rejected as invalid, which is itself fine:
		// the point is only that no duplicate signal is produced.
		t.Logf("zero adjustment refused (acceptable): %v", err)
	}

	if repeated := f.events.typed(inventory.EventInventoryItemOutOfStock); len(repeated) != 0 {
		t.Fatalf("a mutation that left the item out of stock emitted %d redundant signals",
			len(repeated))
	}

	// Restocking transitions back and must signal once more.
	f.events.reset()

	if _, err := f.service.Receive(ctx, f.actor, f.scope, inventory.ReceiveInput{
		BranchID:        f.shop.branchShop,
		InventoryItemID: basis.ID,
		Quantity:        7,
		SourceType:      "INTERNAL",
		SourceRef:       "14i-signal-back-" + uuid.NewString(),
		Lot:             inventory.LotMetadata{LotNumber: "14i-" + uuid.NewString()},
	}); err != nil {
		t.Fatal(err)
	}

	if back := f.events.typed(inventory.EventInventoryItemAvailable); len(back) != 1 {
		t.Fatalf("restocking emitted %d availability signals; want exactly 1", len(back))
	}
}
