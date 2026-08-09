package database

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOrganizationCaseMigration(t *testing.T) {
	dsn := os.Getenv("SUPPORT_DISPUTE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set SUPPORT_DISPUTE_TEST_DATABASE_URL")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()
	if err = ApplyMigrations(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"organization_reference", "source_idempotency_key", "source_version"} {
		var exists bool
		if err = pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='complaints' AND column_name=$1)`, column).Scan(&exists); err != nil || !exists {
			t.Fatalf("column %s exists=%v err=%v", column, exists, err)
		}
	}
}
