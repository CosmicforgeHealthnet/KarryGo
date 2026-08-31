package purchasing

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
)

// Section 15D real-PostgreSQL certification harness.
//
// Sections 15A/15B/15C shipped with unit tests over pure functions. That
// proves the arithmetic but not the system: whether accepted stock actually
// reaches Inventory exactly once, whether a foreign tenant is refused by the
// SQL rather than by a Go guard someone can forget, and whether two concurrent
// confirmations both post. Those are claims about the database, so this
// harness drives the real services against a real database.

// noopReceivingEvents records nothing. Outbox behaviour has its own gate; the
// receiving gates below are about stock and evidence.
type noopReceivingEvents struct{}

func (noopReceivingEvents) Publish(
	_ context.Context, _ pgx.Tx,
	_ string, _ string, _ string, _ int64,
	_ string, _ string, _ string, _ any,
) error {
	return nil
}

type s15Fixture struct {
	pool *pgxpool.Pool

	suppliers *Service
	orders    *PurchaseOrderService
	receiving *GoodsReceivingService
	stock     *inventory.StockService

	businessID string
	branchID   string
	staffID    string

	// A second fully-formed business, for cross-tenant probes.
	foreignBusinessID string
	foreignBranchID   string
	foreignStaffID    string

	actor        Actor
	scope        Scope
	foreignActor Actor
	foreignScope Scope
}

func (f s15Fixture) receivingActor() GoodsReceivingActor {
	return GoodsReceivingActor{
		StaffID:       f.staffID,
		BusinessID:    f.businessID,
		SessionID:     "s15d-session",
		CorrelationID: "s15d-corr",
		CommandRef:    "cmd-" + uuid.NewString(),
	}
}

func (f s15Fixture) foreignReceivingActor() GoodsReceivingActor {
	return GoodsReceivingActor{
		StaffID:       f.foreignStaffID,
		BusinessID:    f.foreignBusinessID,
		SessionID:     "s15d-foreign",
		CorrelationID: "s15d-foreign",
		CommandRef:    "cmd-" + uuid.NewString(),
	}
}

func newS15Fixture(t *testing.T) s15Fixture {
	t.Helper()

	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping Section 15D certification")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if err := database.ApplyMigrations(ctx, pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}

	f := s15Fixture{pool: pool}

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]

	f.businessID, f.branchID, f.staffID = seedTenant(t, pool, "a"+suffix)
	f.foreignBusinessID, f.foreignBranchID, f.foreignStaffID = seedTenant(t, pool, "b"+suffix)

	f.actor = Actor{
		StaffID:       f.staffID,
		VendorID:      f.businessID,
		SessionID:     "s15d",
		CorrelationID: "s15d",
		CommandRef:    "cmd-" + uuid.NewString(),
	}
	f.scope = Scope{Global: true}

	f.foreignActor = Actor{
		StaffID:       f.foreignStaffID,
		VendorID:      f.foreignBusinessID,
		SessionID:     "s15d",
		CorrelationID: "s15d",
		CommandRef:    "cmd-" + uuid.NewString(),
	}
	f.foreignScope = Scope{Global: true}

	invFlags := inventory.Flags{Enabled: true, WritesEnabled: true}
	f.stock = inventory.NewStockService(inventory.StockOptions{
		DB:           pool,
		Foundation:   inventory.NewService(inventory.Dependencies{Flags: invFlags}),
		Transactions: inventory.NewPostgresTransactionBoundary(pool),
		Evidence:     noopInventoryEvidence{},
		Events:       noopInventoryEvents{},
	})

	// A permissive scope resolver. IAM itself is certified separately, by
	// driving the real evaluator; these gates are about stock and evidence, and
	// a resolver that refused everything would only prove the guard fires.
	f.suppliers = NewService(Options{
		DB:            pool,
		Scopes:        grantAllScopes{},
		Enabled:       true,
		WritesEnabled: true,
	})

	f.orders = NewPurchaseOrderService(f.suppliers, nil)
	f.receiving = NewGoodsReceivingService(pool, f.orders, f.stock, noopReceivingEvents{})

	// A purchase order cannot be submitted without an active approval policy,
	// so both tenants get a single permissive rule. Threshold behaviour has its
	// own gate, which configures its own rules.
	for _, actor := range []Actor{f.actor, f.foreignActor} {
		policyActor := actor
		policyActor.CommandRef = "cmd-" + uuid.NewString()

		if _, err := f.orders.ReplaceApprovalPolicy(ctx, policyActor,
			ReplaceApprovalPolicyInput{
				Name:              "Section 15D baseline",
				AllowSelfApproval: true,
				Rules: []ApprovalRuleInput{{
					MinimumAmountMinor:     0,
					RequiredApprovalCount:  1,
					ApprovalScope:          "BRANCH_OR_BUSINESS",
					RequiredPermissionCode: PermissionApprove,
				}},
			}); err != nil {
			t.Fatalf("configure approval policy: %v", err)
		}
	}

	return f
}

// grantAllScopes grants business-wide purchasing scope to every caller.
type grantAllScopes struct{}

func (grantAllScopes) ResolveScope(
	_ context.Context, _ string, _ string,
) (Scope, error) {
	return Scope{Global: true}, nil
}

// branchOnlyScopes grants exactly one branch, for branch-isolation gates.
type branchOnlyScopes struct{ branchID string }

func (b branchOnlyScopes) ResolveScope(
	_ context.Context, _ string, _ string,
) (Scope, error) {
	return Scope{BranchIDs: []string{b.branchID}}, nil
}

type noopInventoryEvidence struct{}

func (noopInventoryEvidence) RecordInventoryEvidence(
	context.Context, inventory.InventoryTransaction, inventory.Evidence,
) error {
	return nil
}

type noopInventoryEvents struct{}

func (noopInventoryEvents) PublishInventoryEvent(
	context.Context, inventory.InventoryTransaction, inventory.Event,
) error {
	return nil
}

// seedTenant creates a business, one branch and one staff member.
func seedTenant(
	t *testing.T,
	pool *pgxpool.Pool,
	tag string,
) (string, string, string) {
	t.Helper()
	ctx := context.Background()

	user := "s15_user_" + tag
	staff := "s15_staff_" + tag
	business := "s15_biz_" + tag
	branch := "s15_branch_" + tag

	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO vendor_users(id,email) VALUES($1,$2)`,
			[]any{user, user + "@example.test"}},
		{`INSERT INTO vendor_staff(id,vendor_id,user_id) VALUES($1,$2,$3)`,
			[]any{staff, business, user}},
		{`INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone)
		   VALUES($1,'Section 15D','ACTIVE',$2,'NGN','Africa/Lagos')`,
			[]any{business, staff}},
		{`INSERT INTO vendor_branches(id,business_id,display_name,address_line1,city,region,country_code,latitude,longitude,timezone,status)
		   VALUES($1,$2,'S15D Branch','1 Test Street','Lagos','Lagos','NG',6.5244,3.3792,'Africa/Lagos','ACTIVE')`,
			[]any{branch, business}},
	} {
		if _, err := pool.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatal(err)
		}
	}

	return business, branch, staff
}

// inventoryItem creates a generic inventory item owned by one business.
//
// INGREDIENT avoids the SHOP_VARIANT ownership trigger, which would require a
// whole Shop catalogue. Purchasing posts into generic Inventory by item id;
// which source type produced the item is not what these gates are about.
func (f s15Fixture) inventoryItem(
	t *testing.T,
	businessID string,
	branchID string,
	tracking string,
) string {
	t.Helper()

	id := "itm_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	lotTracked := tracking == "LOT" || tracking == "EXPIRY"
	policy := "FIFO"
	if tracking == "EXPIRY" {
		policy = "FEFO"
	}

	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO inventory_items(
			id,business_id,branch_id,item_type,source_ref,
			stock_measure_ref,stock_measure_owner_key,stock_unit_kind,
			stock_basis_mode,tracking_mode,lot_tracking_enabled,lot_policy)
		 VALUES($1,$2,$3,'INGREDIENT',$4,'each','shop:each','COUNT','SAME_UNIT',$5,$6,$7)`,
		id, businessID, branchID, "src_"+id, tracking, lotTracked, policy); err != nil {
		t.Fatal(err)
	}

	return id
}

// supplierWithProduct creates a supplier and one mapped supplier product.
func (f s15Fixture) supplierWithProduct(
	t *testing.T,
	actor Actor,
	scope Scope,
	branchID string,
	inventoryItemID string,
	unitsPerPurchaseUnit int64,
	typicalCostMinor int64,
) (Supplier, SupplierProduct) {
	t.Helper()
	ctx := context.Background()

	code := "SUP" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:8])

	supplier, err := f.suppliers.CreateSupplier(ctx, actor, scope, CreateSupplierInput{
		Code:                 code,
		Name:                 "Section 15D Supplier",
		LegalName:            "Section 15D Supplier Ltd",
		DefaultLeadTimeHours: 48,
		Status:               "ACTIVE",
	})
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}

	cost := typicalCostMinor

	product, err := f.suppliers.CreateSupplierProduct(ctx, actor, scope, supplier.ID,
		CreateSupplierProductInput{
			BranchID:                      branchID,
			InventoryItemID:               inventoryItemID,
			SupplierSKU:                   "SKU-" + code,
			PurchaseUnitCode:              "CASE",
			PurchaseUnitName:              "Case",
			ConversionMode:                conversionModeFor(unitsPerPurchaseUnit),
			InventoryUnitsPerPurchaseUnit: unitsPerPurchaseUnit,
			MinimumOrderQuantity:          1,
			PurchaseMultiple:              1,
			TypicalCostMinor:              &cost,
			CurrencyCode:                  "NGN",
		})
	if err != nil {
		t.Fatalf("create supplier product: %v", err)
	}

	return supplier, product
}

// conversionModeFor picks the mode the schema actually accepts. One inventory
// unit per purchase unit is SAME_AS_INVENTORY; anything else must be an exact
// integer multiple, because approximate purchase conversion is never allowed.
func conversionModeFor(unitsPerPurchaseUnit int64) string {
	if unitsPerPurchaseUnit == 1 {
		return ConversionSameAsInventory
	}
	return ConversionExactInteger
}

// orderedPO drives a purchase order all the way to ORDERED.
func (f s15Fixture) orderedPO(
	t *testing.T,
	supplier Supplier,
	product SupplierProduct,
	purchaseQuantity int64,
	expectedDelivery *time.Time,
) PurchaseOrder {
	t.Helper()
	ctx := context.Background()

	actor := f.actor
	actor.CommandRef = "cmd-" + uuid.NewString()

	po, err := f.orders.CreatePurchaseOrder(ctx, actor, CreatePurchaseOrderInput{
		BranchID:             f.branchID,
		SupplierID:           supplier.ID,
		CurrencyCode:         "NGN",
		ExpectedDeliveryDate: expectedDelivery,
		Lines: []PurchaseOrderLineInput{
			{SupplierProductID: product.ID, PurchaseQuantity: purchaseQuantity},
		},
	})
	if err != nil {
		t.Fatalf("create purchase order: %v", err)
	}

	for _, step := range []struct {
		name string
		run  func(a Actor) (PurchaseOrder, error)
	}{
		{"submit", func(a Actor) (PurchaseOrder, error) {
			return f.orders.SubmitPurchaseOrder(ctx, a, po.ID,
				PurchaseOrderTransitionInput{ExpectedVersion: po.Version})
		}},
		{"approve", func(a Actor) (PurchaseOrder, error) {
			return f.orders.ApprovePurchaseOrder(ctx, a, po.ID,
				PurchaseOrderApprovalInput{ExpectedVersion: po.Version})
		}},
		{"order", func(a Actor) (PurchaseOrder, error) {
			return f.orders.MarkPurchaseOrderOrdered(ctx, a, po.ID,
				PurchaseOrderTransitionInput{ExpectedVersion: po.Version})
		}},
	} {
		stepActor := f.actor
		stepActor.CommandRef = "cmd-" + uuid.NewString()

		updated, err := step.run(stepActor)
		if err != nil {
			t.Fatalf("%s purchase order: %v", step.name, err)
		}
		po = updated
	}

	if po.Status != "ORDERED" {
		t.Fatalf("purchase order reached %q, want ORDERED", po.Status)
	}

	return po
}

// balanceOf reads the authoritative generic Inventory on-hand quantity.
func (f s15Fixture) balanceOf(t *testing.T, itemID string) int64 {
	t.Helper()

	var onHand int64
	err := f.pool.QueryRow(context.Background(),
		`SELECT coalesce(on_hand_quantity, 0)
		   FROM inventory_balances
		  WHERE inventory_item_id = $1`, itemID).Scan(&onHand)

	if err != nil {
		// No balance row yet means no stock has ever been posted.
		return 0
	}
	return onHand
}

// receiveOnce runs create -> inspect -> confirm for one line.
func (f s15Fixture) receiveOnce(
	t *testing.T,
	po PurchaseOrder,
	line InspectGoodsReceiptLineInput,
) (GoodsReceipt, ConfirmGoodsReceiptResult) {
	t.Helper()
	ctx := context.Background()

	receipt, err := f.receiving.CreateGoodsReceipt(ctx, f.receivingActor(),
		CreateGoodsReceiptInput{
			PurchaseOrderID: po.ID,
			ReceivedAt:      time.Now().UTC(),
		})
	if err != nil {
		t.Fatalf("create goods receipt: %v", err)
	}

	if line.PurchaseOrderLineID == "" && len(po.Lines) > 0 {
		line.PurchaseOrderLineID = po.Lines[0].ID
	}

	inspected, err := f.receiving.InspectGoodsReceipt(ctx, f.receivingActor(),
		receipt.ID, receipt.Version,
		InspectGoodsReceiptInput{Lines: []InspectGoodsReceiptLineInput{line}})
	if err != nil {
		t.Fatalf("inspect goods receipt: %v", err)
	}

	result, err := f.receiving.ConfirmGoodsReceipt(ctx, f.receivingActor(),
		inspected.ID, inspected.Version)
	if err != nil {
		t.Fatalf("confirm goods receipt: %v", err)
	}

	return inspected, result
}

// Helpers for driving generic Inventory directly from purchasing gates.

func inventoryActorFor(staffID, businessID string) inventory.Actor {
	return inventory.Actor{
		ActorRef:      staffID,
		StaffID:       staffID,
		BusinessID:    businessID,
		CorrelationID: "s15d",
	}
}

func inventoryBranchScope(businessID, branchID string) (inventory.BranchScope, error) {
	return inventory.ExactBranchScope(businessID, branchID)
}

func inventoryAdjustInput(branchID, itemID, ref string) inventory.QuantityMutationInput {
	return inventory.QuantityMutationInput{
		BranchID:        branchID,
		InventoryItemID: itemID,
		SourceType:      "INTERNAL",
		SourceRef:       ref + "-" + uuid.NewString(),
		ReasonCode:      "CONTROLLED_CORRECTION",
	}
}
