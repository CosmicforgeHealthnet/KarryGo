package restrictions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRestrictionProjectionVersionIdempotencyClearAndAppendOnly(t *testing.T) {
	dsn := os.Getenv("DISPATCH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DISPATCH_TEST_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var schemaExists bool
	if err = pool.QueryRow(context.Background(), `SELECT to_regclass('public.dispatch_authority_restrictions') IS NOT NULL`).Scan(&schemaExists); err != nil {
		t.Fatal(err)
	}
	if !schemaExists {
		migration, readErr := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000039_authority_restrictions.up.sql"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = pool.Exec(context.Background(), string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pool.Exec(context.Background(), `TRUNCATE dispatch_authority_restriction_operations,dispatch_authority_restriction_history,dispatch_authority_restrictions CASCADE`); err != nil {
		t.Fatal(err)
	}
	s := New(pool)
	org := "org_" + uuid.NewString()
	apply := Command{OperationReference: "apply-1", OrganizationID: org, SubjectType: "PROVIDER", SubjectReference: "membership-1", ReasonCode: "NON_COMPLIANT", SourceReference: "fleet-r1", SourceVersion: 1}
	first, err := s.Apply(context.Background(), apply)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Apply(context.Background(), apply)
	if err != nil || replay != first {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	changed := apply
	changed.ReasonCode = "OTHER"
	if _, err = s.Apply(context.Background(), changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflict err=%v", err)
	}
	restricted, err := s.Restricted(context.Background(), org, "PROVIDER", "membership-1")
	if err != nil || !restricted {
		t.Fatalf("restricted=%v err=%v", restricted, err)
	}
	clear := apply
	clear.OperationReference = "clear-1"
	clear.SourceVersion = 2
	clear.ReasonCode = "AUTHORIZED_CLEAR"
	if _, err = s.Clear(context.Background(), clear); err != nil {
		t.Fatal(err)
	}
	restricted, err = s.Restricted(context.Background(), org, "PROVIDER", "membership-1")
	if err != nil || restricted {
		t.Fatalf("restricted after clear=%v err=%v", restricted, err)
	}
	if _, err = pool.Exec(context.Background(), `UPDATE dispatch_authority_restriction_history SET reason_code='tampered'`); err == nil {
		t.Fatal("history update must be rejected")
	}
}

func TestRestrictionMigrationRollbackReplay(t *testing.T) {
	dsn := os.Getenv("DISPATCH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DISPATCH_TEST_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(context.Background(), `SELECT pg_advisory_lock(hashtext('dispatch-restriction-migration-test'))`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtext('dispatch-restriction-migration-test'))`)
	}()
	down, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000039_authority_restrictions.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000039_authority_restrictions.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(context.Background(), string(down)); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err = pool.QueryRow(context.Background(), `SELECT to_regclass('public.dispatch_authority_restrictions') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatalf("restriction table after rollback exists=%v err=%v", exists, err)
	}
	if _, err = pool.Exec(context.Background(), string(up)); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(context.Background(), `SELECT to_regclass('public.dispatch_authority_restrictions') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("restriction table after replay exists=%v err=%v", exists, err)
	}
}
