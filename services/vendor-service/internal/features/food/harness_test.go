package food

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/events"
)

// fixedScopes is a test ScopeResolver: it returns whatever scope the test
// declares for a staff id, so authorization behaviour can be exercised without
// standing up the whole Section 5 IAM graph. The real adapter over the Section
// 5 evaluator is wired in cmd/adapters.go, and the permission-to-role mapping
// itself is asserted against the real seed in permissions_integration_test.go.
type fixedScopes map[string]Scope

func (f fixedScopes) ResolveScope(_ context.Context, staffID, _ string) (Scope, error) {
	return f[staffID], nil
}

// scopesByPermission answers per permission, so a test can give one staff
// member manage-without-publish (the kitchen role's real shape).
type scopesByPermission map[string]map[string]Scope

func (s scopesByPermission) ResolveScope(_ context.Context, staffID, permission string) (Scope, error) {
	return s[staffID][permission], nil
}

type recordingAudit struct{ entries []AuditEntry }

func (r *recordingAudit) Record(_ context.Context, entry AuditEntry) error {
	r.entries = append(r.entries, entry)
	return nil
}

// stubMedia answers the Media private-API question without a network call.
type stubMedia struct {
	objects map[string]MediaObject
	err     error
}

func (s stubMedia) GetObject(_ context.Context, _ string, ref string) (MediaObject, error) {
	if s.err != nil {
		return MediaObject{}, s.err
	}
	object, found := s.objects[ref]
	if !found {
		return MediaObject{}, errNotFound("Media object", CodeMediaUnavailable)
	}
	return object, nil
}

// activeMedia answers "owned, complete and safe" for the named references, and
// always carries two negative fixtures so the unsafe and incomplete paths can
// be exercised against the same stub.
func activeMedia(refs ...string) stubMedia {
	objects := map[string]MediaObject{
		"med_quarantined": {ID: "med_quarantined", UploadStatus: "COMPLETED", DeletionState: "ACTIVE", SafetyStatus: "QUARANTINED"},
		"med_pending":     {ID: "med_pending", UploadStatus: "PENDING", DeletionState: "ACTIVE", SafetyStatus: "SAFE"},
	}
	for _, ref := range refs {
		objects[ref] = MediaObject{ID: ref, UploadStatus: "COMPLETED", DeletionState: "ACTIVE", SafetyStatus: "SAFE"}
	}
	return stubMedia{objects: objects}
}

const (
	businessA = "vendor_food_a"
	businessB = "vendor_food_b"
	branchA1  = "brn_food_a1"
	branchA2  = "brn_food_a2"
	branchB1  = "brn_food_b1"
	staffA    = "vs_food_a"
	staffB    = "vs_food_b"
)

type testEnv struct {
	pool    *pgxpool.Pool
	service *Service
	audit   *recordingAudit
}

// newTestEnv builds a real-Postgres environment with two businesses, three
// branches and the RESTAURANT/FAST_FOOD capabilities already granted, so every
// tenancy and capability assertion is against genuinely separate tenants rather
// than a mock.
func newTestEnv(t *testing.T, flags Flags, scopes ScopeResolver, media MediaAuthority) *testEnv {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres food catalogue test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = database.ApplyMigrations(ctx, pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `TRUNCATE food_item_modifier_groups, food_modifiers, food_modifier_groups,
		food_item_variants, food_item_media, food_items, food_menu_sections, food_menus,
		vendor_outbox, vendor_audit_log, vendor_idempotency_keys,
		vendor_branch_capabilities, vendor_capabilities, vendor_branches, vendor_businesses,
		vendor_staff_role_assignments, vendor_staff, vendor_users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}

	for _, business := range []struct{ id, staff, user string }{
		{businessA, staffA, "vu_food_a"},
		{businessB, staffB, "vu_food_b"},
	} {
		if _, err = pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES($1,$1||'@example.test')`, business.user); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES($1,$2,$3,'active')`,
			business.staff, business.id, business.user); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO vendor_businesses(id,legal_name,status,owner_ref,currency,timezone)
			VALUES($1,$1||' Limited','ACTIVE',$2,'NGN','Africa/Lagos')`, business.id, business.staff); err != nil {
			t.Fatal(err)
		}
	}
	for _, branch := range []struct{ id, business string }{
		{branchA1, businessA}, {branchA2, businessA}, {branchB1, businessB},
	} {
		if _, err = pool.Exec(ctx, `INSERT INTO vendor_branches
			(id,business_id,display_name,address_line1,city,region,country_code,latitude,longitude,timezone,status)
			VALUES($1,$2,$1||' Branch','1 Test Road','Lagos','Lagos','NG',6.5,3.4,'Africa/Lagos','ACTIVE')`,
			branch.id, branch.business); err != nil {
			t.Fatal(err)
		}
	}
	// Both food capabilities, ACTIVE, at every branch. Tests that need the
	// negative case downgrade or delete one explicitly.
	for _, business := range []string{businessA, businessB} {
		for _, capability := range FoodCapabilities {
			capabilityID := "vcp_" + business + "_" + capability
			if _, err = pool.Exec(ctx, `INSERT INTO vendor_capabilities(id,business_id,capability_type,status)
				VALUES($1,$2,$3,'ACTIVE')`, capabilityID, business, capability); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, branch := range []struct{ id, business string }{
		{branchA1, businessA}, {branchA2, businessA}, {branchB1, businessB},
	} {
		for _, capability := range FoodCapabilities {
			if _, err = pool.Exec(ctx, `INSERT INTO vendor_branch_capabilities
				(id,business_id,branch_id,vendor_capability_id,capability_type,status)
				VALUES($1,$2,$3,$4,$5,'ACTIVE')`,
				"vbc_"+branch.id+"_"+capability, branch.business, branch.id,
				"vcp_"+branch.business+"_"+capability, capability); err != nil {
				t.Fatal(err)
			}
		}
	}

	if scopes == nil {
		scopes = fixedScopes{staffA: {Global: true}, staffB: {Global: true}}
	}
	recorder := &recordingAudit{}
	env := &testEnv{pool: pool, audit: recorder}
	// The real transactional outbox, not a stub: "state and event commit
	// together" is only worth asserting against the production publisher.
	env.service = NewService(Options{DB: pool, Scopes: scopes, Audit: recorder, Flags: flags, Media: media,
		Events: events.NewDomainPublisher(events.NewOutboxRepository(pool))})
	return env
}

func allFlagsOn() Flags {
	return Flags{CatalogueEnabled: true, WritesEnabled: true}
}

func actorFor(business, staff string) Actor {
	return Actor{StaffID: staff, VendorID: business, CorrelationID: "corr_food_test"}
}

func globalScope() Scope { return Scope{Global: true} }

// seedMenu creates a DRAFT menu with one section, which is the starting point
// for most item-level tests.
func (e *testEnv) seedMenu(t *testing.T, business, staff, branch, capability string) Menu {
	t.Helper()
	menu, err := e.service.CreateMenu(context.Background(), actorFor(business, staff), globalScope(),
		CreateMenuInput{BranchID: branch, CapabilityType: capability, Name: "Menu " + branch + " " + capability})
	if err != nil {
		t.Fatalf("seed menu: %v", err)
	}
	return menu
}

func (e *testEnv) seedSection(t *testing.T, business, staff string, menu Menu, name string) Section {
	t.Helper()
	section, err := e.service.CreateSection(context.Background(), actorFor(business, staff), globalScope(),
		menu.ID, CreateSectionInput{Name: name})
	if err != nil {
		t.Fatalf("seed section %s: %v", name, err)
	}
	return section
}

func (e *testEnv) seedItem(t *testing.T, business, staff string, menu Menu, section Section, name string, price int64) Item {
	t.Helper()
	item, err := e.service.CreateItem(context.Background(), actorFor(business, staff), globalScope(),
		menu.ID, section.ID, CreateItemInput{Name: name, BasePriceMinor: price})
	if err != nil {
		t.Fatalf("seed item %s: %v", name, err)
	}
	return item
}

func (e *testEnv) seedGroup(t *testing.T, business, staff, name string, min, max int) ModifierGroup {
	t.Helper()
	group, err := e.service.CreateModifierGroup(context.Background(), actorFor(business, staff), globalScope(),
		CreateModifierGroupInput{Name: name, MinSelect: min, MaxSelect: max})
	if err != nil {
		t.Fatalf("seed modifier group %s: %v", name, err)
	}
	return group
}

// auditActions lists the recorded audit actions from the database, which is
// where Section 9A writes them (inside the mutation's transaction).
func (e *testEnv) auditActions(t *testing.T, aggregateID string) []string {
	t.Helper()
	rows, err := e.pool.Query(context.Background(),
		`SELECT action FROM vendor_audit_log WHERE aggregate_id=$1 ORDER BY occurred_at, id`, aggregateID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var actions []string
	for rows.Next() {
		var action string
		if err = rows.Scan(&action); err != nil {
			t.Fatal(err)
		}
		actions = append(actions, action)
	}
	return actions
}

func (e *testEnv) outboxTypes(t *testing.T, aggregateID string) []string {
	t.Helper()
	rows, err := e.pool.Query(context.Background(),
		`SELECT event_type FROM vendor_outbox WHERE aggregate_id=$1 ORDER BY created_at, id`, aggregateID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var eventType string
		if err = rows.Scan(&eventType); err != nil {
			t.Fatal(err)
		}
		types = append(types, eventType)
	}
	return types
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func int64Ptr(v int64) *int64    { return &v }
func stringPtr(v string) *string { return &v }
