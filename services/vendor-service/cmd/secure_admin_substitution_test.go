package main

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/admincommands"
	"cosmicforge/logistics/services/vendor-service/internal/features/adminresources"
	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminowner"
	"errors"
	"github.com/jackc/pgx/v5"
	"sync"
	"testing"
	"time"
)

// This double tests the source consent/command transaction. The real Inventory
// transaction and adapter are independently exercised below against stock/lots.
type transactionalReplacementFixture struct{ *replacementInventory }

func (f *transactionalReplacementFixture) ReplaceReservationAllocationInTx(ctx context.Context, tx pgx.Tx, business, branch string, r orders.InventoryReplacementRequest) (orders.InventoryReplacementResult, error) {
	_, err := tx.Exec(ctx, `INSERT INTO section11_test_inventory_effects(operation_key)VALUES($1)ON CONFLICT DO NOTHING`, r.IdempotencyKey)
	return orders.InventoryReplacementResult{OriginalFulfilled: r.OriginalFoundQty, OriginalReleased: r.MissingQty, ReplacementReserved: r.ReplacementQty, Version: 2}, err
}
func TestVendorSecureAdminProcessesOnlyCustomerApprovedSubstitutionAtomically(t *testing.T) {
	s := newSubstitutionEnv(t)
	ctx := context.Background()
	e := vendorAdminFixtureFor(t, s.fixture)
	e.svc.Executor = admincommands.Executor{Orders: s.fixture.service}
	if _, err := s.fixture.env.pool.Exec(ctx, `CREATE TABLE section11_test_inventory_effects(operation_key text PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.fixture.env.pool.Exec(context.Background(), `DROP TABLE section11_test_inventory_effects`) })
	adapter := &transactionalReplacementFixture{s.inventory}
	s.fixture.service.SetFulfilmentAuthorities(adapter, s.missions)
	proposal := s.propose(t, s.candidate(t, s.unitPrice))
	e.enroll(t, "substitutions", proposal.ID)
	pending := e.cmd("PROCESS_APPROVED_SUBSTITUTION", "substitutions", proposal.ID, "", proposal.Version, map[string]string{})
	if _, err := e.svc.Execute(ctx, e.actor, "case-1", e.scope, pending); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("HQ processed a pending customer decision", err)
	}
	approved, err := s.fixture.service.RespondToSubstitution(ctx, s.fixture.customerID, proposal.ID, true, proposal.Version)
	if err != nil {
		t.Fatal(err)
	}
	holds, refunds := s.wallet.callCount(), len(s.wallet.refundedFor())
	command := e.cmd("PROCESS_APPROVED_SUBSTITUTION", "substitutions", proposal.ID, "", approved.Version, map[string]string{})
	// A missing immutable source receipt rolls the processing marker and the
	// joined Inventory transaction back together.
	e.f.env.exec(ctx, `CREATE FUNCTION section11_reject_substitution_receipt() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'test receipt unavailable';END$$;CREATE TRIGGER section11_reject_substitution_receipt BEFORE INSERT ON admin_owner_command_decisions FOR EACH ROW EXECUTE FUNCTION section11_reject_substitution_receipt()`)
	if _, err = e.svc.Execute(ctx, e.actor, "case-1", e.scope, command); err == nil {
		t.Fatal("missing audit accepted")
	}
	e.f.env.exec(ctx, `DROP TRIGGER section11_reject_substitution_receipt ON admin_owner_command_decisions;DROP FUNCTION section11_reject_substitution_receipt()`)
	var count int
	if err = e.f.env.pool.QueryRow(ctx, `SELECT count(*) FROM section11_test_inventory_effects`).Scan(&count); err != nil || count != 0 {
		t.Fatal("stock effect escaped failed receipt", count, err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.Execute(ctx, e.actor, "case-1", e.scope, command)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var processing string
	var version int64
	if err = e.f.env.pool.QueryRow(ctx, `SELECT admin_inventory_processing_state,version FROM commerce_substitution_proposals WHERE id=$1`, proposal.ID).Scan(&processing, &version); err != nil || processing != "APPLIED" || version != approved.Version+1 {
		t.Fatal("missing processing evidence", processing, version, err)
	}
	if err = e.f.env.pool.QueryRow(ctx, `SELECT count(*) FROM section11_test_inventory_effects`).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate preparation", count, err)
	}
	if s.wallet.callCount() != holds || len(s.wallet.refundedFor()) != refunds {
		t.Fatal("inventory processing changed money")
	}
	stale := e.cmd("PROCESS_APPROVED_SUBSTITUTION", "substitutions", proposal.ID, "", approved.Version, map[string]string{})
	if _, err = e.svc.Execute(ctx, e.actor, "case-1", e.scope, stale); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatal("stale decision accepted", err)
	}
}
func TestVendorSecureAdminReplacementAdapterJoinsRealStockTransaction(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)
	original := env.seedShopItem(s17bStockOptions{Business: env.businessA, Staff: env.staffA, Branch: env.branchA1, Name: "Admin original stock", Quantity: 10, PriceMinor: 100})
	replacement := env.seedShopItem(s17bStockOptions{Business: env.businessA, Staff: env.staffA, Branch: env.branchA1, Name: "Admin replacement stock", Quantity: 4, PriceMinor: 100})
	ctx := env.ctx()
	actor := inventory.Actor{BusinessID: env.businessA, StaffID: env.staffA, ActorRef: env.staffA}
	scope := mustExactBranchScope(env.businessA, env.branchA1)
	expires := time.Now().UTC().Add(time.Hour)
	reservation, err := env.stock.CreateReservation(ctx, actor, scope, inventory.CreateReservationInput{BranchID: env.branchA1, SourceType: inventory.ReservationSourceCheckout, SourceRef: "admin-substitution-" + env.suffix, ExpiresAt: &expires, Lines: []inventory.ReservationLineInput{{InventoryItemID: original.InventoryItemID, Quantity: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	request := orders.InventoryReplacementRequest{ReservationRef: reservation.ID, SourceType: inventory.ReservationSourceCheckout, SourceRef: reservation.SourceRef, OriginalItemRef: original.InventoryItemID, OriginalOrderedQty: 10, OriginalFoundQty: 6, MissingQty: 4, ReplacementItemRef: replacement.InventoryItemID, ReplacementQty: 4, ExpectedVersion: reservation.Version, IdempotencyKey: "substitution-inventory-admin-test", CorrelationID: "admin-test"}
	adapter := newSection17FInventoryAuthority(env.stock)
	tx, err := env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.ReplaceReservationAllocationInTx(ctx, tx, env.businessA, env.branchA1, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.OriginalReleased != 4 || result.ReplacementReserved != 4 {
		t.Fatal(result)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if env.availableStock(env.businessA, env.branchA1, original.InventoryItemID) != 0 || env.availableStock(env.businessA, env.branchA1, replacement.InventoryItemID) != 4 {
		t.Fatal("replacement escaped transaction rollback")
	}
	tx, err = env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.ReplaceReservationAllocationInTx(ctx, tx, env.businessA, env.branchA1, request); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if env.availableStock(env.businessA, env.branchA1, original.InventoryItemID) != 4 || env.availableStock(env.businessA, env.branchA1, replacement.InventoryItemID) != 0 {
		t.Fatal("native stock replacement not applied")
	}
	tx, err = env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = adapter.ReplaceReservationAllocationInTx(ctx, tx, env.businessA, env.branchA1, request); err != nil {
		t.Fatal("original-action replay", err)
	}
}

func TestVendorSecureAdminCheaperSubstitutionReadPreservesSignedPriceDifference(t *testing.T) {
	s := newSubstitutionEnv(t)
	proposal := s.propose(t, s.candidate(t, s.unitPrice-100))
	if proposal.PriceDifferenceMinor != -100 {
		t.Fatal("cheaper native proposal required", proposal.PriceDifferenceMinor)
	}
	tx, err := s.fixture.env.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	r, err := adminresources.Source().Read(context.Background(), tx, "substitutions", proposal.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = adminowner.ValidateResource(r, adminowner.VendorProfile().Definitions[r.Kind], true); err != nil {
		t.Fatal(err)
	}
	direction := ""
	var magnitude *int64
	for _, d := range r.Dimensions {
		if d.Name == "price_difference_direction" && d.Value != nil {
			direction = *d.Value
		}
	}
	for _, c := range r.Counters {
		if c.Name == "price_difference_magnitude_minor" {
			magnitude = c.Value
		}
	}
	if direction != "DECREASE" || magnitude == nil || *magnitude != 100 {
		t.Fatal("signed owner price difference lost", direction, magnitude)
	}
}
