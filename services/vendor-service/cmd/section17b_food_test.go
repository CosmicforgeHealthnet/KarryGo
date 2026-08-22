package main

import (
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
	"cosmicforge/logistics/services/vendor-service/internal/features/food"
)

// Section 17B Food revalidation.
//
// Food's own resolver already answers business, branch, capability, menu,
// service window, section, item and operational state as one decision. What
// Section 17B adds is that checkout ENFORCES that decision: Section 17A
// deliberately lets an unsellable item sit in a cart, and this is where that
// stops being allowed.

type s17bFoodItem struct {
	MenuID    string
	SectionID string
	ItemID    string
}

func (e *s17bEnv) foodActor() food.Actor {
	return food.Actor{
		StaffID:       e.staffA,
		VendorID:      e.businessA,
		CorrelationID: "s17b-food",
	}
}

// seedFoodItem builds an ACTIVE menu with one ACTIVE item and no schedule,
// which is the ordinary always-open restaurant case.
func (e *s17bEnv) seedFoodItem(name string, priceMinor int64) s17bFoodItem {
	e.t.Helper()

	ctx := e.ctx()
	actor := e.foodActor()
	scope := food.Scope{Global: true}

	menu, err := e.foodService.CreateMenu(
		ctx,
		actor,
		scope,
		food.CreateMenuInput{
			BranchID:       e.foodBranch,
			CapabilityType: "RESTAURANT",
			Name:           "Menu " + name,
		},
	)
	if err != nil {
		e.t.Fatal(err)
	}

	section, err := e.foodService.CreateSection(
		ctx,
		actor,
		scope,
		menu.ID,
		food.CreateSectionInput{Name: "Mains"},
	)
	if err != nil {
		e.t.Fatal(err)
	}

	item, err := e.foodService.CreateItem(
		ctx,
		actor,
		scope,
		menu.ID,
		section.ID,
		food.CreateItemInput{
			Name:           name,
			BasePriceMinor: priceMinor,
		},
	)
	if err != nil {
		e.t.Fatal(err)
	}

	active := "ACTIVE"

	item, err = e.foodService.UpdateItem(
		ctx,
		actor,
		scope,
		item.ID,
		food.UpdateItemInput{
			Status:          &active,
			ExpectedVersion: item.Version,
		},
	)
	if err != nil {
		e.t.Fatal(err)
	}

	if _, err = e.foodService.SetMenuStatus(
		ctx,
		actor,
		scope,
		menu.ID,
		"ACTIVE",
		menu.Version,
	); err != nil {
		e.t.Fatal(err)
	}

	return s17bFoodItem{
		MenuID:    menu.ID,
		SectionID: section.ID,
		ItemID:    item.ID,
	}
}

func (e *s17bEnv) seedFoodCart(
	customerID string,
	item s17bFoodItem,
	quantity int64,
) cart.Cart {
	e.t.Helper()

	ctx := e.ctx()

	current, err := e.carts.CreateOrGetActiveCart(
		ctx,
		customerID,
		cart.DomainFood,
	)
	if err != nil {
		e.t.Fatal(err)
	}

	current, err = e.carts.AddItem(
		ctx,
		customerID,
		current.ID,
		current.Version,
		cart.AddItemInput{
			ItemRef:  item.ItemID,
			MenuRef:  item.MenuID,
			Quantity: quantity,
		},
	)
	if err != nil {
		e.t.Fatalf("add food cart item: %v", err)
	}

	current, err = e.carts.SetDeliveryAddress(
		ctx,
		customerID,
		current.ID,
		"saved-address-"+e.suffix,
		current.Version,
	)
	if err != nil {
		e.t.Fatal(err)
	}

	return current
}

// TestSection17BFoodValidatesWithoutReservingAnyStock is the Food happy path.
//
// The point of the second half is the boundary: Food's current production
// architecture exposes availability, not a reservation interface, so a Food
// checkout must hold nothing.
func TestSection17BFoodValidatesWithoutReservingAnyStock(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedFoodItem("Jollof Rice", 320000)

	customer := env.customer("food_ok")

	current := env.seedFoodCart(customer, item, 2)

	session, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	)
	if err != nil {
		t.Fatal(err)
	}

	if session.Status != checkout.StatusValidated {
		t.Fatal(describeSession(session))
	}

	if session.Domain != cart.DomainFood {
		t.Fatalf("domain=%s", session.Domain)
	}

	if len(session.Groups) != 1 {
		t.Fatalf("groups=%d want=1: a Food cart is bound to one branch", len(session.Groups))
	}

	group := session.Groups[0]

	if group.BranchID != env.foodBranch {
		t.Fatalf("branch=%s want=%s", group.BranchID, env.foodBranch)
	}

	if group.RequiresStockReservation ||
		group.ReservationStatus != checkout.ReservationNotRequired ||
		group.InventoryReservationRef != "" {
		t.Fatalf("a Food group claimed a stock hold: %+v", group)
	}

	if session.ReservationCoordinationState != checkout.CoordinationNone {
		t.Fatalf("coordination=%s want NONE", session.ReservationCoordinationState)
	}

	if session.QuotedItemsTotalMinor != 640000 ||
		session.MaxAuthorizedItemsTotalMinor != 640000 {
		t.Fatalf("quoted=%d maximum=%d",
			session.QuotedItemsTotalMinor,
			session.MaxAuthorizedItemsTotalMinor,
		)
	}

	// The cart is still frozen, because a Food candidate is still a candidate.
	if frozen := env.reloadCart(customer, current.ID); frozen.Status != cart.StatusCheckoutInProgress {
		t.Fatalf("cart status=%s", frozen.Status)
	}
}

// TestSection17BFoodRefusesASoldOutItem is the sellability gate. Section 17A
// lets a sold-out item sit in the cart; checkout must not.
func TestSection17BFoodRefusesASoldOutItem(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedFoodItem("Suya", 150000)

	customer := env.customer("food_soldout")

	current := env.seedFoodCart(customer, item, 1)

	availability, err := env.foodService.SetItemAvailability(
		env.ctx(),
		env.foodActor(),
		food.Scope{Global: true},
		item.ItemID,
		food.SetAvailabilityInput{
			Status: "SOLD_OUT",
			Reason: "Kitchen ran out",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if availability.Status != "SOLD_OUT" {
		t.Fatalf("availability=%s", availability.Status)
	}

	session, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	)
	if err != nil {
		t.Fatal(err)
	}

	if session.Status != checkout.StatusFailed ||
		session.FailureCode != checkout.FailureLineNotSellable {
		t.Fatal(describeSession(session))
	}

	found := false

	for _, group := range session.Groups {
		for _, line := range group.Lines {
			if line.Status == checkout.LineUnavailable &&
				line.ReasonCode == food.ReasonItemSoldOut {
				found = true
			}
		}
	}

	if !found {
		t.Fatalf("no line reported the authoritative sold-out reason:\n%s",
			describeSession(session))
	}

	// A refused candidate hands the cart straight back.
	if released := env.reloadCart(customer, current.ID); released.Status != cart.StatusActive {
		t.Fatalf("cart status=%s want ACTIVE", released.Status)
	}
}

// TestSection17BFoodRefusesAPausedMenu proves checkout re-reads the whole
// authoritative graph, not just the item row the cart pointed at.
func TestSection17BFoodRefusesAPausedMenu(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedFoodItem("Egusi", 200000)

	customer := env.customer("food_menu")

	current := env.seedFoodCart(customer, item, 1)

	menu, err := env.foodService.GetMenu(
		env.ctx(),
		env.foodActor(),
		food.Scope{Global: true},
		item.MenuID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = env.foodService.SetMenuStatus(
		env.ctx(),
		env.foodActor(),
		food.Scope{Global: true},
		item.MenuID,
		"PAUSED",
		menu.Version,
	); err != nil {
		t.Fatal(err)
	}

	session, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	)
	if err != nil {
		t.Fatal(err)
	}

	if session.Status != checkout.StatusFailed {
		t.Fatal(describeSession(session))
	}

	found := false

	for _, group := range session.Groups {
		for _, line := range group.Lines {
			if line.ReasonCode == food.ReasonMenuNotActive {
				found = true
			}
		}
	}

	if !found {
		t.Fatalf("the paused menu was not the reported reason:\n%s",
			describeSession(session))
	}
}

// TestSection17BReportsAPriceChangeRatherThanRepricingSilently is the price
// version gate. Section 17B must surface the difference; creating the final
// immutable quote is Section 17D's job, not this one's.
func TestSection17BReportsAPriceChangeRatherThanRepricingSilently(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedFoodItem("Pepper Soup", 100000)

	customer := env.customer("food_price")

	current := env.seedFoodCart(customer, item, 2)

	snapshot := current.Items[0].UnitPriceMinor

	if snapshot != 100000 {
		t.Fatalf("cart snapshot price=%d want=100000", snapshot)
	}

	stored, _, _, err := env.foodService.GetItem(
		env.ctx(),
		env.foodActor(),
		food.Scope{Global: true},
		item.ItemID,
	)
	if err != nil {
		t.Fatal(err)
	}

	raised := int64(180000)

	if _, err = env.foodService.UpdateItem(
		env.ctx(),
		env.foodActor(),
		food.Scope{Global: true},
		item.ItemID,
		food.UpdateItemInput{
			BasePriceMinor:  &raised,
			ExpectedVersion: stored.Version,
		},
	); err != nil {
		t.Fatal(err)
	}

	session, err := env.checkout.StartCheckout(
		env.ctx(),
		customer,
		current.ID,
		current.Version,
	)
	if err != nil {
		t.Fatal(err)
	}

	// A price change is reported, not refused: the customer may well accept it.
	if session.Status != checkout.StatusValidated {
		t.Fatal(describeSession(session))
	}

	if !session.PriceChanged {
		t.Fatal("the session did not report that a price changed")
	}

	line := session.Groups[0].Lines[0]

	if line.Status != checkout.LineChanged ||
		line.ReasonCode != checkout.ReasonPriceChanged {
		t.Fatalf("line status=%s reason=%s", line.Status, line.ReasonCode)
	}

	if line.SnapshotUnitPriceMinor != 100000 {
		t.Fatalf("snapshot price=%d want=100000", line.SnapshotUnitPriceMinor)
	}

	if line.UnitPriceMinor != 180000 {
		t.Fatalf("authoritative price=%d want=180000", line.UnitPriceMinor)
	}

	// The candidate is quoted from the AUTHORITATIVE price, never from the
	// cart snapshot the customer last saw.
	if session.QuotedItemsTotalMinor != 360000 {
		t.Fatalf("quoted=%d want=360000", session.QuotedItemsTotalMinor)
	}
}

// TestSection17BFoodCartCannotStartCheckoutWhileFoodIsDisabled is a small
// dependency guard: an authority that cannot answer must produce an error, not
// a confident refusal that looks like the vendor said no.
func TestSection17BRefusesWhenAnAuthorityIsAbsent(t *testing.T) {
	t.Parallel()

	service, err := checkout.NewService(checkout.Options{
		Store:          checkout.NewStore(nil),
		ReservationTTL: 10 * time.Minute,
	})

	if err == nil {
		_ = service
		t.Fatal("a checkout service with no store or authorities was accepted")
	}

	if !errors.Is(err, checkout.ErrDependencyMissing) &&
		!errors.Is(err, checkout.ErrInvalidTTL) {
		t.Fatalf("err=%v", err)
	}
}
