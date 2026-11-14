package integration_test

import (
	"context"
	"testing"
	"time"

	bulkusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/usecases"
	manifestusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/usecases"
	ordersusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/usecases"
	packagesusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/usecases"
	recurringmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/models"
	recurringusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/usecases"
	catalogmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/models"
	catalogusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/usecases"
	shipmentsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Section 15 requirement rows that previously had no PostgreSQL test: the
// catalogue, order, shipment, package, manifest and import read surfaces,
// catalogue PATCH and the recurring-order template commands. These drive the
// real use cases against PostgreSQL; route permissions and OpenAPI parity are
// covered by the Section 15 contract tests.

// TestRow15_1b_ServiceCatalogListGetPatch covers FLEET-15.1b:
// GET /service-catalog, GET/PATCH /service-catalog/:id.
func TestRow15_1b_ServiceCatalogListGetPatch(t *testing.T) {
	f := newShipmentsFixture(t)
	ctx := context.Background()
	active := f.activeCatalogItem(t, "ROWS15_ACTIVE")
	draft, err := f.catalog.Create(ctx, f.tenant, catalogusecases.CreateInput{Name: "Freight lane", Code: "ROWS15_DRAFT", Category: catalogmodels.CategoryPackageDelivery, CapabilityReference: "cap_draft"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := f.catalog.Create(ctx, f.other, catalogusecases.CreateInput{Name: "Foreign", Code: "ROWS15_DRAFT", Category: catalogmodels.CategoryPackageDelivery})
	if err != nil {
		t.Fatal(err)
	}
	all, _, err := f.catalog.List(ctx, f.tenant, catalogusecases.ListInput{Limit: 50})
	ids := map[string]bool{}
	for _, item := range all {
		ids[item.ID] = true
	}
	if err != nil || len(all) != 2 || !ids[active.ID] || !ids[draft.ID] || ids[foreign.ID] {
		t.Fatalf("catalogue: %v %v", ids, err)
	}
	drafts, _, err := f.catalog.List(ctx, f.tenant, catalogusecases.ListInput{Status: catalogmodels.StatusDraft, Limit: 50})
	if err != nil || len(drafts) != 1 || drafts[0].ID != draft.ID {
		t.Fatalf("status filter: %+v %v", drafts, err)
	}
	page, next, err := f.catalog.List(ctx, f.tenant, catalogusecases.ListInput{Limit: 1})
	if err != nil || len(page) != 1 || next == "" {
		t.Fatalf("first page: %d %q %v", len(page), next, err)
	}
	rest, _, err := f.catalog.List(ctx, f.tenant, catalogusecases.ListInput{Limit: 1, Cursor: next})
	if err != nil || len(rest) != 1 || rest[0].ID == page[0].ID {
		t.Fatalf("second page: %+v %v", rest, err)
	}
	if _, _, err = f.catalog.List(ctx, f.tenant, catalogusecases.ListInput{Cursor: "%%%not-a-cursor"}); err == nil {
		t.Fatal("a malformed cursor must be refused")
	}
	got, err := f.catalog.Get(ctx, f.tenant, active.ID)
	if err != nil || got.Status != catalogmodels.StatusActive || got.Code != "ROWS15_ACTIVE" {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err = f.catalog.Get(ctx, f.tenant, foreign.ID); err == nil {
		t.Fatal("cross-tenant catalogue read must fail")
	}

	if _, err = f.catalog.Update(ctx, f.tenant, draft.ID, 0, catalogusecases.UpdateInput{Name: "Freight lane v2"}); err == nil {
		t.Fatal("update without expectedVersion must be refused")
	}
	if _, err = f.catalog.Update(ctx, f.tenant, draft.ID, 99, catalogusecases.UpdateInput{Name: "Freight lane v2"}); err == nil {
		t.Fatal("stale catalogue version must conflict")
	}
	if _, err = f.catalog.Update(ctx, f.other, draft.ID, draft.Version, catalogusecases.UpdateInput{Name: "x"}); err == nil {
		t.Fatal("cross-tenant catalogue update must fail")
	}
	updated, err := f.catalog.Update(ctx, f.tenant, draft.ID, draft.Version, catalogusecases.UpdateInput{Name: "Freight lane v2", Description: "Dedicated freight", SupportedVehicleClasses: []string{"TRUCK_20T"}})
	if err != nil || updated.Name != "Freight lane v2" || updated.Version != draft.Version+1 || len(updated.SupportedVehicleClasses) != 1 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	// The immutable code and category do not move on a PATCH.
	if updated.Code != draft.Code || updated.Category != draft.Category {
		t.Fatalf("PATCH changed identity fields: %+v", updated)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND resource_id=$2`, f.tenant.OrganizationID, draft.ID); n < 2 {
		t.Fatalf("catalogue audit records = %d", n)
	}
}

// TestRow15_2b_OrderListGetStops covers FLEET-15.2b: GET /orders,
// GET /orders/:id and GET /orders/:id/stops.
func TestRow15_2b_OrderListGetStops(t *testing.T) {
	f := newShipmentsFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Orders Reads Co")
	other := f.customer(t, "Orders Reads Two Co")
	item := f.activeCatalogItem(t, "ROWS15_ORDERS")
	confirmed, pickupID, deliveryID := f.confirmedOrderWithStops(t, customer.ID, item.ID)
	draft, err := f.orders.Create(ctx, f.tenant, ordersusecases.CreateInput{CustomerID: other.ID, ServiceCatalogItemID: item.ID, Timezone: "Africa/Lagos"})
	if err != nil {
		t.Fatal(err)
	}

	all, _, err := f.orders.List(ctx, f.tenant, ordersusecases.ListInput{Limit: 50})
	ids := map[string]bool{}
	for _, o := range all {
		ids[o.ID] = true
	}
	if err != nil || len(all) != 2 || !ids[confirmed.ID] || !ids[draft.ID] {
		t.Fatalf("orders: %v %v", ids, err)
	}
	byCustomer, _, err := f.orders.List(ctx, f.tenant, ordersusecases.ListInput{CustomerID: other.ID, Limit: 50})
	if err != nil || len(byCustomer) != 1 || byCustomer[0].ID != draft.ID {
		t.Fatalf("customer filter: %+v %v", byCustomer, err)
	}
	byStatus, _, err := f.orders.List(ctx, f.tenant, ordersusecases.ListInput{Status: confirmed.Status, Limit: 50})
	if err != nil || len(byStatus) != 1 || byStatus[0].ID != confirmed.ID {
		t.Fatalf("status filter: %+v %v", byStatus, err)
	}
	if foreign, _, err := f.orders.List(ctx, f.other, ordersusecases.ListInput{Limit: 50}); err != nil || len(foreign) != 0 {
		t.Fatalf("other org orders: %+v %v", foreign, err)
	}
	got, err := f.orders.Get(ctx, f.tenant, confirmed.ID)
	if err != nil || got.CustomerID != customer.ID || got.ConfirmedAt == nil || got.OrderNumber == "" {
		t.Fatalf("get order: %+v %v", got, err)
	}
	if _, err = f.orders.Get(ctx, f.other, confirmed.ID); err == nil {
		t.Fatal("cross-tenant order read must fail")
	}
	stops, err := f.orders.ListStops(ctx, f.tenant, confirmed.ID)
	if err != nil || len(stops) != 2 || stops[0].ID != pickupID || stops[1].ID != deliveryID || stops[0].Sequence >= stops[1].Sequence {
		t.Fatalf("order stops in sequence: %+v %v", stops, err)
	}
	if _, err = f.orders.ListStops(ctx, f.other, confirmed.ID); err == nil {
		t.Fatal("cross-tenant stop list must fail")
	}
}

// TestRow15_5b_ShipmentListGet covers FLEET-15.5b: GET /shipments and
// GET /shipments/:id.
func TestRow15_5b_ShipmentListGet(t *testing.T) {
	f := newShipmentsFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Shipment Reads Co")
	item := f.activeCatalogItem(t, "ROWS15_SHIPMENTS")
	first, pickupA, deliveryA := f.confirmedOrderWithStops(t, customer.ID, item.ID)
	second, pickupB, deliveryB := f.confirmedOrderWithStops(t, customer.ID, item.ID)
	a, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{OrderID: first.ID, OriginStopID: pickupA, DestinationStopID: deliveryA, DeclaredValueCurrency: "NGN"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{OrderID: second.ID, OriginStopID: pickupB, DestinationStopID: deliveryB, DeclaredValueCurrency: "NGN"})
	if err != nil {
		t.Fatal(err)
	}
	all, _, err := f.shipments.List(ctx, f.tenant, shipmentsusecases.ListInput{Limit: 50})
	if err != nil || len(all) != 2 {
		t.Fatalf("shipments: %+v %v", all, err)
	}
	byOrder, _, err := f.shipments.List(ctx, f.tenant, shipmentsusecases.ListInput{OrderID: second.ID, Limit: 50})
	if err != nil || len(byOrder) != 1 || byOrder[0].ID != b.ID {
		t.Fatalf("order filter: %+v %v", byOrder, err)
	}
	if foreign, _, err := f.shipments.List(ctx, f.other, shipmentsusecases.ListInput{Limit: 50}); err != nil || len(foreign) != 0 {
		t.Fatalf("other org shipments: %+v %v", foreign, err)
	}
	got, err := f.shipments.Get(ctx, f.tenant, a.ID)
	if err != nil || got.OrderID != first.ID || got.OriginStopID != pickupA || got.DestinationStopID != deliveryA || got.DeclaredValueCurrency != "NGN" {
		t.Fatalf("get shipment: %+v %v", got, err)
	}
	if _, err = f.shipments.Get(ctx, f.other, a.ID); err == nil {
		t.Fatal("cross-tenant shipment read must fail")
	}
	confined := f.tenant
	confined.PermittedBranchIDs = []string{"brn_elsewhere"}
	if got.BranchID != "" {
		if _, err = f.shipments.Get(ctx, confined, a.ID); err == nil {
			t.Fatal("cross-branch shipment read must fail")
		}
	}
}

// TestRow15_6e_PackageReads covers FLEET-15.6e: GET /packages/:id/custody,
// /history, GET /packages/:id and GET /shipments/:id/packages.
func TestRow15_6e_PackageReads(t *testing.T) {
	f := newPackagesMatrixFixture(t)
	ctx := context.Background()
	first := f.createPackage(t)
	second := f.createPackage(t)
	now := time.Now().UTC().Truncate(time.Second)

	listed, err := f.service.List(ctx, f.tenant, f.shipmentID)
	if err != nil || len(listed) != 2 {
		t.Fatalf("shipment packages: %+v %v", listed, err)
	}
	if _, err = f.service.List(ctx, f.other, f.shipmentID); err == nil {
		t.Fatal("cross-tenant package list must fail")
	}
	got, err := f.service.Get(ctx, f.tenant, first.ID)
	if err != nil || got.ShipmentID != f.shipmentID || got.PackageCode == "" || got.PackageCode == second.PackageCode {
		t.Fatalf("get package: %+v %v", got, err)
	}
	if _, err = f.service.Get(ctx, f.other, first.ID); err == nil {
		t.Fatal("cross-tenant package read must fail")
	}

	if _, err = f.service.RecordCustody(ctx, f.tenant, first.ID, packagesusecases.CustodyInput{ToActorType: "WAREHOUSE", ToActorReference: "wh-1",
		Condition: "GOOD", OccurredAt: now.Add(-time.Hour), EvidenceMediaID: "media-owned", Source: "TEST"}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.RecordCustody(ctx, f.tenant, first.ID, packagesusecases.CustodyInput{FromActorType: "WAREHOUSE", FromActorReference: "wh-1",
		ToActorType: "DRIVER", ToActorReference: "drv-1", Condition: "GOOD", OccurredAt: now, EvidenceMediaID: "media-owned", Source: "TEST"}); err != nil {
		t.Fatal(err)
	}
	chain, err := f.service.Custody(ctx, f.tenant, first.ID)
	if err != nil || len(chain) != 2 || chain[0].ToActorReference != "wh-1" || chain[1].ToActorReference != "drv-1" || chain[1].FromActorReference != "wh-1" {
		t.Fatalf("custody chain in order: %+v %v", chain, err)
	}
	if other, err := f.service.Custody(ctx, f.tenant, second.ID); err != nil || len(other) != 0 {
		t.Fatalf("custody of another package leaked: %+v %v", other, err)
	}
	if _, err = f.service.Custody(ctx, f.other, first.ID); err == nil {
		t.Fatal("cross-tenant custody read must fail")
	}
	history, err := f.service.History(ctx, f.tenant, first.ID)
	if err != nil || len(history) == 0 || history[0].PackageID != first.ID {
		t.Fatalf("package history: %+v %v", history, err)
	}
	if _, err = f.service.History(ctx, f.other, first.ID); err == nil {
		t.Fatal("cross-tenant history read must fail")
	}
}

// TestRow15_7b_ManifestListGet covers FLEET-15.7b: GET /manifests and
// GET /manifests/:id.
func TestRow15_7b_ManifestListGet(t *testing.T) {
	f := newManifestMatrixFixture(t, manifestAuthorityStub{valid: true})
	ctx := context.Background()
	withItem := f.manifestWithItem(t, "r15", "", "")
	empty, err := f.service.Create(ctx, f.tenant, manifestusecases.CreateInput{BranchID: f.branchID, RouteReference: "route-empty", SealNumber: "SEAL-empty"})
	if err != nil {
		t.Fatal(err)
	}
	all, err := f.service.List(ctx, f.tenant, "", "")
	if err != nil || len(all) != 2 {
		t.Fatalf("manifests: %+v %v", all, err)
	}
	byBranch, err := f.service.List(ctx, f.tenant, "", f.branchID)
	if err != nil || len(byBranch) != 2 {
		t.Fatalf("branch filter: %+v %v", byBranch, err)
	}
	byStatus, err := f.service.List(ctx, f.tenant, "CLOSED", "")
	if err != nil || len(byStatus) != 0 {
		t.Fatalf("status filter: %+v %v", byStatus, err)
	}
	if foreign, err := f.service.List(ctx, f.other, "", ""); err != nil || len(foreign) != 0 {
		t.Fatalf("other org manifests: %+v %v", foreign, err)
	}
	m, items, err := f.service.Get(ctx, f.tenant, withItem.ID)
	if err != nil || m.RouteReference != "route-r15" || len(items) != 1 || items[0].ItemID != f.shipmentID {
		t.Fatalf("get manifest: %+v %+v %v", m, items, err)
	}
	if _, items, err = f.service.Get(ctx, f.tenant, empty.ID); err != nil || len(items) != 0 {
		t.Fatalf("empty manifest items: %+v %v", items, err)
	}
	if _, _, err = f.service.Get(ctx, f.other, withItem.ID); err == nil {
		t.Fatal("cross-tenant manifest read must fail")
	}
	confined := f.tenant
	confined.PermittedBranchIDs = []string{"brn_not_this_one"}
	if _, _, err = f.service.Get(ctx, confined, withItem.ID); err == nil {
		t.Fatal("cross-branch manifest read must fail")
	}
	if c, err := f.service.List(ctx, confined, "", ""); err != nil || len(c) != 0 {
		t.Fatalf("branch-confined manifests: %+v %v", c, err)
	}
}

// TestRow15_8c_OrderImportListGet covers FLEET-15.8c: GET /orders/imports and
// GET /orders/imports/:id.
func TestRow15_8c_OrderImportListGet(t *testing.T) {
	f, imp := newBulkFixture(t, bulkCSV, 10)
	ctx := context.Background()
	all, err := f.service.List(ctx, f.tenant)
	if err != nil || len(all) != 1 || all[0].ID != imp.ID || all[0].FileFormat != "CSV" {
		t.Fatalf("imports: %+v %v", all, err)
	}
	got, rows, err := f.service.Get(ctx, f.tenant, imp.ID)
	if err != nil || got.ID != imp.ID || got.MediaFileID != "media_bulk_matrix" || len(rows) != 0 {
		t.Fatalf("import before validation: %+v rows=%d %v", got, len(rows), err)
	}
	if _, err = f.service.RunWorkerPass(ctx, bulkusecases.WorkerConfig{Owner: "rows15", Lease: time.Second, BatchSize: 1, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	got, rows, err = f.service.Get(ctx, f.tenant, imp.ID)
	if err != nil || len(rows) != 2 || got.TotalRows != 2 {
		t.Fatalf("import after validation: %+v rows=%d %v", got, len(rows), err)
	}
	other := createOrganization(t, f.pool, "Bulk Rows15 Other", "RC-"+ids.New("rows"))
	stranger := tenant.Context{OrganizationID: other.ID, OrganizationUserID: "usr_bulk_other", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "usr_bulk_other"}
	if foreign, err := f.service.List(ctx, stranger); err != nil || len(foreign) != 0 {
		t.Fatalf("other org imports: %+v %v", foreign, err)
	}
	if _, _, err = f.service.Get(ctx, stranger, imp.ID); err == nil {
		t.Fatal("cross-tenant import read must fail")
	}
	confined := f.tenant
	confined.PermittedBranchIDs = []string{"brn_not_this_one"}
	if _, _, err = f.service.Get(ctx, confined, imp.ID); err == nil {
		t.Fatal("cross-branch import read must fail")
	}
}

// TestRow15_9b_RecurringTemplateCommandsAndReads covers FLEET-15.9b:
// POST /recurring-orders, .../activate, /deactivate, GET /recurring-orders
// and GET /recurring-orders/:id.
func TestRow15_9b_RecurringTemplateCommandsAndReads(t *testing.T) {
	flags := recurringusecases.Flags{Enabled: true, WritesEnabled: true}
	f, active := newRecurringMatrixFixture(t, &recurringOrdersStub{}, flags)
	ctx := context.Background()
	branch := f.tenant.PermittedBranchIDs[0]
	base := recurringusecases.CreateInput{BranchID: branch, CustomerID: "cus_r15", ServiceCatalogItemID: "sci_r15",
		RecurrenceType: recurringmodels.TypeCustom, RRule: "FREQ=WEEKLY;INTERVAL=1", Timezone: "Europe/Rome", StartDate: time.Now().Add(time.Hour)}

	bad := base
	bad.RRule = "NOT A RULE"
	if _, err := f.service.Create(ctx, f.tenant, bad); err == nil {
		t.Fatal("an invalid recurrence rule must be refused")
	}
	badTZ := base
	badTZ.Timezone = "Mars/Olympus"
	if _, err := f.service.Create(ctx, f.tenant, badTZ); err == nil {
		t.Fatal("an invalid timezone must be refused")
	}
	elsewhere := base
	elsewhere.BranchID = "brn_outside_scope"
	if _, err := f.service.Create(ctx, f.tenant, elsewhere); err == nil {
		t.Fatal("a template outside the caller's branches must be refused")
	}
	created, err := f.service.Create(ctx, f.tenant, base)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	if created.Status == recurringmodels.StatusActive || created.NormalizedRRule == "" {
		t.Fatalf("new template must start inactive with a normalized rule: %+v", created)
	}

	all, err := f.service.List(ctx, f.tenant, "")
	if err != nil || len(all) != 2 {
		t.Fatalf("templates: %+v %v", all, err)
	}
	activeOnly, err := f.service.List(ctx, f.tenant, recurringmodels.StatusActive)
	if err != nil || len(activeOnly) != 1 || activeOnly[0].ID != active.ID {
		t.Fatalf("status filter: %+v %v", activeOnly, err)
	}
	got, err := f.service.Get(ctx, f.tenant, created.ID)
	if err != nil || got.CustomerID != "cus_r15" || got.Timezone != "Europe/Rome" {
		t.Fatalf("get template: %+v %v", got, err)
	}
	other := createOrganization(t, f.pool, "Recurring Rows15 Other", "RC-"+ids.New("rows"))
	stranger := tenant.Context{OrganizationID: other.ID, OrganizationUserID: "usr_rec_other", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "usr_rec_other"}
	if _, err = f.service.Get(ctx, stranger, created.ID); err == nil {
		t.Fatal("cross-tenant template read must fail")
	}
	if foreign, err := f.service.List(ctx, stranger, ""); err != nil || len(foreign) != 0 {
		t.Fatalf("other org templates: %+v %v", foreign, err)
	}
	if _, err = f.service.SetActive(ctx, stranger, created.ID, created.Version, true); err == nil {
		t.Fatal("cross-tenant activation must fail")
	}
	if _, err = f.service.SetActive(ctx, f.tenant, created.ID, 99, true); err == nil {
		t.Fatal("stale template version must conflict")
	}
	activated, err := f.service.SetActive(ctx, f.tenant, created.ID, created.Version, true)
	if err != nil || activated.Status != recurringmodels.StatusActive || activated.ActivatedAt == nil {
		t.Fatalf("activate: %+v %v", activated, err)
	}
	deactivated, err := f.service.SetActive(ctx, f.tenant, created.ID, activated.Version, false)
	if err != nil || deactivated.Status == recurringmodels.StatusActive {
		t.Fatalf("deactivate: %+v %v", deactivated, err)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND resource_id=$2`, f.tenant.OrganizationID, created.ID); n < 3 {
		t.Fatalf("template audit records (create, activate, deactivate) = %d", n)
	}
}
