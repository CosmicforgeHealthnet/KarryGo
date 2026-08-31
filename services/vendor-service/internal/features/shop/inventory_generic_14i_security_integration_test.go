package shop

import (
	"context"
	"testing"

	"github.com/google/uuid"

	inventory "cosmicforge/logistics/services/vendor-service/internal/features/inventory"
)

// Section 14I tenant, branch and nested-identifier certification for the
// generic Inventory authority against real PostgreSQL.
//
// Section 14H already proves the internal HMAC surface refuses a foreign
// business, but it does so against an in-memory authority. The gates below run
// the real service against the real database, which is where a missing
// business_id predicate in one SQL statement would actually show up.
//
// Every probe uses the shape a real attacker has: a fully valid actor for
// business B naming an identifier that belongs to business A. Nothing is
// forged; the identifiers are simply not theirs.

// foreignActor14I returns an actor and business-wide scope for the second
// seeded business, plus that business's own branch, so a cross-tenant probe is
// a legitimate caller rather than a malformed one.
func foreignActor14I(
	t *testing.T,
	f inventory14EFixture,
) (inventory.Actor, inventory.BranchScope, string) {
	t.Helper()

	var branchB string

	if err := f.shop.pool.QueryRow(
		context.Background(),
		`SELECT id FROM vendor_branches WHERE business_id=$1 LIMIT 1`,
		f.shop.actorB.VendorID,
	).Scan(&branchB); err != nil {
		t.Fatal(err)
	}

	// The foreign caller must be a fully valid actor. An actor missing its
	// staff identity is rejected as malformed, which would make every probe
	// below pass without proving anything about tenant isolation.
	actor := inventory.Actor{
		StaffID:       f.shop.actorB.StaffID,
		BusinessID:    f.shop.actorB.VendorID,
		CorrelationID: "14i-cross-tenant",
	}

	scope, err := inventory.BusinessScope(f.shop.actorB.VendorID)
	if err != nil {
		t.Fatal(err)
	}

	return actor, scope, branchB
}

// assertDenied14I requires a refusal. Generic Inventory deliberately makes a
// foreign resource indistinguishable from a missing one, so the gate is that
// the caller was refused and learned nothing, not that a particular sentinel
// came back.
func assertDenied14I(t *testing.T, gate string, err error) {
	t.Helper()

	if err == nil {
		t.Fatalf("%s: a foreign caller reached inventory that is not theirs", gate)
	}
}

// TestSection14ITenantIsolationAcrossGenericInventory walks the generic read
// and write surface with a foreign but otherwise valid caller.
func TestSection14ITenantIsolationAcrossGenericInventory(t *testing.T) {
	f := inventory14EFixtureForTest(t)
	ctx := context.Background()

	itemID := f.createItem(t, "14I Tenant Isolation", 10)

	foreignActorValue, foreignScope, foreignBranch := foreignActor14I(t, f)

	t.Run("balances of another business are unreachable", func(t *testing.T) {
		balances, err := f.service.ListBalances(
			ctx, foreignActorValue, foreignScope, f.shop.branchShop, 50, 0)

		if err == nil && len(balances) > 0 {
			t.Fatal("a foreign business read another tenant's balances")
		}
	})

	t.Run("item availability of another business is unreachable", func(t *testing.T) {
		_, err := f.service.Availability(ctx, foreignActorValue, foreignScope, itemID)
		assertDenied14I(t, "availability", err)
	})

	t.Run("movements of another business are unreachable", func(t *testing.T) {
		movements, err := f.service.ListMovements(
			ctx, foreignActorValue, foreignScope, f.shop.branchShop,
			inventory.MovementListOptions{Limit: 50})

		if err == nil && len(movements) > 0 {
			t.Fatal("a foreign business read another tenant's movement ledger")
		}
	})

	t.Run("reserving another business's item is refused", func(t *testing.T) {
		_, err := f.service.CreateReservation(ctx, foreignActorValue, foreignScope,
			inventory.CreateReservationInput{
				BranchID:   f.shop.branchShop,
				SourceType: "ORDER",
				SourceRef:  "14i-cross-tenant-" + uuid.NewString(),
				Lines: []inventory.ReservationLineInput{
					{InventoryItemID: itemID, Quantity: 1},
				},
			})

		assertDenied14I(t, "reserve", err)
	})

	t.Run("quarantining another business's item is refused", func(t *testing.T) {
		_, err := f.service.CreateQuarantine(ctx, foreignActorValue, foreignScope,
			inventory.CreateQuarantineInput{
				BranchID:        f.shop.branchShop,
				InventoryItemID: itemID,
				Quantity:        1,
				ReasonCode:      "DAMAGE",
			})

		assertDenied14I(t, "quarantine", err)
	})

	t.Run("stocktaking another business's item is refused", func(t *testing.T) {
		_, err := f.service.CreateStocktake(ctx, foreignActorValue, foreignScope,
			inventory.CreateStocktakeInput{
				BranchID:         f.shop.branchShop,
				Type:             inventory.StocktakeCycle,
				InventoryItemIDs: []string{itemID},
			})

		assertDenied14I(t, "stocktake", err)
	})

	t.Run("transferring another business's item is refused", func(t *testing.T) {
		_, err := f.service.CreateTransfer(ctx, foreignActorValue, foreignScope,
			inventory.CreateTransferInput{
				SourceBranchID:      f.shop.branchShop,
				DestinationBranchID: foreignBranch,
				Lines: []inventory.TransferLineInput{
					{
						SourceInventoryItemID:      itemID,
						DestinationInventoryItemID: itemID,
						Quantity:                   1,
					},
				},
			})

		assertDenied14I(t, "transfer", err)
	})

	t.Run("alerts of another business's branch are unreachable", func(t *testing.T) {
		alerts, err := f.service.ListAlerts(
			ctx, foreignActorValue, foreignScope, f.shop.branchShop, "", 50)

		if err == nil && len(alerts) > 0 {
			t.Fatal("a foreign business read another tenant's inventory alerts")
		}
	})
}

// TestSection14IForeignReservationIsUnreachable proves an identifier obtained
// out of band cannot be read or transitioned by another business. This is the
// nested-IDOR gate: the reservation id is real and the caller is real, but the
// pairing is not.
func TestSection14IForeignReservationIsUnreachable(t *testing.T) {
	f := inventory14EFixtureForTest(t)
	ctx := context.Background()

	itemID := f.createItem(t, "14I Foreign Reservation", 10)

	reservation, err := f.service.CreateReservation(ctx, f.actor, f.scope,
		inventory.CreateReservationInput{
			BranchID:   f.shop.branchShop,
			SourceType: "ORDER",
			SourceRef:  "14i-owned-" + uuid.NewString(),
			Lines: []inventory.ReservationLineInput{
				{InventoryItemID: itemID, Quantity: 2},
			},
		})

	if err != nil {
		t.Fatal(err)
	}

	foreignActorValue, foreignScope, _ := foreignActor14I(t, f)

	t.Run("read is refused", func(t *testing.T) {
		_, err := f.service.GetReservation(
			ctx, foreignActorValue, foreignScope, reservation.ID)
		assertDenied14I(t, "reservation read", err)
	})

	for _, transition := range []struct {
		name string
		call func() error
	}{
		{"confirm", func() error {
			_, e := f.service.ConfirmReservation(
				ctx, foreignActorValue, foreignScope, reservation.ID)
			return e
		}},
		{"commit", func() error {
			_, e := f.service.CommitReservation(
				ctx, foreignActorValue, foreignScope, reservation.ID)
			return e
		}},
		{"release", func() error {
			_, e := f.service.ReleaseReservation(
				ctx, foreignActorValue, foreignScope, reservation.ID)
			return e
		}},
		{"cancel", func() error {
			_, e := f.service.CancelReservation(
				ctx, foreignActorValue, foreignScope, reservation.ID)
			return e
		}},
	} {
		t.Run(transition.name+" is refused", func(t *testing.T) {
			assertDenied14I(t, "reservation "+transition.name, transition.call())
		})
	}

	// The owner's reservation must be untouched by every refused attempt.
	after, err := f.service.GetReservation(ctx, f.actor, f.scope, reservation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != reservation.Status {
		t.Fatalf("a refused cross-tenant transition still moved the reservation: %s -> %s",
			reservation.Status, after.Status)
	}
}

// TestSection14IBranchScopeCannotBeEscaped proves an exact-branch scope cannot
// reach a sibling branch of the same business. Tenant isolation and branch
// isolation are separate gates: a Branch Manager is a legitimate member of the
// business and is stopped by scope, not by ownership.
func TestSection14IBranchScopeCannotBeEscaped(t *testing.T) {
	f := inventory14EFixtureForTest(t)
	ctx := context.Background()

	itemID := f.createItem(t, "14I Branch Escape", 10)

	// Authority for a sibling branch of the same business.
	siblingScope, err := inventory.ExactBranchScope(
		f.actor.BusinessID, f.shop.branchShop2)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("balances outside the scoped branch are refused", func(t *testing.T) {
		balances, err := f.service.ListBalances(
			ctx, f.actor, siblingScope, f.shop.branchShop, 50, 0)

		if err == nil && len(balances) > 0 {
			t.Fatal("an exact-branch scope read a sibling branch's balances")
		}
	})

	t.Run("item availability outside the scoped branch is refused", func(t *testing.T) {
		_, err := f.service.Availability(ctx, f.actor, siblingScope, itemID)
		assertDenied14I(t, "cross-branch availability", err)
	})

	t.Run("reserving outside the scoped branch is refused", func(t *testing.T) {
		_, err := f.service.CreateReservation(ctx, f.actor, siblingScope,
			inventory.CreateReservationInput{
				BranchID:   f.shop.branchShop,
				SourceType: "ORDER",
				SourceRef:  "14i-branch-escape-" + uuid.NewString(),
				Lines: []inventory.ReservationLineInput{
					{InventoryItemID: itemID, Quantity: 1},
				},
			})

		assertDenied14I(t, "cross-branch reserve", err)
	})

	t.Run("an item cannot be paired with a branch it does not belong to", func(t *testing.T) {
		// The item is real and the branch is real and both belong to this
		// business. Only the pairing is wrong, which is exactly the nested
		// identifier attack the branch-bound routes have to refuse.
		_, err := f.service.CreateQuarantine(ctx, f.actor, siblingScope,
			inventory.CreateQuarantineInput{
				BranchID:        f.shop.branchShop2,
				InventoryItemID: itemID,
				Quantity:        1,
				ReasonCode:      "DAMAGE",
			})

		assertDenied14I(t, "item/branch mispairing", err)
	})
}
