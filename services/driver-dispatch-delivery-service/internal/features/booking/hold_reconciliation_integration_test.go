package booking

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The claim query once compared updated_at against "$1-$5::interval", which
// PostgreSQL types as interval-interval and rejects on every tick, so no
// abandoned hold was ever reconciled. This pins the real query against a real
// schema.
func TestClaimAbandonedHoldsRunsAgainstPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("DISPATCH_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("DISPATCH_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC()
	stale, fresh := uuid.NewString(), uuid.NewString()
	insert := `INSERT INTO dispatch_bookings(id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone,payment_method,payment_status,updated_at,hold_reconciliation_next_at)
		VALUES($1,$2,'searching','pickup',6,3,'dropoff',7,4,0,'r','+2348000000000','wallet','unpaid',$3,$3)`
	if _, err = db.Exec(ctx, insert, stale, uuid.NewString(), now.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, insert, fresh, uuid.NewString(), now); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM dispatch_bookings WHERE id IN ($1,$2)`, stale, fresh)
	})

	repo := NewPostgresRepository(db)
	ids, err := repo.claimAbandonedHolds(ctx, now, "test-owner", HoldRetryPolicy{Lease: time.Minute, AbandonAfter: 2 * time.Minute, MaxAttempts: 10})
	if err != nil {
		t.Fatalf("claim query failed: %v", err)
	}
	claimed := map[string]bool{}
	for _, id := range ids {
		claimed[id] = true
	}
	if !claimed[stale] || claimed[fresh] {
		t.Fatalf("claimed=%v want stale only", ids)
	}
}
