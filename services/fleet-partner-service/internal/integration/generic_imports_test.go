package integration_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	bulkhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/http"
	bulkmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
	bulkrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/repositories"
	bulkusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ---------------------------------------------------------------------------
// Section 25C generic import framework — fixture.
//
// A real workforce.Service, assets.Service and warehouse.Service back the
// three new adapters, exactly as internal/app/bulkimports_wiring.go wires
// the real ones — a commit in these tests is a real domain write, not a
// simulation of one. PRIVATE_ORDER uses the same lightweight OrdersPort
// fake the existing legacy-engine matrix suite
// (bulk_imports_section15_matrix_test.go) already uses, since the real
// order-creation path is what that suite proves; this suite's job is
// proving the generic engine's orchestration around whichever adapter it
// is driving, not re-proving orders.Create itself.
// ---------------------------------------------------------------------------

type genericImportsFixture struct {
	pool         *pgxpool.Pool
	service      *bulkusecases.Service
	media        *bulkMediaStub
	orders       *bulkOrdersStub
	orgID        string
	branchID     string
	membershipID string
	warehouseID  string
	locationID   string
}

func newGenericImportsFixture(t *testing.T, name, registration string) *genericImportsFixture {
	t.Helper()
	ctx := context.Background()
	pool := testPool(t)
	org := createOrganization(t, pool, name, registration)
	branch := ids.New("brn")
	if _, e := pool.Exec(ctx, `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone)VALUES($1,$2,'GEN','Generic Ops','Africa/Lagos')`, branch, org.ID); e != nil {
		t.Fatal(e)
	}
	membership := ids.New("mem")
	if _, e := pool.Exec(ctx, `INSERT INTO fleet_partner_organization_provider_memberships(id,organization_id,provider_service,service_category,provider_id,onboarding_source,status)VALUES($1,$2,'driver-dispatch-delivery-service','DISPATCH','provider-gen','ORGANIZATION_SPONSORED','ACTIVE')`, membership, org.ID); e != nil {
		t.Fatal(e)
	}

	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	workforceSvc := workforce.NewService(workforce.Options{Pool: pool, Events: publisher, Flags: workforce.Flags{Workforce: true, Workers: true}})
	assetsSvc := assets.NewService(assets.Options{Pool: pool, Events: publisher, Flags: assets.Flags{Assets: true, Workers: true}})
	warehouseSvc := warehouse.NewService(warehouse.Options{Pool: pool, Events: publisher, Restrictions: unrestrictedWarehouseGate{}, Flags: warehouse.Flags{Warehouses: true, Inventory: true}})

	scope := tenant.Context{OrganizationID: org.ID, OrganizationUserID: membership, ActorType: tenant.ActorTypeOrganizationUser, ActorID: membership}
	wh, err := warehouseSvc.CreateWarehouse(ctx, scope, warehouse.Warehouse{BranchID: branch, Code: "WH-GEN", Name: "Generic Warehouse", AddressReference: "addr", Timezone: "Africa/Lagos"})
	if err != nil {
		t.Fatalf("create warehouse: %v", err)
	}
	// A freshly created warehouse does not default to ACTIVE — Receipt
	// refuses against anything else, matching how a real warehouse must be
	// deliberately activated before it accepts stock.
	wh, err = warehouseSvc.UpdateWarehouse(ctx, scope, wh.ID, wh.Version, warehouse.Warehouse{Status: "ACTIVE"}, false)
	if err != nil {
		t.Fatalf("activate warehouse: %v", err)
	}
	loc, err := warehouseSvc.CreateLocation(ctx, scope, wh.ID, warehouse.Location{Code: "A1", Name: "Bin 1", LocationType: "BIN"})
	if err != nil {
		t.Fatalf("create location: %v", err)
	}

	media := &bulkMediaStub{}
	orders := &bulkOrdersStub{}
	adapters := map[string]bulkusecases.Adapter{
		bulkmodels.TypePrivateOrder:       genericTestOrdersAdapter{orders: orders},
		bulkmodels.TypeWorkforceProfile:   workforceImportAdapterForTest{workforce: workforceSvc},
		bulkmodels.TypeAsset:              assetImportAdapterForTest{assets: assetsSvc},
		bulkmodels.TypeWarehouseInventory: warehouseImportAdapterForTest{warehouse: warehouseSvc},
	}
	repo := bulkrepos.NewRepository(pool)
	service := bulkusecases.NewService(bulkusecases.Options{
		Pool: pool, Repository: repo, Media: media, Orders: orders, Flags: bulkusecases.Flags{Enabled: true, WritesEnabled: true, WorkerEnabled: true},
		MaxBytes: 1 << 20, MaxRows: 100,
		Adapters:     adapters,
		GenericFlags: bulkusecases.GenericFlags{Enabled: true, WritesEnabled: true, ValidationWorker: true, CommitWorker: true, RetryWorker: true},
	})

	return &genericImportsFixture{pool: pool, service: service, media: media, orders: orders, orgID: org.ID,
		branchID: branch, membershipID: membership, warehouseID: wh.ID, locationID: loc.ID}
}

func (f *genericImportsFixture) actor(memberID string) tenant.Context {
	return tenant.Context{OrganizationID: f.orgID, OrganizationUserID: memberID, ActorType: tenant.ActorTypeOrganizationUser, ActorID: memberID}
}

func (f *genericImportsFixture) create(t *testing.T, importType, csvContent, mode string) bulkmodels.Import {
	t.Helper()
	f.media.file = bulkusecases.MediaFile{Format: "CSV", ContentType: "text/csv", SizeBytes: int64(len(csvContent)), Content: []byte(csvContent)}
	job, err := f.service.CreateGeneric(context.Background(), f.actor(f.membershipID), bulkusecases.CreateGenericInput{
		BranchID: f.branchID, ImportType: importType, MediaFileID: "media_" + ids.New("mf"), FileFormat: "CSV", Mode: mode,
	})
	if err != nil {
		t.Fatalf("create generic import: %v", err)
	}
	return job
}

func (f *genericImportsFixture) validate(t *testing.T, id string) bulkusecases.GenericWorkerResult {
	t.Helper()
	ctx := context.Background()
	if _, err := f.service.Validate(ctx, f.actor(f.membershipID), id); err != nil {
		t.Fatalf("validate: %v", err)
	}
	result, err := f.service.RunValidationPass(ctx, bulkusecases.GenericWorkerConfig{Owner: "test-validate", Lease: 5 * time.Second, MaxAttempts: 3})
	if err != nil {
		t.Fatalf("run validation pass: %v", err)
	}
	return result
}

func (f *genericImportsFixture) commit(t *testing.T, id string) bulkusecases.GenericWorkerResult {
	t.Helper()
	ctx := context.Background()
	if _, err := f.service.Commit(ctx, f.actor(f.membershipID), id); err != nil {
		t.Fatalf("commit: %v", err)
	}
	result, err := f.service.RunCommitPass(ctx, bulkusecases.GenericWorkerConfig{Owner: "test-commit", Lease: 5 * time.Second, MaxAttempts: 3})
	if err != nil {
		t.Fatalf("run commit pass: %v", err)
	}
	return result
}

func (f *genericImportsFixture) router(t *testing.T, memberID string) http.Handler {
	t.Helper()
	return f.routerForOrg(t, f.orgID, memberID)
}

// routerForOrg builds a router scoped to an arbitrary organization id in the
// SAME database this fixture already owns — for a cross-tenant test, this is
// the correct way to reach a second organization: testPool(t) takes a
// process-local, test-scoped advisory lock for the whole test function, so
// calling newGenericImportsFixture (which calls testPool) a second time in
// the same test deadlocks against itself rather than reaching a second
// database.
func (f *genericImportsFixture) routerForOrg(t *testing.T, organizationID, memberID string) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(f.pool), nil, 0)
	store := idempotency.NewStore(f.pool, 0)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{OrganizationID: organizationID, OrganizationUserID: memberID,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: memberID})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	bulkhttp.NewHandler(f.service, authorizer, store).RegisterTenantRoutes(group)
	return engine
}

// ---------------------------------------------------------------------------
// Generic-engine PRIVATE_ORDER adapter (test double, mirrors
// internal/app/imports_adapters.go's genericOrdersAdapter but against the
// bulkOrdersStub already defined in bulk_imports_section15_matrix_test.go).
// ---------------------------------------------------------------------------

type genericTestOrdersAdapter struct{ orders *bulkOrdersStub }

func (genericTestOrdersAdapter) Type() string       { return bulkmodels.TypePrivateOrder }
func (genericTestOrdersAdapter) SchemaVersion() int { return 1 }
func (genericTestOrdersAdapter) RequiredColumns() []string {
	return []string{"rowReference", "customerId", "serviceCatalogItemId", "timezone"}
}
func (genericTestOrdersAdapter) OptionalColumns() []string { return []string{"customerReference"} }

func (genericTestOrdersAdapter) ValidateRow(raw map[string]string) bulkusecases.RowOutcome {
	ref := strings.TrimSpace(raw["rowReference"])
	if ref == "" || raw["customerId"] == "" || raw["serviceCatalogItemId"] == "" {
		return bulkusecases.RowOutcome{Valid: false, ErrorCode: "missing_required_field"}
	}
	if _, err := time.LoadLocation(raw["timezone"]); err != nil {
		return bulkusecases.RowOutcome{Valid: false, ErrorCode: "timezone_invalid"}
	}
	return bulkusecases.RowOutcome{Valid: true, ExternalReference: ref, Data: map[string]any{
		"rowReference": ref, "customerId": raw["customerId"], "serviceCatalogItemId": raw["serviceCatalogItemId"], "timezone": raw["timezone"],
	}}
}

func (a genericTestOrdersAdapter) Commit(ctx context.Context, t tenant.Context, correlationID string, data map[string]any) (string, error) {
	row := bulkmodels.OrderRow{RowReference: data["rowReference"].(string), CustomerID: data["customerId"].(string),
		ServiceCatalogItemID: data["serviceCatalogItemId"].(string), Timezone: data["timezone"].(string)}
	return a.orders.CreateImported(ctx, t, bulkusecases.OrderInput{BranchID: branchOfTest(t), Row: row, SourceReference: correlationID})
}

func branchOfTest(t tenant.Context) string {
	if len(t.PermittedBranchIDs) > 0 {
		return t.PermittedBranchIDs[0]
	}
	return ""
}

// ---------------------------------------------------------------------------
// Real domain adapters, identical in behavior to internal/app's, kept as
// separate test-local types so this suite never depends on internal/app
// (which would risk an import cycle back into internal/integration's own
// dependency graph and couples the test to composition-root wiring rather
// than to the Adapter contract itself).
// ---------------------------------------------------------------------------

type workforceImportAdapterForTest struct{ workforce *workforce.Service }

func (workforceImportAdapterForTest) Type() string       { return bulkmodels.TypeWorkforceProfile }
func (workforceImportAdapterForTest) SchemaVersion() int { return 1 }
func (workforceImportAdapterForTest) RequiredColumns() []string {
	return []string{"membershipId", "employeeNumber", "relationshipType", "role"}
}
func (workforceImportAdapterForTest) OptionalColumns() []string {
	return []string{"supervisorMembershipId"}
}

func (workforceImportAdapterForTest) ValidateRow(raw map[string]string) bulkusecases.RowOutcome {
	if raw["membershipId"] == "" || raw["employeeNumber"] == "" || raw["relationshipType"] == "" || raw["role"] == "" {
		return bulkusecases.RowOutcome{Valid: false, ErrorCode: "missing_required_field"}
	}
	return bulkusecases.RowOutcome{Valid: true, ExternalReference: raw["employeeNumber"], Data: map[string]any{
		"membershipId": raw["membershipId"], "employeeNumber": raw["employeeNumber"],
		"relationshipType": raw["relationshipType"], "role": raw["role"],
	}}
}

func (a workforceImportAdapterForTest) Commit(ctx context.Context, t tenant.Context, _ string, data map[string]any) (string, error) {
	p, err := a.workforce.CreateProfile(ctx, t, workforce.CreateProfileInput{
		BranchID: branchOfTest(t), MembershipID: data["membershipId"].(string), EmployeeNumber: data["employeeNumber"].(string),
		RelationshipType: data["relationshipType"].(string), Role: data["role"].(string), StartDate: time.Now().UTC(),
	})
	if err != nil {
		return "", asDuplicateConflictForTest(err, "duplicate_employee")
	}
	return p.ID, nil
}

type assetImportAdapterForTest struct{ assets *assets.Service }

func (assetImportAdapterForTest) Type() string       { return bulkmodels.TypeAsset }
func (assetImportAdapterForTest) SchemaVersion() int { return 1 }
func (assetImportAdapterForTest) RequiredColumns() []string {
	return []string{"assetCode", "assetType"}
}
func (assetImportAdapterForTest) OptionalColumns() []string { return nil }

func (assetImportAdapterForTest) ValidateRow(raw map[string]string) bulkusecases.RowOutcome {
	if raw["assetCode"] == "" || raw["assetType"] == "" {
		return bulkusecases.RowOutcome{Valid: false, ErrorCode: "missing_required_field"}
	}
	return bulkusecases.RowOutcome{Valid: true, ExternalReference: raw["assetCode"], Data: map[string]any{
		"assetCode": raw["assetCode"], "assetType": raw["assetType"],
	}}
}

func (a assetImportAdapterForTest) Commit(ctx context.Context, t tenant.Context, _ string, data map[string]any) (string, error) {
	out, err := a.assets.CreateAsset(ctx, t, assets.Asset{
		BranchID: branchOfTest(t), AssetCode: data["assetCode"].(string), AssetType: data["assetType"].(string),
	})
	if err != nil {
		return "", asDuplicateConflictForTest(err, "duplicate_asset_code")
	}
	return out.ID, nil
}

type warehouseImportAdapterForTest struct{ warehouse *warehouse.Service }

func (warehouseImportAdapterForTest) Type() string       { return bulkmodels.TypeWarehouseInventory }
func (warehouseImportAdapterForTest) SchemaVersion() int { return 1 }
func (warehouseImportAdapterForTest) RequiredColumns() []string {
	return []string{"warehouseId", "locationId", "ownerType", "ownerReference", "sku", "quantity"}
}
func (warehouseImportAdapterForTest) OptionalColumns() []string { return nil }

func (warehouseImportAdapterForTest) ValidateRow(raw map[string]string) bulkusecases.RowOutcome {
	if raw["warehouseId"] == "" || raw["locationId"] == "" || raw["ownerType"] == "" || raw["ownerReference"] == "" || raw["sku"] == "" {
		return bulkusecases.RowOutcome{Valid: false, ErrorCode: "missing_required_field"}
	}
	qty, err := parseFloatForTest(raw["quantity"])
	if err != nil || qty <= 0 {
		return bulkusecases.RowOutcome{Valid: false, ErrorCode: "quantity_invalid"}
	}
	ref := raw["warehouseId"] + "|" + raw["locationId"] + "|" + raw["sku"]
	return bulkusecases.RowOutcome{Valid: true, ExternalReference: ref, Data: map[string]any{
		"warehouseId": raw["warehouseId"], "locationId": raw["locationId"], "ownerType": raw["ownerType"],
		"ownerReference": raw["ownerReference"], "sku": raw["sku"], "quantity": qty,
	}}
}

func (a warehouseImportAdapterForTest) Commit(ctx context.Context, t tenant.Context, correlationID string, data map[string]any) (string, error) {
	item, err := a.warehouse.Receipt(ctx, t, warehouse.ReceiptInput{
		WarehouseID: data["warehouseId"].(string), LocationID: data["locationId"].(string), OwnerType: data["ownerType"].(string),
		OwnerReference: data["ownerReference"].(string), SKU: data["sku"].(string), Quantity: numericForTest(data["quantity"]),
		CorrelationID: correlationID,
	})
	if err != nil {
		return "", err
	}
	return item.ID, nil
}

func numericForTest(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	default:
		return 0
	}
}

func parseFloatForTest(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// asDuplicateConflictForTest mirrors internal/app/imports_adapters.go's
// asDuplicateConflict: only a genuine unique-violation is remapped to a safe
// code, so this test proves the mapping without masking an unrelated error.
func asDuplicateConflictForTest(err error, code string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return errors.New(code)
	}
	return err
}
