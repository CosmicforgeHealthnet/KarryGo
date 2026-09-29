package database

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCustomer14ReplacementMigrationRoundTrip(t *testing.T) {
	dsn := os.Getenv("CUSTOMER14_MIGRATION_ROUNDTRIP_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated disposable migration database not configured")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = ApplyMigrations(ctx, db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	var tip string
	if err = db.QueryRow(ctx, `SELECT version FROM vendor_service_schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&tip); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	latest := ""
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".up.sql") {
			version := strings.TrimSuffix(entry.Name(), ".up.sql")
			if version > latest {
				latest = version
			}
		}
	}
	if tip != latest {
		t.Fatalf("unexpected migration tip: %s, want %s", tip, latest)
	}
	// Later §14 migrations must be unwound before testing the replacement
	// migration's rollback. This database must be disposable and contain no
	// replacement provenance, which rollback deliberately refuses to discard.
	for tip >= "000124_vendor_replacement_checkout_evidence" {
		if err = RollbackLastMigration(ctx, db, "../../migrations"); err != nil {
			t.Fatal(err)
		}
		if err = db.QueryRow(ctx, `SELECT version FROM vendor_service_schema_migrations ORDER BY version DESC LIMIT 1`).Scan(&tip); err != nil {
			t.Fatal(err)
		}
	}
	var present bool
	if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='commerce_checkout_sessions' AND column_name='replacement_key')`).Scan(&present); err != nil {
		t.Fatal(err)
	}
	if present {
		t.Fatal("replacement column survived rollback")
	}
	if err = ApplyMigrations(ctx, db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
}
