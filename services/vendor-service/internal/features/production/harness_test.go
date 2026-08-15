package production

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
// standing up the whole Section 5 IAM graph. The real adapter over the Section 5
// evaluator is wired in cmd/adapters.go, and the permission-to-role mapping is
// asserted against the real migration seed in the IAM test.
type fixedScopes map[string]Scope

func (f fixedScopes) ResolveScope(_ context.Context, staffID, _ string) (Scope, error) {
	return f[staffID], nil
}

// scopesByPermission answers per permission, so a test can give one staff member
// operate-without-manage (the kitchen role's real shape).
type scopesByPermission map[string]map[string]Scope

func (s scopesByPermission) ResolveScope(_ context.Context, staffID, permission string) (Scope, error) {
	return s[staffID][permission], nil
}

type recordingAudit struct{ entries []AuditEntry }

func (r *recordingAudit) Record(_ context.Context, entry AuditEntry) error {
	r.entries = append(r.entries, entry)
	return nil
}

const (
	businessA = "vendor_prod_a"
	businessB = "vendor_prod_b"
	branchA1  = "brn_prod_a1"
	branchA2  = "brn_prod_a2"
	branchB1  = "brn_prod_b1"
	staffA    = "vs_prod_a"
	staffB    = "vs_prod_b"
)

type testEnv struct {
	pool    *pgxpool.Pool
	service *Service
	audit   *recordingAudit
	now     time.Time
}

// newTestEnv builds a real-Postgres environment with two businesses, three
// branches and both production capabilities granted, so every tenancy and
// capacity assertion is against genuinely separate tenants rather than a mock.
func newTestEnv(t *testing.T, flags Flags, scopes ScopeResolver) *testEnv {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres production test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = database.ApplyMigrations(ctx, pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `TRUNCATE production_calendar_revisions, production_job_events,
		production_jobs, capacity_allocation_adjustments, capacity_reservation_allocations,
		capacity_reservations, production_blackouts, production_slots, production_slot_templates,
		production_capacity_rules, production_lines, production_capacity_pools,
		vendor_outbox, vendor_audit_log, vendor_idempotency_keys,
		vendor_branch_capabilities, vendor_capabilities, vendor_branches, vendor_businesses,
		vendor_staff_role_assignments, vendor_staff, vendor_users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}

	for _, business := range []struct{ id, staff, user string }{
		{businessA, staffA, "vu_prod_a"},
		{businessB, staffB, "vu_prod_b"},
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
	for _, business := range []string{businessA, businessB} {
		for _, capability := range []string{"CLOUD_KITCHEN", "PASTRY_BAKERY"} {
			if _, err = pool.Exec(ctx, `INSERT INTO vendor_capabilities(id,business_id,capability_type,status)
				VALUES($1,$2,$3,'ACTIVE')`, "vcp_"+business+"_"+capability, business, capability); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, branch := range []struct{ id, business string }{
		{branchA1, businessA}, {branchA2, businessA}, {branchB1, businessB},
	} {
		for _, capability := range []string{"CLOUD_KITCHEN", "PASTRY_BAKERY"} {
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
	env := &testEnv{pool: pool, audit: recorder, now: time.Now().UTC()}
	// The real transactional outbox, not a stub: "state and event commit
	// together" is only worth asserting against the production publisher.
	env.service = NewService(Options{DB: pool, Scopes: scopes, Audit: recorder, Flags: flags,
		Events: events.NewDomainPublisher(events.NewOutboxRepository(pool))})
	return env
}

func allFlagsOn() Flags {
	return Flags{ProductionEnabled: true, WritesEnabled: true}
}

func actorFor(business, staff string) Actor {
	return Actor{StaffID: staff, VendorID: business, CorrelationID: "corr_prod_test"}
}

func globalScope() Scope { return Scope{Global: true} }

// seedPool inserts a capacity pool directly, so the concurrency gate can be
// proven before the pool management surface exists. Pool CRUD is exercised by
// its own tests.
func (e *testEnv) seedPool(t *testing.T, business, branch, capability, code string, capacity int) string {
	t.Helper()
	return e.seedPoolWithBuffers(t, business, branch, capability, code, capacity, 0, 0)
}

func (e *testEnv) seedPoolWithBuffers(t *testing.T, business, branch, capability, code string, capacity, setup, cleanup int) string {
	t.Helper()
	id := "cpool_" + business + "_" + branch + "_" + code
	if _, err := e.pool.Exec(context.Background(), `
		INSERT INTO production_capacity_pools
			(id,business_id,branch_id,capability,name,code,scope_type,unit_type,
			 default_capacity,setup_buffer_minutes,cleanup_buffer_minutes,status)
		VALUES ($1,$2,$3,$4,$5,$5,'CAPABILITY','PRODUCTION_UNIT',$6,$7,$8,'ACTIVE')`,
		id, business, branch, capability, code, capacity, setup, cleanup); err != nil {
		t.Fatalf("seed pool %s: %v", code, err)
	}
	return id
}

// window returns a deterministic future production window.
func testWindow() (time.Time, time.Time) {
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	return start, start.Add(2 * time.Hour)
}

func (e *testEnv) reserve(business, staff, branch, capability string, allocations []AllocationRequest, hold time.Duration) (Reservation, error) {
	start, end := testWindow()
	reservation, _, err := e.service.Reserve(context.Background(), actorFor(business, staff), globalScope(),
		ReserveInput{BranchID: branch, Capability: capability, WindowStart: start, WindowEnd: end,
			Allocations: allocations, SourceType: "MANUAL", HoldFor: hold})
	return reservation, err
}

func (e *testEnv) reserveAt(business, staff, branch, capability string, start, end time.Time, allocations []AllocationRequest, hold time.Duration) (Reservation, error) {
	reservation, _, err := e.service.Reserve(context.Background(), actorFor(business, staff), globalScope(),
		ReserveInput{BranchID: branch, Capability: capability, WindowStart: start, WindowEnd: end,
			Allocations: allocations, SourceType: "MANUAL", HoldFor: hold})
	return reservation, err
}

func alloc(poolID string, quantity int) []AllocationRequest {
	return []AllocationRequest{{CapacityPoolID: poolID, Quantity: quantity}}
}
