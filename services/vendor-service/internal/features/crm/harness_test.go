package crm

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
)

// fixedScopes is a test ScopeResolver: it returns whatever scope the test
// declares for a staff id, so authorization behaviour can be exercised without
// standing up the whole Section 5 IAM graph. The real adapter over the
// Section 5 evaluator is wired in cmd/adapters.go and covered by the HTTP
// contract test.
type fixedScopes map[string]Scope

func (f fixedScopes) ResolveScope(_ context.Context, staffID, _ string) (Scope, error) {
	return f[staffID], nil
}

// recordingAudit captures audit entries so tests can assert that privacy and
// merge decisions are actually recorded.
type recordingAudit struct{ entries []AuditEntry }

func (r *recordingAudit) Record(_ context.Context, entry AuditEntry) error {
	r.entries = append(r.entries, entry)
	return nil
}

type testEnv struct {
	pool    *pgxpool.Pool
	service *Service
	audit   *recordingAudit
	now     time.Time
}

const (
	businessA = "vendor_crm_a"
	businessB = "vendor_crm_b"
	branchA1  = "brn_a1"
	branchA2  = "brn_a2"
	branchB1  = "brn_b1"
)

// newTestEnv builds a real-Postgres environment with two businesses, so every
// tenancy assertion is against genuinely separate tenants rather than a mock.
var (
	crmTestMigrationsOnce sync.Once
	crmTestMigrationsErr  error
)

func ensureCRMTestMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	crmTestMigrationsOnce.Do(func() {
		crmTestMigrationsErr = database.ApplyMigrations(
			ctx,
			pool,
			"../../../migrations",
		)
	})
	return crmTestMigrationsErr
}

func newTestEnv(t *testing.T, flags Flags) *testEnv {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres CRM test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	// Every connection this pool opens is tagged with crmTestAppName so the
	// concurrency tests can scope their lock observation to this test binary's
	// own backends. Without the tag the only way to find a blocked transaction
	// is to search the whole server, which is how those tests used to be
	// satisfied by unrelated backends. See concurrency_observation_test.go.
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	poolConfig.ConnConfig.RuntimeParams["application_name"] = crmTestAppName

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = ensureCRMTestMigrations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	// Clears exactly what the previous TRUNCATE ... CASCADE cleared, but only
	// touches tables that actually hold rows. See fixture_reset_test.go.
	resetCRMFixture(ctx, t, pool)

	// CRM harness cleanup must preserve platform-owned and other-package seed
	// data. Never TRUNCATE shared Vendor authority/root tables with CASCADE.
	// Delete only the two CRM test tenants, one statement at a time because
	// pgx parameterized execution does not permit multiple SQL commands in a
	// single prepared statement.
	if _, err = pool.Exec(ctx, `
        DELETE FROM vendor_staff_role_assignments
        WHERE staff_id IN (
            SELECT id
            FROM vendor_staff
            WHERE vendor_id IN ($1, $2)
        )
    `, businessA, businessB); err != nil {
		t.Fatal(err)
	}

	if _, err = pool.Exec(ctx, `
        DELETE FROM vendor_branches
        WHERE business_id IN ($1, $2)
    `, businessA, businessB); err != nil {
		t.Fatal(err)
	}

	if _, err = pool.Exec(ctx, `
        DELETE FROM vendor_businesses
        WHERE id IN ($1, $2)
    `, businessA, businessB); err != nil {
		t.Fatal(err)
	}

	if _, err = pool.Exec(ctx, `
        DELETE FROM vendor_staff
        WHERE vendor_id IN ($1, $2)
    `, businessA, businessB); err != nil {
		t.Fatal(err)
	}

	if _, err = pool.Exec(ctx, `
        DELETE FROM vendor_users
        WHERE id IN ('vu_a', 'vu_b')
    `); err != nil {
		t.Fatal(err)
	}

	for _, business := range []struct{ id, staff, user string }{
		{businessA, "vs_a", "vu_a"},
		{businessB, "vs_b", "vu_b"},
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
	for _, assignment := range []struct{ id, staff string }{{"vra_a", "vs_a"}, {"vra_b", "vs_b"}} {
		if _, err = pool.Exec(ctx, `INSERT INTO vendor_staff_role_assignments(id,staff_id,role_id,branch_id)
			VALUES($1,$2,'role_owner',NULL)`, assignment.id, assignment.staff); err != nil {
			t.Fatal(err)
		}
	}

	recorder := &recordingAudit{}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	env := &testEnv{pool: pool, audit: recorder, now: now}
	env.service = NewService(Options{
		DB:     pool,
		Scopes: fixedScopes{"vs_a": {Global: true}, "vs_b": {Global: true}},
		Audit:  recorder,
		Flags:  flags,
		Now:    func() time.Time { return env.now },
	})
	return env
}

// allFlagsOn is the fully-integrated configuration used by most tests.
func allFlagsOn() Flags {
	return Flags{
		CRMEnabled:                    true,
		WritesEnabled:                 true,
		CustomerProjectionEnabled:     true,
		ConsentIntegrationEnabled:     true,
		PrivacyIntegrationEnabled:     true,
		SegmentationEnabled:           true,
		SegmentMaterializationEnabled: true,
		InteractionsEnabled:           true,
		TasksEnabled:                  true,
		LoyaltyEnabled:                true,
		CampaignsEnabled:              true,
		CampaignPreparationEnabled:    true,
		CorporateEnabled:              true,
		CampaignExecutionEnabled:      true,
		IntelligenceEnabled:           true,
		IntelligenceWorkerEnabled:     true,
		ExportsEnabled:                true,
	}
}

func actorFor(business, staff string) Actor {
	return Actor{StaffID: staff, VendorID: business, CorrelationID: "corr_test"}
}

// tx runs fn inside a committed transaction, mirroring how the event ingress
// wraps every handler.
func (e *testEnv) tx(t *testing.T, fn func(tx pgx.Tx) error) error {
	t.Helper()
	ctx := context.Background()
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

// seedProfile creates a relationship the way an authority event would.
func (e *testEnv) seedProfile(t *testing.T, business, staff string, subject SubjectRef) Profile {
	t.Helper()
	var profile Profile
	if err := e.tx(t, func(tx pgx.Tx) error {
		var err error
		profile, err = e.service.ResolveOrCreateProfile(context.Background(), tx, actorFor(business, staff),
			RelationshipInput{Subject: subject, ObservedAt: e.now})
		return err
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	return profile
}

// seedOrder projects one completed order fact.
func (e *testEnv) seedOrder(t *testing.T, business, staff string, profile Profile, ref, branch string, gross, net int64, completedAt time.Time, version int64) {
	t.Helper()
	if err := e.tx(t, func(tx pgx.Tx) error {
		return e.service.ApplyOrderFact(context.Background(), tx, actorFor(business, staff), profile, OrderFactInput{
			SourceService:  "vendor-order-service",
			SourceOrderRef: ref,
			SourceEventID:  "evt_" + ref,
			SourceVersion:  version,
			OrderState:     OrderCompleted,
			CompletedAt:    &completedAt,
			Currency:       "NGN",
			GrossMinor:     gross,
			NetMinor:       net,
			BranchID:       &branch,
			CapabilityType: ptr("RESTAURANT"),
			ObservedAt:     e.now,
		})
	}); err != nil {
		t.Fatalf("seed order %s: %v", ref, err)
	}
}

func (e *testEnv) reload(t *testing.T, business, profileID string) Profile {
	t.Helper()
	profile, err := scanProfile(e.pool.QueryRow(context.Background(),
		`SELECT `+profileCols+` FROM vendor_customer_profiles WHERE business_id=$1 AND id=$2`, business, profileID))
	if err != nil {
		t.Fatalf("reload profile: %v", err)
	}
	return profile
}

func customerSubject(ref string) SubjectRef { return SubjectRef{Type: SubjectCustomer, Ref: ref} }
func guestSubject(ref string) SubjectRef    { return SubjectRef{Type: SubjectGuest, Ref: ref} }
