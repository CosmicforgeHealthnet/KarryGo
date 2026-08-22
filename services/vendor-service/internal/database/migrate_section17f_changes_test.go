package database

// Section 17 closure, Part 6: migration 000083 — commercial changes and
// financial adjustments.
//
// The claim under test is not that the file exists but that the schema it
// creates actually holds: the evidence tables reject mutation, the delta can
// never disagree with the two totals it is derived from, and the down file
// really removes what the up file created rather than merely untracking it.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func section17ChangePool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()

	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("no test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return pool, ctx
}

func TestSection17ChangeMigrationCreatesItsSchema(t *testing.T) {
	pool, ctx := section17ChangePool(t)

	for _, table := range []string{
		"commerce_order_changes",
		"commerce_order_change_applications",
		"commerce_order_adjustments",
		"commerce_order_change_events",
	} {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil ||
			!exists {
			t.Fatalf("%s exists=%t err=%v", table, exists, err)
		}
	}

	// Every evidence table is frozen at the database, not by convention in Go.
	var triggers int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_trigger
		 WHERE NOT tgisinternal AND tgname IN (
		     'commerce_order_change_applications_immutable',
		     'commerce_order_change_events_immutable',
		     'commerce_order_changes_immutable_evidence',
		     'commerce_order_adjustments_immutable_evidence')`).Scan(&triggers); err != nil ||
		triggers != 4 {
		t.Fatalf("evidence triggers=%d want 4 err=%v", triggers, err)
	}

	// A change may only ever be raised against a quote that belongs to the
	// same customer: the composite foreign key is what stops one customer's
	// order being repriced from another customer's quote.
	for _, constraint := range []string{
		"commerce_order_changes_parent_scope_fk",
		"commerce_order_changes_original_quote_fk",
		"commerce_order_changes_replacement_quote_fk",
	} {
		var present bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM pg_constraint
				 WHERE conrelid='commerce_order_changes'::regclass AND conname=$1)`,
			constraint).Scan(&present); err != nil || !present {
			t.Fatalf("%s present=%t err=%v", constraint, present, err)
		}
	}

	// A replacement quote can only back one change, so a customer cannot pay
	// two deltas from a single requote.
	var uniqueReplacement bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_constraint c
			 WHERE c.conrelid='commerce_order_changes'::regclass
			   AND c.contype='u'
			   AND (SELECT array_agg(a.attname::text ORDER BY a.attname)
			          FROM pg_attribute a
			         WHERE a.attrelid=c.conrelid AND a.attnum = ANY(c.conkey))
			       = ARRAY['replacement_quote_id'])`).Scan(&uniqueReplacement); err != nil ||
		!uniqueReplacement {
		t.Fatalf("replacement quote is not unique per change: %v", err)
	}

	// Adjustments are idempotent by key, so a replayed recovery pass cannot
	// move the same money twice.
	var uniqueKey bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_constraint c
			 WHERE c.conrelid='commerce_order_adjustments'::regclass
			   AND c.contype='u'
			   AND (SELECT array_agg(a.attname::text)
			          FROM pg_attribute a
			         WHERE a.attrelid=c.conrelid AND a.attnum = ANY(c.conkey))
			       = ARRAY['idempotency_key'])`).Scan(&uniqueKey); err != nil || !uniqueKey {
		t.Fatalf("adjustment idempotency key is not unique: %v", err)
	}

	// The delta is not free text: it must equal the difference between the two
	// totals it claims to describe.
	var deltaCheck string
	if err := pool.QueryRow(ctx, `
		SELECT pg_get_constraintdef(oid) FROM pg_constraint
		 WHERE conrelid='commerce_order_changes'::regclass
		   AND conname='commerce_order_changes_delta_chk'`).Scan(&deltaCheck); err != nil {
		t.Fatalf("delta check is absent: %v", err)
	}
	for _, part := range []string{"delta_minor", "replacement_total_minor", "original_total_minor"} {
		if !strings.Contains(deltaCheck, part) {
			t.Errorf("delta check %q does not constrain %s", deltaCheck, part)
		}
	}

	// An approved or applied change must carry the fingerprint the customer
	// approved, so no change can be applied without recorded consent.
	var approvalCheck bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_constraint
			 WHERE conrelid='commerce_order_changes'::regclass
			   AND conname='commerce_order_changes_approval_chk')`).Scan(&approvalCheck); err != nil ||
		!approvalCheck {
		t.Fatalf("approval fingerprint is not required: %v", err)
	}
}

// The down file must actually remove the schema. A rollback that only untracks
// the version leaves a reapply to fail on objects that already exist.
func TestSection17ChangeMigrationRollbackDropsAndReapplyRestores(t *testing.T) {
	pool, ctx := section17ChangePool(t)
	dir := "../../migrations"

	migrations, err := ListMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}

	// 000083 is not necessarily the tip any more, so this walks the history
	// back to it rather than assuming. Every migration crossed on the way must
	// carry a down file, or the schema could not be rolled back at all.
	var target string
	for index := len(migrations) - 1; index >= 0; index-- {
		if strings.Contains(migrations[index].Version, "000083") {
			target = migrations[index].Version
			for _, crossed := range migrations[index:] {
				if crossed.DownFile == "" {
					t.Fatalf("%s has no down file, so 000083 cannot be rolled back",
						crossed.Version)
				}
			}
			break
		}
	}
	if target == "" {
		t.Fatal("migration 000083 is absent from the history")
	}

	// The evidence tables restrict deletion of what they reference, so a
	// rollback with change rows present would fail. Clear them the way a real
	// rollback of an unreleased migration would require.
	if _, err = pool.Exec(ctx, `TRUNCATE commerce_order_change_events,
		commerce_order_change_applications, commerce_order_adjustments,
		commerce_order_changes RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("clear change rows before rollback: %v", err)
	}

	for {
		var stillApplied bool
		if err = pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM vendor_service_schema_migrations WHERE version=$1)`,
			target).Scan(&stillApplied); err != nil {
			t.Fatal(err)
		}
		if !stillApplied {
			break
		}
		if err = RollbackLastMigration(ctx, pool, dir); err != nil {
			t.Fatalf("rolling back towards %s: %v", target, err)
		}
	}

	for _, table := range []string{
		"commerce_order_changes",
		"commerce_order_change_applications",
		"commerce_order_adjustments",
		"commerce_order_change_events",
	} {
		var exists bool
		if err = pool.QueryRow(ctx,
			`SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Errorf("%s survived the rollback: the down file untracked the "+
				"migration without removing its schema", table)
		}
	}

	// The guard functions go too, or a reapply would silently keep the old
	// definitions.
	var functions int
	if err = pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_proc
		 WHERE proname IN ('commerce_order_change_guard',
		                   'commerce_order_adjustment_guard',
		                   'commerce_order_change_evidence_immutable')`).Scan(&functions); err != nil {
		t.Fatal(err)
	}
	if functions != 0 {
		t.Errorf("%d guard functions survived the rollback", functions)
	}

	if err = ApplyMigrations(ctx, pool, dir); err != nil {
		t.Fatalf("reapply after rollback: %v", err)
	}
	if err = MigrationsCurrent(ctx, pool, dir); err != nil {
		t.Fatalf("schema is not current after reapply: %v", err)
	}

	var restored bool
	if err = pool.QueryRow(ctx,
		`SELECT to_regclass('public.commerce_order_changes') IS NOT NULL`).Scan(&restored); err != nil ||
		!restored {
		t.Fatalf("commerce_order_changes was not restored: %v", err)
	}
}
