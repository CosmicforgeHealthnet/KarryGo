package main

import (
	"testing"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
)

// Section 17B's required checkout sequence puts "validate Vendor active" and
// "validate branch active" ahead of catalogue, price and stock. Food's own
// resolver answers both as part of one sellability decision. Shop's does not:
// `requireShopBranch` reads the branch status and never inspects it, accepts a
// DRAFT or PAUSED capability, and never looks at the business row at all.
//
// So Section 17B has to assert both itself for Shop rather than inherit them,
// and these are the tests that hold it to that. A suspended vendor whose
// catalogue rows are untouched is exactly the case that would otherwise sell.

func (e *s17bEnv) setBusinessStatus(businessID, status string) {
	e.t.Helper()

	if _, err := e.pool.Exec(
		e.ctx(),
		`UPDATE vendor_businesses SET status=$2 WHERE id=$1`,
		businessID,
		status,
	); err != nil {
		e.t.Fatal(err)
	}
}

func (e *s17bEnv) setBranchStatus(branchID, status string) {
	e.t.Helper()

	if _, err := e.pool.Exec(
		e.ctx(),
		`UPDATE vendor_branches SET status=$2 WHERE id=$1`,
		branchID,
		status,
	); err != nil {
		e.t.Fatal(err)
	}
}

func (e *s17bEnv) setBranchCapabilityStatus(branchID, status string) {
	e.t.Helper()

	if _, err := e.pool.Exec(
		e.ctx(),
		`UPDATE vendor_branch_capabilities SET status=$2 WHERE branch_id=$1`,
		branchID,
		status,
	); err != nil {
		e.t.Fatal(err)
	}
}

// section17BAssertShopRefused runs a checkout that must be refused with the
// given line reason, and asserts nothing was held.
func section17BAssertShopRefused(
	t *testing.T,
	env *s17bEnv,
	current cart.Cart,
	customer string,
	item s17bShopItem,
	expectedReason string,
	seededQuantity int64,
) {
	t.Helper()

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
				line.ReasonCode == expectedReason {
				found = true
			}
		}
	}

	if !found {
		t.Fatalf("no line reported %s:\n%s", expectedReason, describeSession(session))
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != seededQuantity {
		t.Fatalf("available=%d want=%d: a refused line still took stock",
			available, seededQuantity)
	}

	if released := env.reloadCart(customer, current.ID); released.Status != cart.StatusActive {
		t.Fatalf("cart status=%s want ACTIVE", released.Status)
	}
}

func TestSection17BShopRefusesASuspendedVendor(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Suspended Vendor Stock",
		Quantity:   6,
		PriceMinor: 20000,
	})

	customer := env.customer("biz_suspended")
	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 2})

	// The catalogue is untouched: only the vendor's own standing changed.
	env.setBusinessStatus(env.businessA, "SUSPENDED")

	section17BAssertShopRefused(
		t,
		env,
		current,
		customer,
		item,
		checkout.ReasonBusinessInactive,
		6,
	)
}

func TestSection17BShopRefusesAnInactiveBranch(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Closed Branch Stock",
		Quantity:   4,
		PriceMinor: 20000,
	})

	customer := env.customer("branch_closed")
	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 1})

	env.setBranchStatus(env.branchA1, "CLOSED")

	section17BAssertShopRefused(
		t,
		env,
		current,
		customer,
		item,
		checkout.ReasonBranchInactive,
		4,
	)
}

// A paused branch is the ordinary "we are not trading right now" case, and it
// must refuse just as firmly as a closed one.
func TestSection17BShopRefusesAPausedBranch(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Paused Branch Stock",
		Quantity:   3,
		PriceMinor: 20000,
	})

	customer := env.customer("branch_paused")
	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 1})

	env.setBranchStatus(env.branchA1, "PAUSED")

	section17BAssertShopRefused(
		t,
		env,
		current,
		customer,
		item,
		checkout.ReasonBranchInactive,
		3,
	)
}

// Section 12's own branch check accepts a capability in DRAFT or PAUSED,
// because a vendor still building a catalogue needs to be able to. Selling
// from it is a different question.
func TestSection17BShopRefusesANonActiveBranchCapability(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Paused Capability Stock",
		Quantity:   5,
		PriceMinor: 20000,
	})

	customer := env.customer("cap_paused")
	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 1})

	env.setBranchCapabilityStatus(env.branchA1, "PAUSED")

	section17BAssertShopRefused(
		t,
		env,
		current,
		customer,
		item,
		checkout.ReasonCapabilityInactive,
		5,
	)
}

// The mirror case: an ACTIVE vendor, branch and capability still sell.
func TestSection17BShopSellsFromAnActiveTenant(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Active Tenant Stock",
		Quantity:   5,
		PriceMinor: 20000,
	})

	customer := env.customer("tenant_active")
	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 2})

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
}

// TestSection17BShopDefersOnlyKnownStockReasons pins the direction of the
// Section 12 reason filter.
//
// Section 17B defers to Section 14 for an allowlisted set of stock answers
// and refuses everything else it does not recognise. If that were inverted -
// a denylist of catalogue reasons - a reason code Section 12 added later
// would fall through this adapter and sell. This test states the set, so
// widening it has to be a deliberate edit here too.
func TestSection17BShopDefersOnlyKnownStockReasons(t *testing.T) {
	t.Parallel()

	expected := map[string]bool{
		"SELLABLE":                        true,
		"OUT_OF_STOCK":                    true,
		"INSUFFICIENT_STOCK":              true,
		"BACKORDER_AVAILABLE":             true,
		"PREORDER_AVAILABLE":              true,
		"INVENTORY_CONVERSION_UNRESOLVED": true,
	}

	for reason := range expected {
		if !shopStockDeferredReasons[reason] {
			t.Errorf("%s should be deferred to Section 14 and is not", reason)
		}
	}

	for reason := range shopStockDeferredReasons {
		if !expected[reason] {
			t.Errorf("%s was added to the deferred set without a decision here", reason)
		}
	}

	// Every catalogue denial Section 12 declares today, and an invented one,
	// must all be refusals rather than deferrals.
	for _, reason := range []string{
		"PRODUCT_INACTIVE",
		"VARIANT_INACTIVE",
		"LISTING_INACTIVE",
		"SELL_UNIT_INACTIVE",
		"WHOLESALE_DISABLED",
		"RETAIL_DISABLED",
		"MOQ_NOT_MET",
		"ORDER_MULTIPLE_INVALID",
		"SHOP_CAPABILITY_DISABLED",
		"SOME_REASON_SECTION_12_ADDS_LATER",
	} {
		if shopStockDeferredReasons[reason] {
			t.Errorf("%s is being deferred rather than refused", reason)
		}
	}
}
