package outbox

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func publisherTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required for outbox publisher certification")
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	var ready bool
	if err = db.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='taxi_outbox' AND column_name='last_error')`).Scan(&ready); err != nil || !ready {
		t.Skip("Taxi migration 009 is not applied")
	}
	return db
}

type countingDestination struct {
	mu         sync.Mutex
	targetID   string
	deliveries int
	failures   int
}

func (d *countingDestination) Deliver(_ context.Context, record Record) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.targetID != "" && record.ID != d.targetID {
		return nil
	}
	d.deliveries++
	if d.failures > 0 {
		d.failures--
		return errors.New("transient destination failure")
	}
	return nil
}

func seedPublisherRecord(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	id, aggregate := uuid.NewString(), uuid.NewString()
	_, err := db.Exec(context.Background(), `INSERT INTO taxi_outbox(id,aggregate_type,aggregate_id,event_type,payload,available_at,target) VALUES($1,'taxi_goods_mission',$2,'taxi.goods.completed.v1','{}','1900-01-01','vendor-service')`, id, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(context.Background(), `DELETE FROM taxi_outbox WHERE id=$1`, id) })
	return id
}

func TestPublisherMultipleWorkersAndTransientRetry(t *testing.T) {
	db := publisherTestDB(t)
	id := seedPublisherRecord(t, db)
	destination := &countingDestination{targetID: id, failures: 1}
	store := NewStore(db)
	publisher := NewPublisher(store, destination, "worker-a")
	if err := publisher.ProcessBatch(context.Background(), 1); err == nil {
		t.Fatal("transient failure was hidden")
	}
	_, _ = db.Exec(context.Background(), `UPDATE taxi_outbox SET available_at='1900-01-01' WHERE id=$1`, id)
	workers := []*Publisher{NewPublisher(store, destination, "worker-b"), NewPublisher(store, destination, "worker-c")}
	var wg sync.WaitGroup
	for _, worker := range workers {
		wg.Add(1)
		go func(worker *Publisher) { defer wg.Done(); _ = worker.ProcessBatch(context.Background(), 1) }(worker)
	}
	wg.Wait()
	var attempts int
	var delivered *time.Time
	if err := db.QueryRow(context.Background(), `SELECT attempts,delivered_at FROM taxi_outbox WHERE id=$1`, id).Scan(&attempts, &delivered); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || delivered == nil || destination.deliveries != 2 {
		t.Fatalf("attempts=%d delivered=%v calls=%d", attempts, delivered, destination.deliveries)
	}
}

func TestPublisherLeaseRecoveryAfterSendBeforeAck(t *testing.T) {
	db := publisherTestDB(t)
	id := seedPublisherRecord(t, db)
	store := NewStore(db)
	destination := &countingDestination{targetID: id}
	records, err := store.Claim(context.Background(), "crashed-worker", 1, time.Minute)
	if err != nil || len(records) != 1 {
		t.Fatalf("claim=%d err=%v", len(records), err)
	}
	if err = destination.Deliver(context.Background(), records[0]); err != nil {
		t.Fatal(err)
	}
	// The process dies before Ack. Expiring the lease models recovery without
	// sleeping; Vendor dedupes the deterministic event id on redelivery.
	_, _ = db.Exec(context.Background(), `UPDATE taxi_outbox SET lease_until=now()-interval '1 second' WHERE id=$1`, id)
	if err = NewPublisher(store, destination, "recovery-worker").ProcessBatch(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	var delivered bool
	if err = db.QueryRow(context.Background(), `SELECT delivered_at IS NOT NULL FROM taxi_outbox WHERE id=$1`, id).Scan(&delivered); err != nil || !delivered || destination.deliveries != 2 {
		t.Fatalf("delivered=%t calls=%d err=%v", delivered, destination.deliveries, err)
	}
}
