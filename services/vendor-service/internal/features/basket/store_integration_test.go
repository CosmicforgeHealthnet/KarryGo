package basket

// PostgreSQL-backed certification.
//
// Everything asserted here is a guarantee the DATABASE carries, not one the
// Go code carries, so it can only be proved against a real database. The
// in-memory tests elsewhere in this package prove that the service asks for
// the right thing; these prove that asking twice, from two processes, gets
// one answer.
//
// Skipped without VENDOR_SERVICE_TEST_DATABASE_URL, matching the convention
// every other Vendor Service integration test follows. A skip is reported as
// a skip - never as a pass - because "exactly once" asserted by a test that
// did not run is not an assertion.

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")

	if dsn == "" {
		t.Skip(
			"VENDOR_SERVICE_TEST_DATABASE_URL not set; " +
				"skipping real-Postgres basket certification",
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	t.Cleanup(pool.Close)

	return pool
}

// seedTemplate writes a minimal template so generations have something to
// reference. The composite foreign key means an occurrence cannot exist
// without one, which is itself part of the isolation model.
func seedTemplate(
	t *testing.T,
	pool *pgxpool.Pool,
	customerID string,
) string {
	t.Helper()

	id := "rbt_" + uuid.NewString()

	_, err := pool.Exec(context.Background(), `
		INSERT INTO commerce_recurring_basket_templates (
			id, customer_id, name, currency, address_ref, requirements,
			cadence, timezone, local_hour, local_minute, weekday
		) VALUES ($1,$2,'Weekly','NGN','addr_1','[]'::jsonb,
		          'WEEKLY','Africa/Lagos',9,0,2)`,
		id, customerID,
	)
	if err != nil {
		t.Fatalf("seed template: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM commerce_recurring_basket_templates WHERE id = $1`,
			id,
		)
	})

	return id
}

// ---------------------------------------------------------------------------
// EXACTLY ONCE
// ---------------------------------------------------------------------------

// THE MANDATORY CONCURRENCY TEST.
//
// Sixteen workers race for one scheduled occurrence. Exactly one may create
// it; every other must read back the winner's row rather than making its own.
// This is the guarantee an in-memory lock cannot give, because every failure
// mode it protects against involves more than one process.
func TestOccurrence_ConcurrentWorkersProduceOneGeneration(t *testing.T) {
	pool := testPool(t)

	customerID := "cust_" + uuid.NewString()
	templateID := seedTemplate(t, pool, customerID)

	store := NewStore(pool)

	occurrenceAt := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	key := "2026-09-01T09:00@Africa/Lagos"

	const workers = 16

	var (
		wait    sync.WaitGroup
		mutex   sync.Mutex
		created int
		ids     = map[string]bool{}
	)

	wait.Add(workers)

	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wait.Done()

			generation, wasCreated, err := store.ClaimOccurrence(
				context.Background(), Generation{
					TemplateID:    templateID,
					CustomerID:    customerID,
					OccurrenceKey: key,
					OccurrenceAt:  occurrenceAt,
					Timezone:      "Africa/Lagos",
				},
			)

			mutex.Lock()
			defer mutex.Unlock()

			if err != nil {
				t.Errorf("claim: %v", err)

				return
			}

			if wasCreated {
				created++
			}

			ids[generation.ID] = true
		}()
	}

	wait.Wait()

	if created != 1 {
		t.Fatalf("%d workers created a generation, want exactly 1", created)
	}

	// And every worker walked away holding the SAME generation, so a losing
	// worker cannot go on to build a second proposal against its own row.
	if len(ids) != 1 {
		t.Fatalf("workers saw %d distinct generations, want 1", len(ids))
	}

	var rows int

	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM commerce_recurring_basket_generations
		 WHERE template_id = $1 AND occurrence_key = $2`,
		templateID, key,
	).Scan(&rows); err != nil {
		t.Fatal(err)
	}

	if rows != 1 {
		t.Fatalf("%d generation rows exist, want 1", rows)
	}
}

// A crash between claiming and finishing must be RESUMED, not repeated.
//
// The retry finds its own earlier claim and returns it. Creating a second one
// would turn one scheduled delivery into two proposals - and, downstream, two
// orders.
func TestOccurrence_RetryAfterCrashResumesTheSameGeneration(t *testing.T) {
	pool := testPool(t)

	customerID := "cust_" + uuid.NewString()
	templateID := seedTemplate(t, pool, customerID)

	store := NewStore(pool)

	claim := Generation{
		TemplateID:    templateID,
		CustomerID:    customerID,
		OccurrenceKey: "2026-09-08T09:00@Africa/Lagos",
		OccurrenceAt:  time.Date(2026, 9, 8, 8, 0, 0, 0, time.UTC),
		Timezone:      "Africa/Lagos",
	}

	first, created, err := store.ClaimOccurrence(context.Background(), claim)
	if err != nil || !created {
		t.Fatalf("first claim: created=%v err=%v", created, err)
	}

	// The process dies here. The retry presents the same key, because the key
	// is derived from the schedule and nothing volatile.
	second, createdAgain, err := store.ClaimOccurrence(
		context.Background(), claim,
	)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}

	if createdAgain {
		t.Fatal("a retry created a second generation")
	}

	if second.ID != first.ID {
		t.Fatalf("retry got %s, want the original %s", second.ID, first.ID)
	}
}

// ---------------------------------------------------------------------------
// Proposal acceptance
// ---------------------------------------------------------------------------

func seedProposal(
	t *testing.T,
	store *Store,
	customerID string,
) Proposal {
	t.Helper()

	proposal, err := store.CreateProposal(context.Background(), Proposal{
		CustomerID:               customerID,
		BaseCartID:               "cart_" + uuid.NewString(),
		BaseCartVersion:          3,
		Objective:                ObjectiveValue,
		Currency:                 "NGN",
		MerchandiseTotalMinor:    80000,
		DeliveryTotalMinor:       20000,
		FinalDeliveredTotalMinor: 100000,
		VendorCount:              1,
		Fingerprint:              uuid.NewString(),
		ExpiresAt:                time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("seed proposal: %v", err)
	}

	t.Cleanup(func() {
		_, _ = store.db.Exec(context.Background(),
			`DELETE FROM commerce_basket_proposals WHERE id = $1`,
			proposal.ID,
		)
	})

	return proposal
}

// Two concurrent acceptances must produce ONE cart version.
//
// The guard is in the UPDATE's own WHERE clause, so the race is decided by
// the database rather than by a check-then-act the two callers both passed.
func TestProposal_ConcurrentAcceptanceProducesOneCartVersion(t *testing.T) {
	pool := testPool(t)

	store := NewStore(pool)

	customerID := "cust_" + uuid.NewString()
	proposal := seedProposal(t, store, customerID)

	const attempts = 8

	var (
		wait      sync.WaitGroup
		mutex     sync.Mutex
		succeeded int
	)

	wait.Add(attempts)

	for attempt := 0; attempt < attempts; attempt++ {
		version := int64(4 + attempt)

		go func() {
			defer wait.Done()

			err := store.MarkProposalAccepted(
				context.Background(), customerID, proposal.ID,
				version, time.Now().UTC(),
			)

			mutex.Lock()
			defer mutex.Unlock()

			if err == nil {
				succeeded++
			}
		}()
	}

	wait.Wait()

	if succeeded != 1 {
		t.Fatalf("%d acceptances succeeded, want exactly 1", succeeded)
	}

	stored, err := store.GetProposal(
		context.Background(), customerID, proposal.ID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if stored.State != ProposalAccepted {
		t.Fatalf("state = %s, want ACCEPTED", stored.State)
	}

	if stored.AcceptedCartVersion == 0 || stored.AcceptedAt == nil {
		t.Fatal("an accepted proposal must record the version it produced")
	}
}

// ---------------------------------------------------------------------------
// Isolation
// ---------------------------------------------------------------------------

// A proposal id alone must never be enough to read somebody else's shopping.
func TestProposal_IsScopedToItsOwner(t *testing.T) {
	pool := testPool(t)

	store := NewStore(pool)

	owner := "cust_" + uuid.NewString()
	intruder := "cust_" + uuid.NewString()

	proposal := seedProposal(t, store, owner)

	if _, err := store.GetProposal(
		context.Background(), intruder, proposal.ID,
	); err == nil {
		t.Fatal("another customer read this proposal")
	}

	// And cannot accept it either, which is the more dangerous half: reading
	// leaks information, accepting rewrites a stranger's basket.
	if err := store.MarkProposalAccepted(
		context.Background(), intruder, proposal.ID, 4, time.Now().UTC(),
	); err == nil {
		t.Fatal("another customer accepted this proposal")
	}
}

func TestTemplate_IsScopedToItsOwner(t *testing.T) {
	pool := testPool(t)

	owner := "cust_" + uuid.NewString()
	intruder := "cust_" + uuid.NewString()

	templateID := seedTemplate(t, pool, owner)

	store := NewStore(pool)

	if _, err := store.GetTemplate(
		context.Background(), intruder, templateID,
	); err == nil {
		t.Fatal("another customer read this recurring basket")
	}

	if _, err := store.SetTemplateEnabled(
		context.Background(), intruder, templateID, false, 1,
	); err == nil {
		t.Fatal("another customer disabled this recurring basket")
	}
}

// ---------------------------------------------------------------------------
// Template concurrency
// ---------------------------------------------------------------------------

// Two people editing one household's recurring basket must not silently
// overwrite each other.
func TestTemplate_ConcurrentEditsDoNotOverwriteSilently(t *testing.T) {
	pool := testPool(t)

	customerID := "cust_" + uuid.NewString()
	templateID := seedTemplate(t, pool, customerID)

	store := NewStore(pool)

	stored, err := store.GetTemplate(
		context.Background(), customerID, templateID,
	)
	if err != nil {
		t.Fatal(err)
	}

	stored.Name = "First edit"

	if _, err = store.UpdateTemplate(
		context.Background(), stored, stored.Version,
	); err != nil {
		t.Fatalf("first edit: %v", err)
	}

	// The second editor is still holding the version they read. Their write
	// is refused rather than applied over the first one.
	stored.Name = "Second edit"

	if _, err = store.UpdateTemplate(
		context.Background(), stored, stored.Version,
	); err == nil {
		t.Fatal("a stale edit overwrote a newer one")
	}
}

// ---------------------------------------------------------------------------
// Database-enforced invariants
// ---------------------------------------------------------------------------

// The schema, not the service, is what refuses these. A future code path that
// forgets to check must still be unable to write them.
func TestSchema_RefusesIncoherentRows(t *testing.T) {
	pool := testPool(t)

	customerID := "cust_" + uuid.NewString()

	ctx := context.Background()

	// A proposal whose parts do not add up to its whole.
	if _, err := pool.Exec(ctx, `
		INSERT INTO commerce_basket_proposals (
			id, customer_id, objective, currency,
			merchandise_total_minor, delivery_total_minor,
			final_delivered_total_minor, fingerprint, expires_at
		) VALUES ($1,$2,'VALUE','NGN',100,50,999,'fp',now() + interval '1 hour')`,
		"bop_"+uuid.NewString(), customerID,
	); err == nil {
		t.Fatal("a proposal with an incoherent total was accepted")
	}

	// An unbounded standing purchase authority.
	if _, err := pool.Exec(ctx, `
		INSERT INTO commerce_recurring_basket_templates (
			id, customer_id, name, currency, address_ref, requirements,
			approval_policy, ceiling_minor, cadence, timezone,
			local_hour, local_minute, weekday
		) VALUES ($1,$2,'Bad','NGN','addr','[]'::jsonb,
		          'AUTO_IF_UNCHANGED',0,'WEEKLY','Africa/Lagos',9,0,2)`,
		"rbt_"+uuid.NewString(), customerID,
	); err == nil {
		t.Fatal("auto-approval with no ceiling was accepted")
	}

	// A monthly schedule on a day that does not exist in February.
	if _, err := pool.Exec(ctx, `
		INSERT INTO commerce_recurring_basket_templates (
			id, customer_id, name, currency, address_ref, requirements,
			cadence, timezone, local_hour, local_minute, weekday, day_of_month
		) VALUES ($1,$2,'Bad','NGN','addr','[]'::jsonb,
		          'MONTHLY','Africa/Lagos',9,0,2,31)`,
		"rbt_"+uuid.NewString(), customerID,
	); err == nil {
		t.Fatal("a monthly schedule on the 31st was accepted")
	}
}
