package migrations_test

import (
	"context"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"github.com/google/uuid"
	"os"
	"testing"
)

func TestRelayUncertaintyUpgradePreservesLegacyOutcomes(t *testing.T) {
	pool, cleanup := operationalTestPoolThrough(t, "000069_offline_location_atomic.up.sql")
	defer cleanup()
	ctx := context.Background()
	provider := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO providers(id,phone) VALUES($1,'+2348000007070')`, provider); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"PENDING", "DELIVERING", "DEAD_LETTER", "UNKNOWN_OUTCOME"} {
		if _, err := pool.Exec(ctx, `INSERT INTO dispatch_fleet_relay_operations(operation_type,provider_id,assignment_id,idempotency_key,request_fingerprint,payload,payload_hash,status) VALUES('PROVIDER_ASSIGNMENT_ACCEPT',$1,$2,$2,'fixture','{}','fixture',$2)`, provider, state); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile("000070_relay_uncertain_outcome_fence.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	var fenced int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_fleet_relay_operations WHERE readback_required`).Scan(&fenced); err != nil || fenced != 3 {
		t.Fatalf("legacy fences=%d err=%v", fenced, err)
	}
	repo := marketplace.NewRepository(pool)
	var deadID string
	if err = pool.QueryRow(ctx, `SELECT id::text FROM dispatch_fleet_relay_operations WHERE status='DEAD_LETTER'`).Scan(&deadID); err != nil {
		t.Fatal(err)
	}
	if err = repo.ReplayRelay(ctx, deadID); err != marketplace.ErrNotFound {
		t.Fatalf("legacy uncertain replay=%v", err)
	}
	down, err := os.ReadFile("000070_relay_uncertain_outcome_fence.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err == nil {
		t.Fatal("uncertainty protection silently rolled back")
	}
}
