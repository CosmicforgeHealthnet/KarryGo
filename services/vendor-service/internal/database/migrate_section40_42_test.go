package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSection4042ExistingDataUpgradeAndCriticalSchema(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	if err = RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}

	suffix := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO vendor_idempotency_keys
		(scope_id,endpoint,idempotency_key,request_fingerprint,status,response_status,response_body,created_at,expires_at)
		VALUES($1,'/legacy','legacy-key','legacy-fingerprint','COMPLETED',201,'{}',now(),now()+interval '1 day')`, "legacy-"+suffix); err != nil {
		t.Fatal(err)
	}
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}

	var status string
	var updated time.Time
	if err = pool.QueryRow(ctx, `SELECT status,updated_at FROM vendor_idempotency_keys WHERE scope_id=$1`, "legacy-"+suffix).Scan(&status, &updated); err != nil {
		t.Fatalf("legacy row did not survive upgrade: %v", err)
	}
	if status != "COMPLETED" || updated.IsZero() {
		t.Fatalf("legacy status=%q updated=%v", status, updated)
	}
	for _, relation := range []string{
		"vendor_privacy_retention_policies",
		"vendor_data_legal_holds",
		"commerce_order_restricted_evidence",
		"vendor_privacy_retention_jobs",
		"vendor_idempotency_keys_actor_operation_idx",
		"vendor_privacy_retention_jobs_due_idx",
	} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, relation).Scan(&exists); err != nil || !exists {
			t.Fatalf("relation %s exists=%t err=%v", relation, exists, err)
		}
	}
	var triggerExists bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_trigger
		WHERE tgname='commerce_parent_orders_privacy_retention_trg' AND NOT tgisinternal)`).Scan(&triggerExists); err != nil || !triggerExists {
		t.Fatalf("retention trigger exists=%t err=%v", triggerExists, err)
	}
}
