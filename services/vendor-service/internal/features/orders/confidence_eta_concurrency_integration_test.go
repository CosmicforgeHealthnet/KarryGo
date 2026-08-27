package orders

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
)

func confidenceETATestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres confidence ETA races")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = database.ApplyMigrations(ctx, pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	return pool
}

func seedETAParent(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := "po_" + uuid.NewString()
	now := time.Now().UTC()
	if _, err := pool.Exec(context.Background(), `INSERT INTO commerce_parent_orders(id,customer_id,currency,state,created_at,updated_at) VALUES($1,$2,'NGN','CREATED',$3,$3)`, id, "customer_"+uuid.NewString(), now); err != nil {
		t.Fatal(err)
	}
	return id
}

func databaseETAInput(parent, event string, version int64) ETAInput {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return ETAInput{ParentOrderID: parent, MissionSlotRef: "mission_1", PlanVersion: 1,
		SourceEventID: event, SourceAuthority: "DISPATCH", SourceVersion: version,
		SourceOccurredAt: now, Trigger: TriggerMissionCreated, Domain: "FOOD",
		RouteSeconds: 900, RouteProvider: "route", RouteEvidenceVersion: "route_1",
		RouteObservedAt: now, PickupStopCount: 1}
}

func TestConfidenceETAVersionRaceIntegration(t *testing.T) {
	pool := confidenceETATestPool(t)
	parent := seedETAParent(t, pool)
	store := NewStore(pool)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for version := int64(1); version <= 2; version++ {
		wg.Add(1)
		go func(version int64) {
			defer wg.Done()
			<-start
			tx, err := pool.BeginTx(context.Background(), pgx.TxOptions{})
			if err == nil {
				_, _, err = store.applyConfidenceETATx(context.Background(), tx, databaseETAInput(parent, "event_"+string(rune('0'+version)), version))
			}
			if err == nil {
				err = tx.Commit(context.Background())
			} else if tx != nil {
				_ = tx.Rollback(context.Background())
			}
			errs <- err
		}(version)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var versions, distinct int
	if err := pool.QueryRow(context.Background(), `SELECT count(*),count(DISTINCT eta_version) FROM commerce_delivery_eta_versions WHERE parent_order_id=$1`, parent).Scan(&versions, &distinct); err != nil {
		t.Fatal(err)
	}
	if versions < 1 || versions > 2 || versions != distinct {
		t.Fatalf("history versions=%d distinct=%d", versions, distinct)
	}
}

func TestConfidenceETAStalePlanAndOldProviderIntegration(t *testing.T) {
	pool := confidenceETATestPool(t)
	parent := seedETAParent(t, pool)
	store := NewStore(pool)
	apply := func(in ETAInput) (DeliveryETA, bool) {
		t.Helper()
		tx, err := pool.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background()) //nolint:errcheck
		eta, created, err := store.applyConfidenceETATx(context.Background(), tx, in)
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(context.Background()); err != nil {
			t.Fatal(err)
		}
		return eta, created
	}
	current := databaseETAInput(parent, "event_plan_2", 2)
	current.PlanVersion, current.ProviderRef, current.ProviderAssigned = 2, "provider_1", true
	if _, created := apply(current); !created {
		t.Fatal("initial current ETA not created")
	}
	stale := databaseETAInput(parent, "event_plan_1", 3)
	stale.PlanVersion = 1
	if eta, created := apply(stale); created || eta.PlanVersion != 2 {
		t.Fatalf("stale plan moved current pointer: created=%t eta=%+v", created, eta)
	}
	oldProvider := databaseETAInput(parent, "event_old_provider", 4)
	oldProvider.PlanVersion, oldProvider.ProviderRef, oldProvider.ProviderAssigned = 2, "provider_2", true
	oldProvider.Trigger = TriggerTimingUpdated
	if eta, created := apply(oldProvider); created || eta.ETAVersion != 1 {
		t.Fatalf("old provider update was accepted: created=%t eta=%+v", created, eta)
	}
}
