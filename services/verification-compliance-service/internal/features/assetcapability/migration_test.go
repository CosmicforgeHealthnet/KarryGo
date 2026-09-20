package assetcapability

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/verification-compliance-service/internal/database"
)

// TestSection12MigrationRollbackAndReapply proves 000003 is reversible and
// repeatable, and that reversing it leaves the migrations before it intact.
//
// The interesting part is not that the tables appear and disappear. It is the
// two shared objects 000003 touches: it widens the outbox's delivery_status
// vocabulary and adds a routing column to a table the vendor callback worker is
// already using. A down migration that restored the old CHECK while rows still
// held the new value would fail at exactly the moment an operator needed the
// rollback to work.
func TestSection12MigrationRollbackAndReapply(t *testing.T) {
	dsn := os.Getenv("VERIFICATION_COMPLIANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VERIFICATION_COMPLIANCE_TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err = database.Apply(ctx, pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}

	section12Tables := []string{
		"provider_verification_records", "vehicle_documents", "asset_inspections",
		"asset_capability_cases", "asset_capability_decisions", "service_tier_certifications",
	}
	assertTables := func(want bool, when string) {
		t.Helper()
		for _, table := range section12Tables {
			var exists bool
			if err = pool.QueryRow(ctx,
				`SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if exists != want {
				t.Fatalf("%s exists=%t %s, want %t", table, exists, when, want)
			}
		}
	}
	assertTables(true, "after apply")

	// Seed the states a real rollback would meet: a legacy vendor outbox row,
	// and a Section 12 row parked in the status 000003 introduced.
	if _, err = pool.Exec(ctx,
		`INSERT INTO verification_outbox(aggregate_id,event_type,event_version,payload)
		 VALUES(gen_random_uuid(),'vendor.verification.status_changed',1,'{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx,
		`INSERT INTO verification_outbox(aggregate_id,event_type,event_version,payload,delivery_status,delivery_target)
		 VALUES(gen_random_uuid(),$1,1,'{}','NOT_REQUIRED','')`,
		EventCapabilityStatusChanged); err != nil {
		t.Fatal(err)
	}

	down, err := os.ReadFile("../../../migrations/000003_asset_capability_certification.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	assertTables(false, "after rollback")

	// The vendor rows the earlier migrations own survive, and nothing is left
	// holding a status the restored CHECK forbids.
	var remaining int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM verification_outbox`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("outbox rows after rollback = %d, want 2: a rollback must not delete recorded facts", remaining)
	}
	var notRequired int
	if err = pool.QueryRow(ctx,
		`SELECT count(*) FROM verification_outbox WHERE delivery_status='NOT_REQUIRED'`).Scan(&notRequired); err != nil {
		t.Fatal(err)
	}
	if notRequired != 0 {
		t.Fatalf("%d rows still hold NOT_REQUIRED after rollback; the restored CHECK forbids it", notRequired)
	}
	var routingColumn bool
	if err = pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM information_schema.columns
		  WHERE table_name='verification_outbox' AND column_name='delivery_target')`).Scan(&routingColumn); err != nil {
		t.Fatal(err)
	}
	if routingColumn {
		t.Fatal("delivery_target survived rollback")
	}

	up, err := os.ReadFile("../../../migrations/000003_asset_capability_certification.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("reapply: %v", err)
	}
	assertTables(true, "after reapply")

	// The constraints the feature's correctness rests on are back.
	var definition string
	if err = pool.QueryRow(ctx,
		`SELECT pg_get_constraintdef(oid) FROM pg_constraint
		  WHERE conname='verification_outbox_delivery_status_check'`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, "'NOT_REQUIRED'") {
		t.Fatalf("delivery_status check lost NOT_REQUIRED after reapply: %s", definition)
	}

	var trigger bool
	if err = pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_trigger
		  WHERE tgname='asset_capability_decisions_append_only' AND NOT tgisinternal)`).Scan(&trigger); err != nil {
		t.Fatal(err)
	}
	if !trigger {
		t.Fatal("the append-only trigger did not survive reapply; decisions would become editable")
	}
}
