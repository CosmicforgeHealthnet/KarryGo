package movement

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/events"
)

func reconcilePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOT EXECUTED: TAXI_TEST_DATABASE_URL (isolated disposable PostgreSQL) is required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type fakeProjection struct {
	versions map[string]int64
	asked    [][]string
}

func (f *fakeProjection) ProjectedVersions(_ context.Context, refs []string) (map[string]int64, error) {
	f.asked = append(f.asked, refs)
	return f.versions, nil
}

// Backfill republishes what the projection is missing or behind on, and
// nothing else. Replaying a movement the consumer already has would be
// noise; inventing one it never had would be a lie.
func TestPostgresReconcileRepublishesOnlyWhatTheProjectionLacks(t *testing.T) {
	db := reconcilePool(t)
	ctx := context.Background()
	// Exclude fixtures left by other packages in the shared test database.
	var since time.Time
	if err := db.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&since); err != nil {
		t.Fatal(err)
	}
	market, zone, code := uuid.NewString(), uuid.NewString(), "RC-"+uuid.NewString()[:8]
	if _, err := db.Exec(ctx, `INSERT INTO markets(id,country,name,code) VALUES($1,'NG','Reconcile',$2)`, market, code); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO operating_zones(id,market_id,name,code,center_lat,center_lng,match_radius_km,supported_classes) VALUES($1,$2,'Reconcile',$3,6.5,3.3,5,'{economy}')`, zone, market, code); err != nil {
		t.Fatal(err)
	}
	newBooking := func(status string) string {
		id := uuid.NewString()
		if _, err := db.Exec(ctx, `INSERT INTO taxi_bookings(id,customer_id,market_id,operating_zone_id,vehicle_class,status,movement_version)
			VALUES($1,$2,$3,$4,'economy',$5,1)`, id, uuid.NewString(), market, zone, status); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO taxi_booking_stops(id,booking_id,sequence,type,address,lat,lng) VALUES($1,$2,1,'pickup','Yaba',6.5,3.3),($3,$2,2,'dropoff','Lekki',6.6,3.4)`,
			uuid.NewString(), id, uuid.NewString()); err != nil {
			t.Fatal(err)
		}
		return id
	}
	missing, current := newBooking("in_progress"), newBooking("completed")
	t.Cleanup(func() {
		for _, id := range []string{missing, current} {
			_, _ = db.Exec(ctx, `DELETE FROM taxi_outbox WHERE aggregate_id=$1`, id)
			_, _ = db.Exec(ctx, `DELETE FROM taxi_booking_stops WHERE booking_id=$1`, id)
			_, _ = db.Exec(ctx, `DELETE FROM taxi_bookings WHERE id=$1`, id)
		}
		_, _ = db.Exec(ctx, `DELETE FROM operating_zones WHERE id=$1`, zone)
		_, _ = db.Exec(ctx, `DELETE FROM markets WHERE id=$1`, market)
	})

	// The projection already holds the second booking at Taxi's own version.
	projection := &fakeProjection{versions: map[string]int64{current: 1}}
	reconciler := &Reconciler{Pool: db, Writer: &Writer{Enabled: true}, Reader: projection, Batch: 500}
	result, err := reconciler.Reconcile(ctx, since)
	if err != nil {
		t.Fatal(err)
	}
	if result.Republished != 1 {
		t.Fatalf("republished = %d, want only the movement the projection lacks", result.Republished)
	}
	var queued int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM taxi_outbox WHERE aggregate_id=$1 AND target='customer-service'`, missing).Scan(&queued); err != nil || queued != 1 {
		t.Fatalf("queued for the missing movement = %d (%v)", queued, err)
	}
	if err = db.QueryRow(ctx, `SELECT count(*) FROM taxi_outbox WHERE aggregate_id=$1 AND target='customer-service'`, current).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("an up-to-date movement was republished %d times", queued)
	}

	// What it republished is the state Taxi actually holds, with a version
	// above the one the projection reported.
	var payload []byte
	if err = db.QueryRow(ctx, `SELECT payload FROM taxi_outbox WHERE aggregate_id=$1 AND target='customer-service'`, missing).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var envelope events.Envelope
	if err = json.Unmarshal(payload, &envelope); err != nil || envelope.Validate() != nil {
		t.Fatalf("republished envelope is not consumable: %v", err)
	}
	var summary events.MovementSummary
	if err = json.Unmarshal(envelope.Payload, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.State != events.MovementStateInProgress || summary.SourceRef != missing || envelope.SourceVersion < 2 {
		t.Fatalf("republished %+v at version %d", summary, envelope.SourceVersion)
	}

	// A second pass with the projection now up to date republishes nothing.
	projection.versions[missing] = envelope.SourceVersion
	second, err := reconciler.Reconcile(ctx, since)
	if err != nil || second.Republished != 0 {
		t.Fatalf("second pass republished %d (%v)", second.Republished, err)
	}
}
