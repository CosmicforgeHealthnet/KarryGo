package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	crmmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	crmrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	crmusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/usecases"
	ordersmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	ordersrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/repositories"
	ordersusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/usecases"
	catalogmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/models"
	catalogrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/repositories"
	catalogusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/usecases"
	shipmentsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/models"
	shipmentsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/repositories"
	shipmentsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ---------------------------------------------------------------------------
// Fixture
// ---------------------------------------------------------------------------

type shipmentsFixture struct {
	pool      *pgxpool.Pool
	catalog   *catalogusecases.Service
	orders    *ordersusecases.Service
	shipments *shipmentsusecases.Service
	crm       *crmusecases.Service
	tenant    tenant.Context
	other     tenant.Context
}

func allShipmentsFlagsOn() shipmentsusecases.Flags {
	return shipmentsusecases.Flags{Enabled: true, WritesEnabled: true}
}

func newShipmentsFixture(t *testing.T) *shipmentsFixture {
	t.Helper()
	return newShipmentsFixtureOnPool(t, testPool(t))
}

func newShipmentsFixtureOnPool(t *testing.T, pool *pgxpool.Pool) *shipmentsFixture {
	t.Helper()
	primary := createOrganization(t, pool, "Shipments Primary Fleet", "RC-SHP-1")
	secondary := createOrganization(t, pool, "Shipments Secondary Fleet", "RC-SHP-2")

	metrics := observability.NewMetrics()
	crmRepo := crmrepos.NewRepository(pool)
	crmService := crmusecases.NewService(crmusecases.Options{
		Pool: pool, Repository: crmRepo, Metrics: metrics, Flags: allCRMFlagsOn(),
		TokenSecret:   []byte("shipments-integration-token-secret"),
		ContactSecret: []byte("shipments-integration-contact-secret"),
		TokenTTL:      48 * time.Hour,
	})

	catalogRepo := catalogrepos.NewRepository(pool)
	catalogService := catalogusecases.NewService(catalogusecases.Options{
		Pool: pool, Repository: catalogRepo, Metrics: metrics, Flags: allCatalogFlagsOn(),
	})

	ordersRepo := ordersrepos.NewRepository(pool)
	ordersService := ordersusecases.NewService(ordersusecases.Options{
		Pool: pool, Repository: ordersRepo,
		Catalog: app.NewOrdersCatalogAdapter(catalogRepo), CRM: app.NewOrdersCRMAdapter(crmRepo),
		Metrics: metrics, Flags: allOrdersFlagsOn(),
	})

	shipmentsRepo := shipmentsrepos.NewRepository(pool)
	shipmentsService := shipmentsusecases.NewService(shipmentsusecases.Options{
		Pool: pool, Repository: shipmentsRepo, Orders: app.NewShipmentsOrdersAdapter(ordersRepo),
		Metrics: metrics, Flags: allShipmentsFlagsOn(),
	})

	return &shipmentsFixture{
		pool: pool, catalog: catalogService, orders: ordersService, shipments: shipmentsService, crm: crmService,
		tenant: tenant.Context{OrganizationID: primary.ID, OrganizationUserID: "usr_shipments_primary",
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: "usr_shipments_primary"},
		other: tenant.Context{OrganizationID: secondary.ID, OrganizationUserID: "usr_shipments_other",
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: "usr_shipments_other"},
	}
}

func (f *shipmentsFixture) customer(t *testing.T, name string) crmmodels.Customer {
	t.Helper()
	customer, err := f.crm.CreateCustomer(context.Background(), f.tenant,
		crmusecases.CreateCustomerInput{
			CustomerType: crmmodels.CustomerTypeBusiness, LegalName: name,
			RegistrationNumber: "RC-" + name, TaxReference: "TIN-" + name,
			DefaultCurrency: "NGN", IdempotencyKey: "create:" + name,
		})
	if err != nil {
		t.Fatalf("create customer %q: %v", name, err)
	}
	return customer
}

func (f *shipmentsFixture) activeCatalogItem(t *testing.T, code string) catalogmodels.CatalogItem {
	t.Helper()
	ctx := context.Background()
	item, err := f.catalog.Create(ctx, f.tenant, catalogusecases.CreateInput{
		Name: "Shipments Item " + code, Code: code, Category: catalogmodels.CategoryPackageDelivery,
		CapabilityReference: "cap_" + code,
	})
	if err != nil {
		t.Fatalf("create catalog item: %v", err)
	}
	activated, err := f.catalog.Activate(ctx, f.tenant, item.ID, item.Version)
	if err != nil {
		t.Fatalf("activate catalog item: %v", err)
	}
	return activated
}

// confirmedOrderWithStops creates and confirms an order carrying a pickup and
// a delivery stop, returning the order and both stop IDs.
func (f *shipmentsFixture) confirmedOrderWithStops(t *testing.T, customerID, itemID string) (ordersmodels.Order, string, string) {
	t.Helper()
	ctx := context.Background()
	order, err := f.orders.Create(ctx, f.tenant, ordersusecases.CreateInput{
		CustomerID: customerID, ServiceCatalogItemID: itemID, Timezone: "Africa/Lagos",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	pickup, err := f.orders.AddStop(ctx, f.tenant, order.ID, ordersusecases.AddStopInput{
		StopType: ordersmodels.StopTypePickup, Timezone: "Africa/Lagos",
	})
	if err != nil {
		t.Fatalf("add pickup stop: %v", err)
	}
	delivery, err := f.orders.AddStop(ctx, f.tenant, order.ID, ordersusecases.AddStopInput{
		StopType: ordersmodels.StopTypeDelivery, Timezone: "Africa/Lagos",
	})
	if err != nil {
		t.Fatalf("add delivery stop: %v", err)
	}
	confirmed, err := f.orders.Confirm(ctx, f.tenant, order.ID, order.Version)
	if err != nil {
		t.Fatalf("confirm order: %v", err)
	}
	return confirmed, pickup.ID, delivery.ID
}

// ---------------------------------------------------------------------------
// Shipments
// ---------------------------------------------------------------------------

func TestShipments_CreateUpdateLifecycleAndIsolation(t *testing.T) {
	f := newShipmentsFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Shipment Lifecycle Co")
	item := f.activeCatalogItem(t, "SHIPMENT_LIFECYCLE_ITEM")
	order, pickupID, deliveryID := f.confirmedOrderWithStops(t, customer.ID, item.ID)

	// Origin and destination must belong to the order.
	_, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: order.ID, OriginStopID: "ost_00000000000000000000000000000000",
		DestinationStopID: deliveryID, DeclaredValueCurrency: "NGN",
	})
	if shipmentsErrorCode(err) != "shipment_origin_invalid" {
		t.Fatalf("expected shipment_origin_invalid, got %v", err)
	}

	// Origin and destination must differ.
	_, err = f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: order.ID, OriginStopID: pickupID, DestinationStopID: pickupID, DeclaredValueCurrency: "NGN",
	})
	if err == nil {
		t.Fatal("expected an error when origin equals destination")
	}

	shipment, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: order.ID, OriginStopID: pickupID, DestinationStopID: deliveryID,
		DeclaredValueAmountMinor: 500000, DeclaredValueCurrency: "NGN",
	})
	if err != nil {
		t.Fatalf("create shipment: %v", err)
	}
	if shipment.Status != shipmentsmodels.ShipmentDraft {
		t.Fatalf("expected DRAFT, got %s", shipment.Status)
	}
	if shipment.PackageCount != 0 {
		t.Fatalf("expected zero packages before the A5 slice, got %d", shipment.PackageCount)
	}

	updated, err := f.shipments.Update(ctx, f.tenant, shipment.ID, shipment.Version, shipmentsusecases.UpdateInput{
		DeclaredValueAmountMinor: 750000, DeclaredValueCurrency: "NGN",
		RequiredVehicleClass: "VAN",
	})
	if err != nil {
		t.Fatalf("update shipment: %v", err)
	}
	if updated.DeclaredValueAmountMinor != 750000 || updated.RequiredVehicleClass != "VAN" {
		t.Fatalf("update did not persist: %+v", updated)
	}

	// A shipment not yet created against an unconfirmed order is refused.
	draftOrder, err := f.orders.Create(ctx, f.tenant, ordersusecases.CreateInput{
		CustomerID: customer.ID, ServiceCatalogItemID: item.ID, Timezone: "Africa/Lagos",
	})
	if err != nil {
		t.Fatalf("create draft order: %v", err)
	}
	_, err = f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: draftOrder.ID, OriginStopID: pickupID, DestinationStopID: deliveryID, DeclaredValueCurrency: "NGN",
	})
	if err == nil {
		t.Fatal("expected an error creating a shipment against an unconfirmed order")
	}

	// Cross-tenant read is indistinguishable from not-found.
	if _, err := f.shipments.Get(ctx, f.other, shipment.ID); shipmentsErrorCode(err) != "shipment_not_found" {
		t.Fatalf("cross-tenant read must be not-found, got %v", err)
	}
}

func TestShipments_LockedAfterDraft(t *testing.T) {
	f := newShipmentsFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Locked Shipment Co")
	item := f.activeCatalogItem(t, "LOCKED_SHIPMENT_ITEM")
	order, pickupID, deliveryID := f.confirmedOrderWithStops(t, customer.ID, item.ID)

	shipment, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: order.ID, OriginStopID: pickupID, DestinationStopID: deliveryID, DeclaredValueCurrency: "NGN",
	})
	if err != nil {
		t.Fatalf("create shipment: %v", err)
	}

	// Move the shipment straight to READY at the repository level to exercise
	// the lock without needing a not-yet-built status-transition endpoint.
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_shipments SET status = 'READY' WHERE id = $1`, shipment.ID); err != nil {
		t.Fatalf("force shipment to READY: %v", err)
	}

	_, err = f.shipments.Update(ctx, f.tenant, shipment.ID, shipment.Version, shipmentsusecases.UpdateInput{
		DeclaredValueAmountMinor: 1, DeclaredValueCurrency: "NGN",
	})
	if shipmentsErrorCode(err) != "shipment_locked" {
		t.Fatalf("expected shipment_locked once past DRAFT, got %v", err)
	}

	_, err = f.shipments.AddCargoItem(ctx, f.tenant, shipment.ID, shipmentsusecases.AddCargoItemInput{
		Description: "Boxes", Quantity: "1", Unit: "PALLET", DeclaredValueCurrency: "NGN",
	})
	if err != nil {
		t.Fatalf("cargo can still be added while READY: %v", err)
	}

	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_shipments SET status = 'PLANNED' WHERE id = $1`, shipment.ID); err != nil {
		t.Fatalf("force shipment to PLANNED: %v", err)
	}
	_, err = f.shipments.AddCargoItem(ctx, f.tenant, shipment.ID, shipmentsusecases.AddCargoItemInput{
		Description: "More boxes", Quantity: "1", Unit: "PALLET", DeclaredValueCurrency: "NGN",
	})
	if shipmentsErrorCode(err) != "shipment_locked" {
		t.Fatalf("expected shipment_locked once PLANNED, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Cargo items and compliance
// ---------------------------------------------------------------------------

func TestShipments_CargoComplianceFailsClosed(t *testing.T) {
	f := newShipmentsFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Cargo Compliance Co")
	item := f.activeCatalogItem(t, "CARGO_COMPLIANCE_ITEM")
	order, pickupID, deliveryID := f.confirmedOrderWithStops(t, customer.ID, item.ID)
	shipment, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: order.ID, OriginStopID: pickupID, DestinationStopID: deliveryID, DeclaredValueCurrency: "NGN",
	})
	if err != nil {
		t.Fatalf("create shipment: %v", err)
	}

	general, err := f.shipments.AddCargoItem(ctx, f.tenant, shipment.ID, shipmentsusecases.AddCargoItemInput{
		Description: "Office supplies", Quantity: "10", Unit: "BOX",
		HandlingCategory: shipmentsmodels.HandlingGeneral, DeclaredValueCurrency: "NGN",
	})
	if err != nil {
		t.Fatalf("create general cargo: %v", err)
	}
	if general.RestrictedItemStatus != shipmentsmodels.RestrictionNotRestricted {
		t.Fatalf("general cargo must be NOT_RESTRICTED, got %s", general.RestrictedItemStatus)
	}

	dangerous, err := f.shipments.AddCargoItem(ctx, f.tenant, shipment.ID, shipmentsusecases.AddCargoItemInput{
		Description: "Lithium batteries", Quantity: "5", Unit: "CRATE",
		HandlingCategory: shipmentsmodels.HandlingDangerousGoods, DeclaredValueCurrency: "NGN",
	})
	if err != nil {
		t.Fatalf("create dangerous-goods cargo: %v", err)
	}
	// Never approved from free text or caller input alone — the server
	// computed this from the handling category, not from anything the caller
	// asserted about the item's safety.
	if dangerous.RestrictedItemStatus != shipmentsmodels.RestrictionRequiresReview {
		t.Fatalf("dangerous goods must require compliance review, got %s", dangerous.RestrictedItemStatus)
	}
	if shipmentsmodels.RestrictionBlocksShipment(general.RestrictedItemStatus) {
		t.Fatal("general cargo must not block the shipment")
	}
	if !shipmentsmodels.RestrictionBlocksShipment(dangerous.RestrictedItemStatus) {
		t.Fatal("dangerous goods pending review must block the shipment")
	}

	medical, err := f.shipments.AddCargoItem(ctx, f.tenant, shipment.ID, shipmentsusecases.AddCargoItemInput{
		Description: "Vaccines", Quantity: "2", Unit: "COOLER",
		HandlingCategory: shipmentsmodels.HandlingMedical, DeclaredValueCurrency: "NGN",
	})
	if err != nil {
		t.Fatalf("create medical cargo: %v", err)
	}
	if medical.RestrictedItemStatus != shipmentsmodels.RestrictionRequiresReview {
		t.Fatalf("medical cargo must require compliance review, got %s", medical.RestrictedItemStatus)
	}

	items, err := f.shipments.ListCargoItems(ctx, f.tenant, shipment.ID)
	if err != nil {
		t.Fatalf("list cargo items: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 cargo items, got %d", len(items))
	}
}

func TestShipments_CargoQuantityAndCoordinateValidation(t *testing.T) {
	f := newShipmentsFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Cargo Validation Co")
	item := f.activeCatalogItem(t, "CARGO_VALIDATION_ITEM")
	order, pickupID, deliveryID := f.confirmedOrderWithStops(t, customer.ID, item.ID)
	shipment, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: order.ID, OriginStopID: pickupID, DestinationStopID: deliveryID, DeclaredValueCurrency: "NGN",
	})
	if err != nil {
		t.Fatalf("create shipment: %v", err)
	}

	_, err = f.shipments.AddCargoItem(ctx, f.tenant, shipment.ID, shipmentsusecases.AddCargoItemInput{
		Description: "Bad quantity", Quantity: "0", Unit: "BOX", DeclaredValueCurrency: "NGN",
	})
	if err == nil {
		t.Fatal("expected an error for a zero quantity")
	}

	badMax := "-5"
	badMin := "5"
	_, err = f.shipments.AddCargoItem(ctx, f.tenant, shipment.ID, shipmentsusecases.AddCargoItemInput{
		Description: "Bad temperature range", Quantity: "1", Unit: "BOX",
		TemperatureMin: &badMin, TemperatureMax: &badMax, DeclaredValueCurrency: "NGN",
	})
	if err == nil {
		t.Fatal("expected an error when temperatureMax is below temperatureMin")
	}
}

func shipmentsErrorCode(err error) string {
	return crmErrorCode(err)
}
