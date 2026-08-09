package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	crmmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	crmrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/repositories"
	crmusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/usecases"
	ordersmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	ordersrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/repositories"
	ordersusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/usecases"
	searchrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/repositories"
	searchusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/usecases"
	catalogmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/models"
	catalogrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/repositories"
	catalogusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ---------------------------------------------------------------------------
// Fixture
// ---------------------------------------------------------------------------

type ordersFixture struct {
	pool       *pgxpool.Pool
	catalog    *catalogusecases.Service
	orders     *ordersusecases.Service
	crm        *crmusecases.Service
	search     *searchusecases.Service
	ordersRepo *ordersrepos.Repository
	tenant     tenant.Context
	other      tenant.Context
}

func allCatalogFlagsOn() catalogusecases.Flags {
	return catalogusecases.Flags{Enabled: true, WritesEnabled: true}
}

func allOrdersFlagsOn() ordersusecases.Flags {
	return ordersusecases.Flags{Enabled: true, WritesEnabled: true}
}

func newOrdersFixture(t *testing.T) *ordersFixture {
	t.Helper()
	pool := testPool(t)
	primary := createOrganization(t, pool, "Orders Primary Fleet", "RC-ORD-1")
	secondary := createOrganization(t, pool, "Orders Secondary Fleet", "RC-ORD-2")

	metrics := observability.NewMetrics()
	crmRepo := crmrepos.NewRepository(pool)
	crmService := crmusecases.NewService(crmusecases.Options{
		Pool: pool, Repository: crmRepo, Metrics: metrics, Flags: allCRMFlagsOn(),
		TokenSecret:   []byte("orders-integration-token-secret"),
		ContactSecret: []byte("orders-integration-contact-secret"),
		TokenTTL:      48 * time.Hour,
	})

	catalogRepo := catalogrepos.NewRepository(pool)
	catalogService := catalogusecases.NewService(catalogusecases.Options{
		Pool: pool, Repository: catalogRepo, Metrics: metrics, Flags: allCatalogFlagsOn(),
	})

	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	searchService := searchusecases.NewService(searchusecases.Options{
		Pool: pool, Repository: searchrepos.NewRepository(pool), Authorizer: authorizer,
		Metrics: metrics, Flags: searchusecases.Flags{Enabled: true},
	})

	ordersRepo := ordersrepos.NewRepository(pool)
	ordersService := ordersusecases.NewService(ordersusecases.Options{
		Pool: pool, Repository: ordersRepo,
		Catalog: app.NewOrdersCatalogAdapter(catalogRepo), CRM: app.NewOrdersCRMAdapter(crmRepo),
		Search:  app.NewOrdersSearchAdapter(searchService),
		Metrics: metrics, Flags: allOrdersFlagsOn(),
	})

	return &ordersFixture{
		pool: pool, catalog: catalogService, orders: ordersService, crm: crmService, search: searchService,
		ordersRepo: ordersRepo,
		tenant: tenant.Context{OrganizationID: primary.ID, OrganizationUserID: "usr_orders_primary",
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: "usr_orders_primary"},
		other: tenant.Context{OrganizationID: secondary.ID, OrganizationUserID: "usr_orders_other",
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: "usr_orders_other"},
	}
}

func (f *ordersFixture) activeCatalogItem(t *testing.T, code string) catalogmodels.CatalogItem {
	t.Helper()
	ctx := context.Background()
	item, err := f.catalog.Create(ctx, f.tenant, catalogusecases.CreateInput{
		Name: "Same Day Delivery " + code, Code: code, Category: catalogmodels.CategoryPackageDelivery,
		CapabilityReference: "cap_" + code,
		RequiredFieldsSchema: catalogmodels.JSONMap{
			"receiverName": map[string]any{"type": "string", "required": true},
		},
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

func (f *ordersFixture) customer(t *testing.T, name string) crmmodels.Customer {
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

func ordersErrorCode(err error) string {
	if err == nil {
		return ""
	}
	return string(apperrors.From(err).Code)
}

// ---------------------------------------------------------------------------
// Service catalogue
// ---------------------------------------------------------------------------

func TestServiceCatalog_LifecycleAndTaxiFailClosed(t *testing.T) {
	f := newOrdersFixture(t)
	ctx := context.Background()

	item, err := f.catalog.Create(ctx, f.tenant, catalogusecases.CreateInput{
		Name: "Same Day", Code: "same day one", Category: catalogmodels.CategoryPackageDelivery,
		CapabilityReference: "cap_same_day",
	})
	if err != nil {
		t.Fatalf("create catalog item: %v", err)
	}
	if item.Status != catalogmodels.StatusDraft {
		t.Fatalf("expected DRAFT, got %s", item.Status)
	}
	if item.Code != "SAME_DAY_ONE" {
		t.Fatalf("expected normalized code SAME_DAY_ONE, got %s", item.Code)
	}

	// A reused code is refused.
	_, err = f.catalog.Create(ctx, f.tenant, catalogusecases.CreateInput{
		Name: "Duplicate", Code: "Same Day One", Category: catalogmodels.CategoryPackageDelivery,
	})
	if ordersErrorCode(err) != "service_catalog_code_conflict" {
		t.Fatalf("expected a code conflict, got %v", err)
	}

	activated, err := f.catalog.Activate(ctx, f.tenant, item.ID, item.Version)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if activated.Status != catalogmodels.StatusActive {
		t.Fatalf("expected ACTIVE, got %s", activated.Status)
	}

	deactivated, err := f.catalog.Deactivate(ctx, f.tenant, item.ID, activated.Version)
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	archived, err := f.catalog.Archive(ctx, f.tenant, item.ID, deactivated.Version)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived.Status != catalogmodels.StatusArchived {
		t.Fatalf("expected ARCHIVED, got %s", archived.Status)
	}

	// An archived item admits no further transition, and its code stays
	// permanently reserved — even for a semantically different new item.
	if _, err := f.catalog.Activate(ctx, f.tenant, item.ID, archived.Version); ordersErrorCode(err) != "service_catalog_item_invalid_state" {
		t.Fatalf("expected an invalid-state error reactivating an archived item, got %v", err)
	}
	if _, err := f.catalog.Create(ctx, f.tenant, catalogusecases.CreateInput{
		Name: "New meaning", Code: "SAME_DAY_ONE", Category: catalogmodels.CategoryHauling,
	}); ordersErrorCode(err) != "service_catalog_code_conflict" {
		t.Fatalf("an archived code must stay reserved, got %v", err)
	}

	// Taxi/PassengerTransport activation is refused unconditionally.
	for _, category := range []string{catalogmodels.CategoryTaxi, catalogmodels.CategoryPassengerTransport} {
		taxiItem, err := f.catalog.Create(ctx, f.tenant, catalogusecases.CreateInput{
			Name: "Taxi item " + category, Code: "TAXI_" + category, Category: category,
			CapabilityReference: "cap_taxi",
		})
		if err != nil {
			t.Fatalf("create %s catalog item: %v", category, err)
		}
		if _, err := f.catalog.Activate(ctx, f.tenant, taxiItem.ID, taxiItem.Version); ordersErrorCode(err) != "service_catalog_category_disabled" {
			t.Fatalf("%s activation must be refused unconditionally, got %v", category, err)
		}
	}

	// Activation without a capability reference is refused.
	noCap, err := f.catalog.Create(ctx, f.tenant, catalogusecases.CreateInput{
		Name: "No capability", Code: "NO_CAP", Category: catalogmodels.CategoryHauling,
	})
	if err != nil {
		t.Fatalf("create no-capability item: %v", err)
	}
	if _, err := f.catalog.Activate(ctx, f.tenant, noCap.ID, noCap.Version); ordersErrorCode(err) != "service_catalog_capability_required" {
		t.Fatalf("expected capability_required, got %v", err)
	}

	// Cross-tenant read is indistinguishable from not-found.
	if _, err := f.catalog.Get(ctx, f.other, item.ID); ordersErrorCode(err) != "service_catalog_item_not_found" {
		t.Fatalf("cross-tenant read must be not-found, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Private orders: create, stops, confirm, cancel
// ---------------------------------------------------------------------------

func TestOrders_CreateConfirmCancelLifecycle(t *testing.T) {
	f := newOrdersFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Lifecycle Co")
	item := f.activeCatalogItem(t, "LIFECYCLE_ITEM")

	// Missing required service field is refused at creation.
	_, err := f.orders.Create(ctx, f.tenant, ordersusecases.CreateInput{
		CustomerID: customer.ID, ServiceCatalogItemID: item.ID, Timezone: "Africa/Lagos",
	})
	if err == nil {
		t.Fatal("expected an error for a missing required service field")
	}

	order, err := f.orders.Create(ctx, f.tenant, ordersusecases.CreateInput{
		CustomerID: customer.ID, ServiceCatalogItemID: item.ID, Timezone: "Africa/Lagos",
		ServiceFields: ordersmodels.JSONMap{"receiverName": "Ada Okafor"},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.Status != ordersmodels.StatusDraft {
		t.Fatalf("expected DRAFT, got %s", order.Status)
	}
	if order.Source != ordersmodels.SourceManual {
		t.Fatalf("expected MANUAL source, got %s", order.Source)
	}

	// Confirming with no stops is refused.
	if _, err := f.orders.Confirm(ctx, f.tenant, order.ID, order.Version); ordersErrorCode(err) != "order_stops_required" {
		t.Fatalf("expected order_stops_required, got %v", err)
	}

	pickup, err := f.orders.AddStop(ctx, f.tenant, order.ID, ordersusecases.AddStopInput{
		StopType: ordersmodels.StopTypePickup, Timezone: "Africa/Lagos",
	})
	if err != nil {
		t.Fatalf("add pickup stop: %v", err)
	}
	if pickup.Sequence != 0 {
		t.Fatalf("expected sequence 0, got %d", pickup.Sequence)
	}
	delivery, err := f.orders.AddStop(ctx, f.tenant, order.ID, ordersusecases.AddStopInput{
		StopType: ordersmodels.StopTypeDelivery, Timezone: "Africa/Lagos",
	})
	if err != nil {
		t.Fatalf("add delivery stop: %v", err)
	}
	if delivery.Sequence != 1 {
		t.Fatalf("expected sequence 1, got %d", delivery.Sequence)
	}

	// A stale expected version is refused.
	if _, err := f.orders.Confirm(ctx, f.tenant, order.ID, order.Version+99); ordersErrorCode(err) != "conflict" {
		t.Fatalf("expected a version conflict, got %v", err)
	}

	confirmed, err := f.orders.Confirm(ctx, f.tenant, order.ID, order.Version)
	if err != nil {
		t.Fatalf("confirm order: %v", err)
	}
	if confirmed.Status != ordersmodels.StatusConfirmed {
		t.Fatalf("expected CONFIRMED, got %s", confirmed.Status)
	}
	if confirmed.ConfirmedAt == nil {
		t.Fatal("expected confirmedAt to be set")
	}
	if confirmed.ServiceCatalogSnapshot["code"] != item.Code {
		t.Fatalf("expected a frozen catalogue snapshot, got %v", confirmed.ServiceCatalogSnapshot)
	}

	// Confirming again from CONFIRMED is refused (not idempotent — CONFIRMED
	// has no self-edge).
	if _, err := f.orders.Confirm(ctx, f.tenant, order.ID, confirmed.Version); ordersErrorCode(err) != "order_not_confirmable" {
		t.Fatalf("expected order_not_confirmable, got %v", err)
	}

	history, err := f.orders.History(ctx, f.tenant, order.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 1 || history[0].NewStatus != ordersmodels.StatusConfirmed {
		t.Fatalf("expected one CONFIRMED history entry, got %+v", history)
	}

	cancelled, err := f.orders.Cancel(ctx, f.tenant, order.ID, confirmed.Version, "customer_changed_mind")
	if err != nil {
		t.Fatalf("cancel order: %v", err)
	}
	if cancelled.Status != ordersmodels.StatusCancelled {
		t.Fatalf("expected CANCELLED, got %s", cancelled.Status)
	}
	if cancelled.CancelReasonCode != "customer_changed_mind" {
		t.Fatalf("expected reason code preserved, got %s", cancelled.CancelReasonCode)
	}

	// Cancelling a terminal order again is refused.
	if _, err := f.orders.Cancel(ctx, f.tenant, order.ID, cancelled.Version, "again"); ordersErrorCode(err) != "order_not_cancellable" {
		t.Fatalf("expected order_not_cancellable, got %v", err)
	}

	// Cross-tenant read is indistinguishable from not-found.
	if _, err := f.orders.Get(ctx, f.other, order.ID); ordersErrorCode(err) != "order_not_found" {
		t.Fatalf("cross-tenant read must be not-found, got %v", err)
	}
}

// TestOrders_IndexedIntoSearchAcrossLifecycle proves an order becomes real,
// end-to-end findable through Section 25 search on creation, and that its
// indexed status tracks its lifecycle through confirm and cancel — this is
// the WP3 gap-close: the search projection was previously proven correct in
// isolation, but nothing actually populated it from a real write path.
func TestOrders_IndexedIntoSearchAcrossLifecycle(t *testing.T) {
	f := newOrdersFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Search Indexed Co")
	item := f.activeCatalogItem(t, "SEARCH_INDEXED_ITEM")

	order, err := f.orders.Create(ctx, f.tenant, ordersusecases.CreateInput{
		CustomerID: customer.ID, ServiceCatalogItemID: item.ID, Timezone: "Africa/Lagos",
		ServiceFields: ordersmodels.JSONMap{"receiverName": "Ada Okafor"},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	owner := tenant.Context{OrganizationID: f.tenant.OrganizationID,
		OrganizationUserID: seedApprovalMember(t, f.pool, f.tenant.OrganizationID, "rol_system_owner", "search-owner"),
		ActorType:          tenant.ActorTypeOrganizationUser}

	results, err := f.search.Search(ctx, owner, searchusecases.SearchInput{Text: order.OrderNumber})
	if err != nil {
		t.Fatalf("search after create: %v", err)
	}
	if len(results) != 1 || results[0].SubjectReference != order.ID {
		t.Fatalf("search after create = %+v, want exactly the new order", results)
	}
	if results[0].Status != ordersmodels.StatusDraft {
		t.Fatalf("indexed status = %s, want DRAFT", results[0].Status)
	}

	if _, err := f.orders.AddStop(ctx, f.tenant, order.ID, ordersusecases.AddStopInput{
		StopType: ordersmodels.StopTypePickup, Timezone: "Africa/Lagos",
	}); err != nil {
		t.Fatalf("add pickup stop: %v", err)
	}
	if _, err := f.orders.AddStop(ctx, f.tenant, order.ID, ordersusecases.AddStopInput{
		StopType: ordersmodels.StopTypeDelivery, Timezone: "Africa/Lagos",
	}); err != nil {
		t.Fatalf("add delivery stop: %v", err)
	}
	confirmed, err := f.orders.Confirm(ctx, f.tenant, order.ID, order.Version)
	if err != nil {
		t.Fatalf("confirm order: %v", err)
	}

	results, err = f.search.Search(ctx, owner, searchusecases.SearchInput{Text: order.OrderNumber})
	if err != nil {
		t.Fatalf("search after confirm: %v", err)
	}
	if len(results) != 1 || results[0].Status != ordersmodels.StatusConfirmed {
		t.Fatalf("search after confirm = %+v, want CONFIRMED", results)
	}
	if results[0].SourceVersion != confirmed.Version {
		t.Fatalf("indexed sourceVersion = %d, want %d (the order's own version)",
			results[0].SourceVersion, confirmed.Version)
	}

	if _, err := f.orders.Cancel(ctx, f.tenant, order.ID, confirmed.Version, "customer_changed_mind"); err != nil {
		t.Fatalf("cancel order: %v", err)
	}
	results, err = f.search.Search(ctx, owner, searchusecases.SearchInput{Text: order.OrderNumber})
	if err != nil {
		t.Fatalf("search after cancel: %v", err)
	}
	if len(results) != 1 || results[0].Status != ordersmodels.StatusCancelled {
		t.Fatalf("search after cancel = %+v, want CANCELLED", results)
	}

	// A caller in the other tenant, even with a matching role, finds nothing:
	// the search candidate set is tenant-scoped in SQL before permission
	// filtering ever runs.
	otherOwner := tenant.Context{OrganizationID: f.other.OrganizationID,
		OrganizationUserID: seedApprovalMember(t, f.pool, f.other.OrganizationID, "rol_system_owner", "search-other"),
		ActorType:          tenant.ActorTypeOrganizationUser}
	crossTenant, err := f.search.Search(ctx, otherOwner, searchusecases.SearchInput{Text: order.OrderNumber})
	if err != nil {
		t.Fatalf("cross-tenant search: %v", err)
	}
	if len(crossTenant) != 0 {
		t.Fatalf("cross-tenant search leaked %d results", len(crossTenant))
	}
}

func TestOrders_InactiveCatalogItemBlocksCreateAndConfirm(t *testing.T) {
	f := newOrdersFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Inactive Catalog Co")

	draftItem, err := f.catalog.Create(ctx, f.tenant, catalogusecases.CreateInput{
		Name: "Still Draft", Code: "STILL_DRAFT", Category: catalogmodels.CategoryHauling,
	})
	if err != nil {
		t.Fatalf("create draft catalog item: %v", err)
	}

	_, err = f.orders.Create(ctx, f.tenant, ordersusecases.CreateInput{
		CustomerID: customer.ID, ServiceCatalogItemID: draftItem.ID, Timezone: "Africa/Lagos",
	})
	if ordersErrorCode(err) != "order_catalog_item_not_active" {
		t.Fatalf("expected order_catalog_item_not_active, got %v", err)
	}
}

func TestOrders_BlockedCustomerCannotReceiveNewOrder(t *testing.T) {
	f := newOrdersFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Restricted Co")
	item := f.activeCatalogItem(t, "RESTRICTED_ITEM")

	if _, err := f.crm.ChangeCustomerRisk(ctx, f.tenant, customer.ID,
		crmusecases.ChangeCustomerRiskInput{RiskStatus: crmmodels.RiskRestricted, ReasonCode: "fraud_review",
			ExpectedVersion: customer.Version, IdempotencyKey: "restrict"}); err != nil {
		t.Fatalf("restrict customer: %v", err)
	}

	_, err := f.orders.Create(ctx, f.tenant, ordersusecases.CreateInput{
		CustomerID: customer.ID, ServiceCatalogItemID: item.ID, Timezone: "Africa/Lagos",
	})
	if ordersErrorCode(err) != "customer_risk_restricted" {
		t.Fatalf("expected customer_risk_restricted, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Section 14 -> Section 15 conversion-intent handoff
// ---------------------------------------------------------------------------

func TestOrders_ConvertConversionIntent_ConsumedExactlyOnce(t *testing.T) {
	f := newOrdersFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Quotation Handoff Co")
	item := f.activeCatalogItem(t, "HANDOFF_ITEM")

	detail, err := f.crm.CreateQuotation(ctx, f.tenant, crmusecases.CreateQuotationInput{
		CustomerID: customer.ID, Currency: "NGN",
		ValidFrom: time.Now().UTC().Add(-time.Hour), ValidUntil: time.Now().UTC().Add(72 * time.Hour),
		LineItems: []crmusecases.LineItemInput{
			{Description: "Same day delivery", Quantity: "1", UnitType: "TRIP", UnitAmountMinor: 500000},
		},
		IdempotencyKey: "handoff-quotation",
	})
	if err != nil {
		t.Fatalf("create quotation: %v", err)
	}
	submitted, err := f.crm.SubmitQuotation(ctx, f.tenant, detail.Quotation.ID,
		crmusecases.SubmitQuotationInput{ExpectedVersion: detail.Quotation.Version, IdempotencyKey: "handoff-submit"})
	if err != nil {
		t.Fatalf("submit quotation: %v", err)
	}
	issued, err := f.crm.IssueQuotation(ctx, f.tenant, detail.Quotation.ID,
		crmusecases.IssueQuotationInput{MintCustomerToken: true,
			ExpectedVersion: submitted.Quotation.Version, IdempotencyKey: "handoff-issue"})
	if err != nil {
		t.Fatalf("issue quotation: %v", err)
	}
	accepted, err := f.crm.AcceptQuotationByToken(ctx, issued.CustomerToken, detail.Quotation.ID,
		crmusecases.AcceptQuotationInput{AcceptedVersionID: issued.Version.ID, IdempotencyKey: "handoff-accept"})
	if err != nil {
		t.Fatalf("accept quotation: %v", err)
	}

	convertInput := ordersusecases.ConvertConversionIntentInput{
		ServiceCatalogItemID: item.ID, Timezone: "Africa/Lagos",
		ServiceFields: ordersmodels.JSONMap{"receiverName": "Ada Okafor"},
	}
	order, err := f.orders.ConvertConversionIntent(ctx, f.tenant, issued.Version.ID, convertInput)
	if err != nil {
		t.Fatalf("convert conversion intent: %v", err)
	}
	if order.Source != ordersmodels.SourceQuotation {
		t.Fatalf("expected QUOTATION source, got %s", order.Source)
	}
	if order.QuotationID != detail.Quotation.ID {
		t.Fatalf("expected quotationId to be preserved, got %s", order.QuotationID)
	}
	if order.Status != ordersmodels.StatusDraft {
		t.Fatalf("expected a DRAFT order, got %s", order.Status)
	}

	// A repeated conversion attempt is an idempotent replay, not a duplicate.
	replay, err := f.orders.ConvertConversionIntent(ctx, f.tenant, issued.Version.ID, convertInput)
	if err != nil {
		t.Fatalf("replay conversion: %v", err)
	}
	if replay.ID != order.ID {
		t.Fatalf("replay must return the same order, got %s vs %s", replay.ID, order.ID)
	}

	var orderCount int
	if err := f.pool.QueryRow(ctx,
		`SELECT count(*) FROM fleet_partner_private_orders WHERE conversion_intent_id = $1`,
		accepted.ConversionIntent.ID).Scan(&orderCount); err != nil {
		t.Fatalf("count orders for intent: %v", err)
	}
	if orderCount != 1 {
		t.Fatalf("expected exactly one order for the intent, got %d", orderCount)
	}

	// The intent is durably marked consumed and points at the order.
	var consumedAt *time.Time
	var privateOrderID *string
	if err := f.pool.QueryRow(ctx,
		`SELECT consumed_at, private_order_id FROM fleet_partner_quotation_conversion_intents WHERE id = $1`,
		accepted.ConversionIntent.ID).Scan(&consumedAt, &privateOrderID); err != nil {
		t.Fatalf("read intent consumption: %v", err)
	}
	if consumedAt == nil {
		t.Fatal("expected consumed_at to be set")
	}
	if privateOrderID == nil || *privateOrderID != order.ID {
		t.Fatalf("expected private_order_id to point at the order, got %v", privateOrderID)
	}

	// An unknown accepted-version id is a clean not-found.
	if _, err := f.orders.ConvertConversionIntent(ctx, f.tenant, "qvr_00000000000000000000000000000000",
		convertInput); ordersErrorCode(err) != "conversion_intent_not_found" {
		t.Fatalf("expected conversion_intent_not_found, got %v", err)
	}
}
