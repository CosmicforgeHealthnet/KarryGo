package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestOutboxGuardRestoreSafeMigrationShape(t *testing.T) {
	up, err := os.ReadFile("000067_outbox_guard_restore_safe.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	down, err := os.ReadFile("000067_outbox_guard_restore_safe.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(up), "ALTER FUNCTION dispatch_outbox_payload_is_safe(JSONB) SET search_path FROM CURRENT") {
		t.Fatal("up migration must pin the guard's search_path")
	}
	if !strings.Contains(string(down), "RESET search_path") {
		t.Fatal("down migration must reset the pin")
	}
}

// pg_restore evaluates CHECK constraints with an empty search_path. The
// secret guard is recursive, so without a pinned search_path its inner call
// failed and a backup holding outbox rows could not be restored.
func TestOutboxGuardEvaluatesUnderEmptySearchPath(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	var schema string
	if err = conn.QueryRow(ctx, `SELECT n.nspname FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
		WHERE p.proname='dispatch_outbox_payload_is_safe' AND n.nspname=ANY(current_schemas(false)) LIMIT 1`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `SET search_path = ''`); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(context.Background(), `RESET search_path`) //nolint:errcheck
	guard := `"` + schema + `".dispatch_outbox_payload_is_safe($1::jsonb)`
	for payload, want := range map[string]bool{
		`{"status":"READY","nested":{"items":[{"sku":"a"}]}}`: true,
		`{"nested":{"otp":"123456"}}`:                         false,
		`[{"deep":[{"receiver_phone":"+234"}]}]`:              false,
	} {
		var safe bool
		if err = conn.QueryRow(ctx, "SELECT "+guard, payload).Scan(&safe); err != nil {
			t.Fatalf("guard failed under an empty search_path (restore would abort): %v", err)
		}
		if safe != want {
			t.Fatalf("guard(%s) = %t, want %t", payload, safe, want)
		}
	}
}
