package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
	"cosmicforge/logistics/services/vendor-service/internal/features/food"
	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	"cosmicforge/logistics/services/vendor-service/internal/features/shop"
)

// Section 17B integration harness.
//
// These tests run against the REAL production adapters from
// cmd/section17b_adapters.go, the real Section 17A cart service, the real
// Section 12 Shop catalogue and the real Section 14 Inventory authority, on
// real PostgreSQL. A checkout that only ever reserved from a fake would prove
// nothing about the one property this section exists for: that two customers
// cannot both buy the last unit.

// -----------------------------------------------------------------------------
// Test doubles that are genuinely outside Vendor Service
// -----------------------------------------------------------------------------

// s17bScopes grants global vendor scope. Section 5/6 IAM is not what these
// tests are about, and it has its own certification.
type s17bScopes struct{}

func (s17bScopes) ResolveScope(context.Context, string, string) (shop.Scope, error) {
	return shop.Scope{Global: true}, nil
}

type s17bFoodScopes struct{}

func (s17bFoodScopes) ResolveScope(context.Context, string, string) (food.Scope, error) {
	return food.Scope{Global: true}, nil
}

// s17bAddresses stands in for Customer Service's saved-location authority,
// which lives in another service entirely.
type s17bAddresses struct{}

func (s17bAddresses) ResolveSavedAddress(
	_ context.Context,
	customerID string,
	addressRef string,
) (cart.AddressSnapshot, error) {
	return cart.AddressSnapshot{
		SourceAddressRef: addressRef,
		Title:            "Home",
		Address:          "12 Test Close, Lagos",
		Latitude:         6.5244,
		Longitude:        3.3792,
		SnapshottedAt:    time.Now().UTC(),
	}, nil
}

type s17bNoopInventoryEvidence struct{}

func (s17bNoopInventoryEvidence) RecordInventoryEvidence(
	context.Context,
	inventory.InventoryTransaction,
	inventory.Evidence,
) error {
	return nil
}

type s17bNoopInventoryEvents struct{}

func (s17bNoopInventoryEvents) PublishInventoryEvent(
	context.Context,
	inventory.InventoryTransaction,
	inventory.Event,
) error {
	return nil
}

// testClock is a movable clock so a reservation TTL can be made to elapse
// without a test ever sleeping.
type testClock struct {
	mu     sync.Mutex
	offset time.Duration
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return time.Now().UTC().Add(c.offset)
}

func (c *testClock) advance(by time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.offset += by
}

// -----------------------------------------------------------------------------
// Fixture
// -----------------------------------------------------------------------------

type s17bEnv struct {
	t    *testing.T
	pool *pgxpool.Pool

	shopService *shop.Service
	foodService *food.Service
	stock       *inventory.StockService
	carts       *cart.Service
	checkout    *checkout.Service

	clock *testClock
	ttl   time.Duration

	suffix string

	businessA string
	businessB string

	branchA1 string
	branchA2 string
	branchB1 string

	foodBranch string

	staffA string
	staffB string
}

func (e *s17bEnv) ctx() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

	e.t.Cleanup(cancel)

	return ctx
}

func (e *s17bEnv) customer(label string) string {
	return "s17b_cust_" + label + "_" + e.suffix
}

func (e *s17bEnv) shopActor(business, staff string) shop.Actor {
	return shop.Actor{
		StaffID:       staff,
		VendorID:      business,
		CorrelationID: "s17b-test",
	}
}

func newSection17BEnv(t *testing.T, ttl time.Duration) *s17bEnv {
	t.Helper()

	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")

	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping Section 17B integration")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(pool.Close)

	suffix := uuid.NewString()[:12]

	env := &s17bEnv{
		t:      t,
		pool:   pool,
		clock:  &testClock{},
		ttl:    ttl,
		suffix: suffix,

		businessA: "s17b_biz_a_" + suffix,
		businessB: "s17b_biz_b_" + suffix,

		branchA1: "s17b_br_a1_" + suffix,
		branchA2: "s17b_br_a2_" + suffix,
		branchB1: "s17b_br_b1_" + suffix,

		foodBranch: "s17b_br_food_" + suffix,

		staffA: "s17b_staff_a_" + suffix,
		staffB: "s17b_staff_b_" + suffix,
	}

	env.seedTenants(ctx)
	env.buildServices()

	return env
}

func (e *s17bEnv) seedTenants(ctx context.Context) {
	e.t.Helper()

	for _, seed := range []struct{ business, staff, user string }{
		{e.businessA, e.staffA, "s17b_user_a_" + e.suffix},
		{e.businessB, e.staffB, "s17b_user_b_" + e.suffix},
	} {
		if _, err := e.pool.Exec(
			ctx,
			`INSERT INTO vendor_users(id,email) VALUES($1,$2)`,
			seed.user,
			seed.user+"@example.test",
		); err != nil {
			e.t.Fatal(err)
		}

		if _, err := e.pool.Exec(
			ctx,
			`INSERT INTO vendor_staff(id,vendor_id,user_id) VALUES($1,$2,$3)`,
			seed.staff,
			seed.business,
			seed.user,
		); err != nil {
			e.t.Fatal(err)
		}

		if _, err := e.pool.Exec(
			ctx,
			`INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone)
             VALUES($1,'Section 17B test','ACTIVE',$2,'NGN','Africa/Lagos')`,
			seed.business,
			seed.staff,
		); err != nil {
			e.t.Fatal(err)
		}
	}

	branches := []struct{ id, business string }{
		{e.branchA1, e.businessA},
		{e.branchA2, e.businessA},
		{e.branchB1, e.businessB},
		{e.foodBranch, e.businessA},
	}

	for _, branch := range branches {
		if _, err := e.pool.Exec(
			ctx,
			`INSERT INTO vendor_branches(
                id,business_id,display_name,address_line1,city,region,
                country_code,latitude,longitude,timezone,status)
             VALUES($1,$2,$3,'1 Test Street','Lagos','Lagos','NG',6.5244,3.3792,'Africa/Lagos','ACTIVE')`,
			branch.id,
			branch.business,
			"Branch "+branch.id,
		); err != nil {
			e.t.Fatal(err)
		}
	}

	capabilities := []struct{ business, branch, capability string }{
		{e.businessA, e.branchA1, "SHOP_GROCERY"},
		{e.businessA, e.branchA2, "SHOP_GROCERY"},
		{e.businessB, e.branchB1, "SHOP_GROCERY"},
		{e.businessA, e.foodBranch, "RESTAURANT"},
	}

	capabilityIDs := map[string]string{}

	for index, capability := range capabilities {
		key := capability.business + ":" + capability.capability

		capabilityID, seeded := capabilityIDs[key]

		if !seeded {
			capabilityID = fmt.Sprintf("s17b_cap_%d_%s", index, e.suffix)
			capabilityIDs[key] = capabilityID

			if _, err := e.pool.Exec(
				ctx,
				`INSERT INTO vendor_capabilities(id,business_id,capability_type,status)
                 VALUES($1,$2,$3,'ACTIVE')`,
				capabilityID,
				capability.business,
				capability.capability,
			); err != nil {
				e.t.Fatal(err)
			}
		}

		if _, err := e.pool.Exec(
			ctx,
			`INSERT INTO vendor_branch_capabilities(
                id,business_id,branch_id,vendor_capability_id,capability_type,status)
             VALUES($1,$2,$3,$4,$5,'ACTIVE')`,
			fmt.Sprintf("s17b_bcap_%d_%s", index, e.suffix),
			capability.business,
			capability.branch,
			capabilityID,
			capability.capability,
		); err != nil {
			e.t.Fatal(err)
		}
	}
}

// buildServices wires the same components cmd/main.go wires, with two
// deliberate substitutions: vendor scope resolution and Customer Service
// address resolution, neither of which is Section 17B's subject.
func (e *s17bEnv) buildServices() {
	e.t.Helper()

	outbox := events.NewOutboxRepository(e.pool)

	e.shopService = shop.NewService(shop.Options{
		DB:     e.pool,
		Scopes: s17bScopes{},
		Flags:  shop.Flags{ShopEnabled: true, WritesEnabled: true},
		Events: events.NewDomainPublisher(outbox),
	})

	e.foodService = food.NewService(food.Options{
		DB:     e.pool,
		Scopes: s17bFoodScopes{},
		Flags:  food.Flags{CatalogueEnabled: true, WritesEnabled: true},
		Events: events.NewDomainPublisher(outbox),
	})

	inventoryFoundation := inventory.NewService(inventory.Dependencies{
		Flags: inventory.Flags{Enabled: true, WritesEnabled: true},
	})

	e.stock = inventory.NewStockService(inventory.StockOptions{
		DB:           e.pool,
		Foundation:   inventoryFoundation,
		Transactions: inventory.NewPostgresTransactionBoundary(e.pool),
		Evidence:     s17bNoopInventoryEvidence{},
		Events:       s17bNoopInventoryEvents{},
	})

	e.carts = cart.NewService(
		cart.NewStore(e.pool),
		newSection17AFoodAuthority(e.pool, e.foodService),
		newSection17AShopAuthority(e.pool, e.shopService),
		s17bAddresses{},
	)

	service, err := checkout.NewService(checkout.Options{
		Store: checkout.NewStore(e.pool),

		Food: newSection17BFoodAuthority(e.pool, e.foodService),
		Shop: newSection17BShopAuthority(e.pool, e.shopService, e.stock),

		Stock: newSection17BStockAuthority(e.stock),

		ReservationTTL: e.ttl,
		StrandedGrace:  30 * time.Second,

		Now: e.clock.now,
	})
	if err != nil {
		e.t.Fatal(err)
	}

	e.checkout = service
}

// -----------------------------------------------------------------------------
// Shop catalogue + stock seeding
// -----------------------------------------------------------------------------

type s17bShopItem struct {
	ProductID  string
	VariantID  string
	ListingID  string
	SellUnitID string

	// InventoryItemID is the generic Section 14 identity, which the Section
	// 14C compatibility bridge keeps equal to the Shop stock basis id.
	InventoryItemID string

	MeasureID string
}

type s17bStockOptions struct {
	Business string
	Staff    string
	Branch   string

	Name string

	Quantity int64

	PriceMinor int64

	// VariableWeight seeds a VARIABLE_FULFILMENT basis whose sell unit
	// requires an actual measurement, which is the variable-weight path.
	VariableWeight bool

	OverfillBps  int
	UnderfillBps int

	// FractionDenominator seeds a fractional sell unit, which has no
	// conversion authority anywhere and must therefore fail closed.
	FractionDenominator int

	// MeasureID overrides the sell-unit and stock-basis measure. It is needed
	// for the fractional case: Shop refuses a fraction that does not divide a
	// measure into whole base units, so a "half" unit only exists on a
	// measure with no universal content, such as a paint container.
	MeasureID string
}

func (e *s17bEnv) seedShopItem(opts s17bStockOptions) s17bShopItem {
	e.t.Helper()

	ctx := e.ctx()

	actor := e.shopActor(opts.Business, opts.Staff)
	scope := shop.Scope{Global: true}

	if _, err := e.shopService.SetOperatingModels(
		ctx,
		actor,
		scope,
		opts.Branch,
		shop.SetOperatingModelsInput{
			PrimaryModel: shop.ModelSupermarket,
			Models:       []string{shop.ModelSupermarket},
		},
	); err != nil {
		e.t.Fatal(err)
	}

	category, err := e.shopService.CreateCategory(
		ctx,
		actor,
		scope,
		shop.CreateCategoryInput{
			Name: "S17B " + opts.Name + " " + uuid.NewString()[:8],
		},
	)
	if err != nil {
		e.t.Fatal(err)
	}

	product, err := e.shopService.CreateProduct(
		ctx,
		actor,
		scope,
		shop.CreateProductInput{
			CategoryID:      category.ID,
			Name:            opts.Name,
			ProductClass:    "PACKAGED",
			OperatingModels: []string{shop.ModelSupermarket},
		},
	)
	if err != nil {
		e.t.Fatal(err)
	}

	// Shop products are created DRAFT. A checkout must refuse a draft product,
	// so the fixture activates it explicitly rather than relying on a default.
	activeStatus := "ACTIVE"

	if _, err = e.shopService.UpdateProduct(
		ctx,
		actor,
		scope,
		product.ID,
		shop.UpdateProductInput{
			Status:          &activeStatus,
			ExpectedVersion: product.Version,
		},
	); err != nil {
		e.t.Fatal(err)
	}

	variant, err := e.shopService.CreateVariant(
		ctx,
		actor,
		scope,
		product.ID,
		shop.CreateVariantInput{
			Name:             opts.Name + " unit",
			PackageLevel:     "EACH",
			RetailPriceMinor: &opts.PriceMinor,
		},
	)
	if err != nil {
		e.t.Fatal(err)
	}

	measureID := "smd_piece"
	measurementMode := shop.ModeFixed
	consumption := shop.ConsumptionSameUnit
	measurementRequired := false

	if opts.VariableWeight {
		measureID = "smd_gram"
		measurementMode = shop.ModeVariableWeight
		consumption = shop.ConsumptionVariable
		measurementRequired = true
	}

	if opts.MeasureID != "" {
		measureID = opts.MeasureID
	}

	sellUnitInput := shop.CreateSellUnitInput{
		VariantID:           variant.ID,
		MeasureID:           measureID,
		MeasurementMode:     measurementMode,
		PriceMinor:          &opts.PriceMinor,
		RetailEnabled:       boolPointer(true),
		MeasurementRequired: &measurementRequired,
	}

	if opts.OverfillBps > 0 {
		overfill := opts.OverfillBps
		sellUnitInput.OverfillBps = &overfill
	}

	if opts.UnderfillBps > 0 {
		underfill := opts.UnderfillBps
		sellUnitInput.UnderfillBps = &underfill
	}

	if opts.FractionDenominator > 1 {
		numerator := 1
		denominator := opts.FractionDenominator

		sellUnitInput.FractionNumerator = &numerator
		sellUnitInput.FractionDenominator = &denominator
	}

	unit, err := e.shopService.CreateSellUnit(
		ctx,
		actor,
		scope,
		product.ID,
		sellUnitInput,
	)
	if err != nil {
		e.t.Fatal(err)
	}

	listing, err := e.shopService.CreateListing(
		ctx,
		actor,
		scope,
		opts.Branch,
		shop.CreateListingInput{
			VariantID:      variant.ID,
			OperatingModel: shop.ModelSupermarket,
			RetailEnabled:  boolPointer(true),
			SellUnitIDs:    []string{unit.ID},
		},
	)
	if err != nil {
		e.t.Fatal(err)
	}

	basis, err := e.shopService.ConfigureStockBasis(
		ctx,
		actor,
		scope,
		opts.Branch,
		shop.ConfigureStockBasisInput{
			ListingID:        listing.ID,
			MeasureID:        measureID,
			ConsumptionMode:  consumption,
			AllocationPolicy: shop.AllocationFIFO,
			LotTracking:      true,
		},
	)
	if err != nil {
		e.t.Fatal(err)
	}

	item := s17bShopItem{
		ProductID:       product.ID,
		VariantID:       variant.ID,
		ListingID:       listing.ID,
		SellUnitID:      unit.ID,
		InventoryItemID: basis.ID,
		MeasureID:       measureID,
	}

	if opts.Quantity > 0 {
		e.receiveStock(opts.Business, opts.Staff, opts.Branch, item, opts.Quantity)
	}

	return item
}

// receiveStock puts physical stock into the GENERIC Section 14 ledger.
//
// Shop's own Section 12B inventory tables are deliberately not seeded here:
// Section 14 is the stock authority for this service, and a test that seeded
// both would not notice if checkout read the wrong one.
func (e *s17bEnv) receiveStock(
	business string,
	staff string,
	branch string,
	item s17bShopItem,
	quantity int64,
) {
	e.t.Helper()

	scope, err := inventory.ExactBranchScope(business, branch)

	if err != nil {
		e.t.Fatal(err)
	}

	if _, err = e.stock.Receive(
		e.ctx(),
		inventory.Actor{
			StaffID:       staff,
			BusinessID:    business,
			CorrelationID: "s17b-seed",
		},
		scope,
		inventory.ReceiveInput{
			BranchID:        branch,
			InventoryItemID: item.InventoryItemID,
			Quantity:        quantity,
			SourceType:      "INTERNAL",
			SourceRef:       "s17b-seed-" + uuid.NewString(),
			Lot: inventory.LotMetadata{
				LotNumber: "s17b-" + uuid.NewString()[:8],
			},
		},
	); err != nil {
		e.t.Fatal(err)
	}
}

func (e *s17bEnv) availableStock(
	business string,
	branch string,
	inventoryItemID string,
) int64 {
	e.t.Helper()

	scope, err := inventory.ExactBranchScope(business, branch)

	if err != nil {
		e.t.Fatal(err)
	}

	balance, err := e.stock.Availability(
		e.ctx(),
		inventory.Actor{
			ActorRef:   "s17b-assert",
			BusinessID: business,
		},
		scope,
		inventoryItemID,
	)
	if err != nil {
		e.t.Fatal(err)
	}

	return balance.AvailableQuantity
}

func boolPointer(value bool) *bool { return &value }

// -----------------------------------------------------------------------------
// Cart seeding
// -----------------------------------------------------------------------------

type s17bCartLine struct {
	Item     s17bShopItem
	Quantity int64

	ExpectedMeasureQuantity int64
}

// seedShopCart builds a real Section 17A cart through the real cart service,
// including the real Section 17A Shop authority adapter.
func (e *s17bEnv) seedShopCart(
	customerID string,
	lines ...s17bCartLine,
) cart.Cart {
	e.t.Helper()

	ctx := e.ctx()

	current, err := e.carts.CreateOrGetActiveCart(
		ctx,
		customerID,
		cart.DomainShop,
	)
	if err != nil {
		e.t.Fatal(err)
	}

	for _, line := range lines {
		input := cart.AddItemInput{
			ItemRef:     line.Item.ProductID,
			VariantRef:  line.Item.VariantID,
			ListingRef:  line.Item.ListingID,
			SellUnitRef: line.Item.SellUnitID,
			Quantity:    line.Quantity,
		}

		if line.ExpectedMeasureQuantity > 0 {
			input.RequestedMeasureRef = line.Item.MeasureID
			input.ExpectedMeasureQuantity = line.ExpectedMeasureQuantity
		}

		current, err = e.carts.AddItem(
			ctx,
			customerID,
			current.ID,
			current.Version,
			input,
		)
		if err != nil {
			e.t.Fatalf("add cart item: %v", err)
		}
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

func (e *s17bEnv) reloadCart(customerID, cartID string) cart.Cart {
	e.t.Helper()

	current, err := e.carts.GetCart(e.ctx(), customerID, cartID)

	if err != nil {
		e.t.Fatal(err)
	}

	return current
}

// describeSession renders the per-line reasons a failed candidate carries, so
// a failing assertion says why rather than only that.
func describeSession(session checkout.Session) string {
	out := fmt.Sprintf(
		"session=%s status=%s failure=%s coordination=%s",
		session.ID,
		session.Status,
		session.FailureCode,
		session.ReservationCoordinationState,
	)

	for _, group := range session.Groups {
		out += fmt.Sprintf(
			"\n  group %s/%s reservation=%s ref=%s failure=%s",
			group.BusinessID,
			group.BranchID,
			group.ReservationStatus,
			group.InventoryReservationRef,
			group.ReservationFailureCode,
		)

		for _, line := range group.Lines {
			out += fmt.Sprintf(
				"\n    line %s status=%s reason=%s detail=%q basis=%s qty=%d",
				line.CartItemID,
				line.Status,
				line.ReasonCode,
				line.ReasonDetail,
				line.ReservationBasis,
				line.ReservationQuantity,
			)
		}
	}

	return out
}
