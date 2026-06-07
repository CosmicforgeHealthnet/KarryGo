package migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestVehicleMigrationFilesExist(t *testing.T) {
	required := []string{
		"000014_create_bikes.up.sql",
		"000014_create_bikes.down.sql",
		"000015_create_bike_documents.up.sql",
		"000015_create_bike_documents.down.sql",
		"000016_create_bike_audit.up.sql",
		"000016_create_bike_audit.down.sql",
	}
	for _, name := range required {
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(".", name)); err != nil {
				t.Fatalf("vehicle migration %s is missing: %v", name, err)
			}
		})
	}
}

func TestVehicleMigrationsApplyValidateAndRollback(t *testing.T) {
	databaseURL := os.Getenv("DISPATCH_VEHICLE_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("TEST_DATABASE_URL")
	}
	if databaseURL == "" {
		t.Skip("set DISPATCH_VEHICLE_TEST_DATABASE_URL or TEST_DATABASE_URL to run vehicle migration checks")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer conn.Release()

	schema := "vehicle_migration_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE") //nolint:errcheck

	if _, err := conn.Exec(ctx, "SET search_path TO "+schema+", public"); err != nil {
		t.Fatalf("set search_path: %v", err)
	}

	// Apply base migrations required by FK references.
	for _, name := range []string{
		"000004_create_providers.up.sql",
		"000005_create_emergency_contacts.up.sql",
		"000006_create_guarantors.up.sql",
		"000007_create_ratings.up.sql",
	} {
		execMigrationFile(t, ctx, conn, name)
	}

	// Apply vehicle migrations.
	execMigrationFile(t, ctx, conn, "000014_create_bikes.up.sql")
	execMigrationFile(t, ctx, conn, "000015_create_bike_documents.up.sql")
	execMigrationFile(t, ctx, conn, "000016_create_bike_audit.up.sql")

	// Verify tables exist.
	for _, tbl := range []string{"bikes", "bike_documents", "bike_audit"} {
		assertVehicleTableExists(t, ctx, conn, schema, tbl)
	}

	// Required indexes.
	for _, idx := range []string{
		"idx_bikes_provider",
		"idx_bikes_status",
		"idx_bike_docs_bike",
		"idx_bike_docs_provider",
		"idx_bike_audit_bike",
		"idx_bike_audit_provider",
	} {
		assertIndexExists(t, ctx, conn, schema, idx)
	}

	// bikes.bike_type CHECK rejects invalid value.
	providerID := insertVehicleTestProvider(t, ctx, conn)

	_, errType := conn.Exec(ctx,
		`INSERT INTO bikes (provider_id, bike_type, brand, model, year, color, plate_number)
		 VALUES ($1, 'truck', 'X', 'Y', 2020, 'Black', 'TST-CHK-01')`,
		providerID)
	if errType == nil {
		t.Fatal("bikes bike_type CHECK should reject 'truck'")
	}

	// bikes.verification_status CHECK rejects invalid value.
	_, errStatus := conn.Exec(ctx,
		`INSERT INTO bikes (provider_id, bike_type, brand, model, year, color, plate_number, verification_status)
		 VALUES ($1, 'motorcycle', 'X', 'Y', 2020, 'Black', 'TST-CHK-02', 'bad_status')`,
		providerID)
	if errStatus == nil {
		t.Fatal("bikes verification_status CHECK should reject 'bad_status'")
	}

	// Insert a valid bike.
	var bikeID string
	if err := conn.QueryRow(ctx,
		`INSERT INTO bikes (provider_id, bike_type, brand, model, year, color, plate_number)
		 VALUES ($1, 'motorcycle', 'Honda', 'CB125F', 2022, 'Red', 'TST-VLD-01')
		 RETURNING id::text`,
		providerID,
	).Scan(&bikeID); err != nil {
		t.Fatalf("insert valid bike: %v", err)
	}

	// bikes.plate_number uniqueness.
	_, errPlate := conn.Exec(ctx,
		`INSERT INTO bikes (provider_id, bike_type, brand, model, year, color, plate_number)
		 VALUES ($1, 'motorcycle', 'Honda', 'CB125F', 2022, 'Red', 'TST-VLD-01')`,
		providerID)
	if errPlate == nil {
		t.Fatal("bikes plate_number UNIQUE should reject duplicate")
	}

	// bike_documents.document_type CHECK rejects invalid value.
	_, errDoc := conn.Exec(ctx,
		`INSERT INTO bike_documents (bike_id, provider_id, document_type, file_url)
		 VALUES ($1, $2, 'passport', 'local-private://test')`,
		bikeID, providerID)
	if errDoc == nil {
		t.Fatal("bike_documents document_type CHECK should reject 'passport'")
	}

	// bike_audit.action CHECK rejects invalid value.
	_, errAction := conn.Exec(ctx,
		`INSERT INTO bike_audit (bike_id, provider_id, action, from_status, to_status)
		 VALUES ($1, $2, 'bad_action', 'unverified', 'unverified')`,
		bikeID, providerID)
	if errAction == nil {
		t.Fatal("bike_audit action CHECK should reject 'bad_action'")
	}

	// bike_audit.from_status CHECK rejects invalid value.
	_, errFrom := conn.Exec(ctx,
		`INSERT INTO bike_audit (bike_id, provider_id, action, from_status, to_status)
		 VALUES ($1, $2, 'registered', 'bad_status', 'unverified')`,
		bikeID, providerID)
	if errFrom == nil {
		t.Fatal("bike_audit from_status CHECK should reject 'bad_status'")
	}

	// Insert valid audit row.
	if _, err := conn.Exec(ctx,
		`INSERT INTO bike_audit (bike_id, provider_id, action, from_status, to_status)
		 VALUES ($1, $2, 'registered', 'unverified', 'unverified')`,
		bikeID, providerID); err != nil {
		t.Fatalf("insert valid audit row: %v", err)
	}

	// ── Rollback (reverse order) ──────────────────────────────────────────────
	execMigrationFile(t, ctx, conn, "000016_create_bike_audit.down.sql")
	execMigrationFile(t, ctx, conn, "000015_create_bike_documents.down.sql")
	execMigrationFile(t, ctx, conn, "000014_create_bikes.down.sql")

	for _, tbl := range []string{"bikes", "bike_documents", "bike_audit"} {
		assertTableDropped(t, ctx, conn, schema, tbl)
	}
}

func insertVehicleTestProvider(t *testing.T, ctx context.Context, conn *pgxpool.Conn) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := conn.Exec(ctx,
		`INSERT INTO providers (id, phone, country, verification_status, is_active, onboarding_complete)
		 VALUES ($1, $2, 'NG', 'unverified', true, false)`,
		id, "+2348"+strings.ReplaceAll(uuid.NewString()[:8], "-", ""),
	); err != nil {
		t.Fatalf("insert test provider: %v", err)
	}
	return id
}

func assertVehicleTableExists(t *testing.T, ctx context.Context, conn *pgxpool.Conn, schema, table string) {
	t.Helper()
	var exists bool
	if err := conn.QueryRow(ctx,
		`SELECT to_regclass($1) IS NOT NULL`, schema+"."+table,
	).Scan(&exists); err != nil {
		t.Fatalf("query table %s.%s: %v", schema, table, err)
	}
	if !exists {
		t.Fatalf("table %s.%s does not exist after migration", schema, table)
	}
}
